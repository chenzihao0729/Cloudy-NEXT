package sourceplugin

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

const MaxScriptSize = 1 << 20

var errInvalidScript = errors.New("source plugin must export musicUrl or handle LX musicUrl requests")

type Info struct {
	Name    string
	Sources map[string]string
}

type Resolver struct {
	loop    *eventloop.EventLoop
	ctx     context.Context
	cancel  context.CancelFunc
	client  *http.Client
	mu      sync.Mutex
	callMu  sync.Mutex
	closed  bool
	program *goja.Program
	state   *scriptState
	vm      *goja.Runtime
	logMu   sync.RWMutex
	logFn   func(level, message string)
}

type scriptState struct {
	handlers map[string]goja.Callable
	info     Info
	active   context.Context
}

type response struct {
	StatusCode int                 `json:"statusCode"`
	Status     int                 `json:"status"`
	OK         bool                `json:"ok"`
	Headers    map[string][]string `json:"headers"`
	Body       any                 `json:"body"`
	BodyText   string              `json:"bodyText"`
}

func (r response) jsValue() map[string]any {
	return map[string]any{
		"statusCode": r.StatusCode,
		"status":     r.Status,
		"ok":         r.OK,
		"headers":    r.Headers,
		"body":       r.Body,
		"bodyText":   r.BodyText,
	}
}

func Validate(script string) (Info, error) {
	r, info, err := newResolver(script, false)
	if err != nil {
		return Info{}, err
	}
	r.Close()
	return info, nil
}

func NewResolver(script string) (*Resolver, Info, error) {
	return newResolver(script, true)
}

func newResolver(script string, allowNetwork bool) (*Resolver, Info, error) {
	if len(script) == 0 || len(script) > MaxScriptSize {
		return nil, Info{}, fmt.Errorf("source plugin size must be between 1 byte and %d bytes", MaxScriptSize)
	}
	program, err := goja.Compile("music-source.js", script, false)
	if err != nil {
		return nil, Info{}, fmt.Errorf("invalid JavaScript: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Resolver{loop: eventloop.NewEventLoop(eventloop.EnableConsole(false)), ctx: ctx, cancel: cancel, program: program}
	if allowNetwork {
		r.client = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || len(via) >= 5 {
				return errors.New("plugin redirects are restricted to HTTPS")
			}
			return nil
		}}
	}
	r.loop.Start()
	state := &scriptState{handlers: make(map[string]goja.Callable)}
	r.state = state
	ready := make(chan error, 1)
	if !r.loop.RunOnLoop(func(vm *goja.Runtime) {
		r.vm = vm
		state.active = ctx
		if err := r.setup(vm, state, script); err != nil {
			ready <- err
			return
		}
		if _, err := vm.RunProgram(program); err != nil {
			ready <- fmt.Errorf("plugin initialization failed: %w", err)
			return
		}
		if err := r.checkEntry(vm, state); err != nil {
			ready <- err
			return
		}
		ready <- nil
	}) {
		r.Close()
		return nil, Info{}, errors.New("plugin runtime stopped")
	}
	timer := time.AfterFunc(5*time.Second, func() { r.interrupt("plugin initialization timed out") })
	select {
	case err := <-ready:
		timer.Stop()
		if err != nil {
			r.Close()
			return nil, Info{}, err
		}
	case <-time.After(5 * time.Second):
		r.Close()
		return nil, Info{}, errors.New("plugin initialization timed out")
	}
	return r, state.info, nil
}

func (r *Resolver) setup(vm *goja.Runtime, state *scriptState, script string) error {
	_ = vm.Set("module", map[string]any{"exports": map[string]any{}})
	_ = vm.Set("exports", map[string]any{})
	consoleLog := func(level string) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			parts := make([]string, 0, len(call.Arguments))
			for _, argument := range call.Arguments {
				parts = append(parts, safeLogValue(argument.Export()))
			}
			r.logMu.RLock()
			logFn := r.logFn
			r.logMu.RUnlock()
			if logFn != nil {
				logFn(level, strings.Join(parts, " "))
			}
			return goja.Undefined()
		}
	}
	_ = vm.Set("console", map[string]any{
		"log": consoleLog("log"), "info": consoleLog("info"), "debug": consoleLog("debug"),
		"warn": consoleLog("warn"), "error": consoleLog("error"),
	})
	request := func(call goja.FunctionCall) goja.Value { return r.request(vm, state, call) }
	events := map[string]any{"request": "request", "inited": "inited", "updateAlert": "updateAlert"}
	lx := map[string]any{
		"EVENT_NAMES":       events,
		"env":               map[string]any{"platform": "windows", "os": "Windows", "device": "PC", "isMobile": false},
		"version":           "1.0.4",
		"currentScriptInfo": map[string]any{"rawScript": script, "name": scriptName(script)},
		"on": func(call goja.FunctionCall) goja.Value {
			event := call.Argument(0).String()
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				state.handlers[event] = fn
			}
			return goja.Undefined()
		},
		"send": func(call goja.FunctionCall) goja.Value {
			if call.Argument(0).String() == "inited" {
				if raw := call.Argument(1).Export(); raw != nil {
					if data, ok := raw.(map[string]interface{}); ok {
						state.info.Sources = sourceNames(data["sources"])
					}
				}
			}
			return goja.Undefined()
		},
		"request": request,
	}
	buffer := map[string]any{
		"from": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(bufferValue(call.Argument(0).String(), call.Argument(1).String()))
		},
		"bufToString": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(bufferString(call.Argument(0).Export(), call.Argument(1).String()))
		},
	}
	crypto := map[string]any{
		"md5": func(call goja.FunctionCall) goja.Value {
			sum := md5.Sum([]byte(call.Argument(0).String()))
			return vm.ToValue(hex.EncodeToString(sum[:]))
		},
	}
	lx["utils"] = map[string]any{"buffer": buffer, "crypto": crypto}
	_ = vm.Set("lx", lx)
	// CeruMusic scripts use a separate global API; this compatibility surface
	// intentionally maps only its request and notification calls.
	_ = vm.Set("cerumusic", map[string]any{
		"request":           request,
		"utils":             lx["utils"],
		"env":               lx["env"],
		"version":           lx["version"],
		"currentScriptInfo": lx["currentScriptInfo"],
		"NoticeCenter":      func(goja.FunctionCall) goja.Value { return goja.Undefined() },
		"stopRequests":      func(goja.FunctionCall) goja.Value { return goja.Undefined() },
	})
	return nil
}

func (r *Resolver) SetLogHandler(handler func(level, message string)) {
	r.logMu.Lock()
	r.logFn = handler
	r.logMu.Unlock()
}

func safeLogValue(value any) string {
	text := fmt.Sprint(value)
	text = loggedURL.ReplaceAllStringFunc(text, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.RawQuery == "" {
			return raw
		}
		parsed.RawQuery = "[redacted]"
		parsed.Fragment = ""
		return parsed.String()
	})
	if len(text) > 1024 {
		text = text[:1024] + "…"
	}
	for _, marker := range []string{"X-API-Key", "apiKey", "apikey", "token", "authorization", "cookie"} {
		lower := strings.ToLower(text)
		if index := strings.Index(lower, strings.ToLower(marker)); index >= 0 {
			text = text[:index] + marker + "=[redacted]"
			break
		}
	}
	return text
}

var loggedURL = regexp.MustCompile(`https?://[^\s"'<>]+`)

func RedactLog(message string) string { return safeLogValue(message) }

func (r *Resolver) checkEntry(vm *goja.Runtime, state *scriptState) error {
	value := vm.Get("module").ToObject(vm).Get("exports")
	if value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
		object := value.ToObject(vm)
		if _, ok := goja.AssertFunction(object.Get("musicUrl")); ok {
			state.info.Name = scriptNameFromExports(vm, value)
			if sources := object.Get("sources"); sources != nil && !goja.IsUndefined(sources) && !goja.IsNull(sources) {
				state.info.Sources = sourceNames(sources.Export())
			}
			return nil
		}
	}
	if _, ok := state.handlers["request"]; ok {
		if state.info.Sources == nil {
			state.info.Sources = map[string]string{}
		}
		return nil
	}
	return errInvalidScript
}

func scriptNameFromExports(vm *goja.Runtime, value goja.Value) string {
	info := value.ToObject(vm).Get("pluginInfo")
	if info != nil && !goja.IsUndefined(info) && !goja.IsNull(info) {
		name := info.ToObject(vm).Get("name")
		if name != nil && !goja.IsUndefined(name) {
			return name.String()
		}
	}
	return "JavaScript 音源"
}

func scriptName(script string) string {
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if strings.HasPrefix(line, "@name ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "@name "))
		}
	}
	return "JavaScript 音源"
}

func sourceNames(raw any) map[string]string {
	items, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string]string, len(items))
	for id, value := range items {
		if row, ok := value.(map[string]interface{}); ok {
			name, _ := row["name"].(string)
			result[id] = name
		}
	}
	return result
}

func (r *Resolver) Resolve(ctx context.Context, platform string, musicInfo map[string]any, quality string) (string, error) {
	r.callMu.Lock()
	defer r.callMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	type result struct {
		value any
		err   error
	}
	done := make(chan result, 1)
	if !r.loop.RunOnLoop(func(vm *goja.Runtime) {
		state := r.state
		state.active = ctx
		payload := map[string]any{"action": "musicUrl", "source": platform, "info": map[string]any{"musicInfo": musicInfo, "type": quality}}
		var value goja.Value
		if handler, ok := r.requestHandler(vm); ok {
			value, _ = handler(goja.Undefined(), vm.ToValue(payload))
		} else {
			fn := vm.Get("module").ToObject(vm).Get("exports").ToObject(vm).Get("musicUrl")
			call, ok := goja.AssertFunction(fn)
			if !ok {
				done <- result{err: errInvalidScript}
				return
			}
			value, _ = call(goja.Undefined(), vm.ToValue(platform), vm.ToValue(musicInfo), vm.ToValue(quality))
		}
		if value == nil {
			done <- result{err: errors.New("plugin returned no playback address")}
			return
		}
		if promise, ok := value.Export().(*goja.Promise); ok {
			if promise.State() != goja.PromiseStatePending {
				if promise.State() == goja.PromiseStateRejected {
					done <- result{err: errors.New(promise.Result().String())}
				} else {
					done <- result{value: promise.Result().Export()}
				}
				return
			}
			obj := value.ToObject(vm)
			then, _ := goja.AssertFunction(obj.Get("then"))
			if then == nil {
				done <- result{err: errors.New("plugin returned an unsupported Promise")}
				return
			}
			resolve := vm.ToValue(func(call goja.FunctionCall) goja.Value {
				done <- result{value: call.Argument(0).Export()}
				return goja.Undefined()
			})
			reject := vm.ToValue(func(call goja.FunctionCall) goja.Value {
				done <- result{err: errors.New(call.Argument(0).String())}
				return goja.Undefined()
			})
			_, _ = then(value, resolve, reject)
			return
		}
		done <- result{value: value.Export()}
	}) {
		return "", errors.New("plugin runtime is closed")
	}
	timer := time.AfterFunc(20*time.Second, func() { r.interrupt("plugin request timed out") })
	select {
	case got := <-done:
		timer.Stop()
		if got.err != nil {
			return "", got.err
		}
		address, ok := got.value.(string)
		if !ok {
			if object, ok := got.value.(map[string]interface{}); ok {
				address, _ = object["url"].(string)
			}
		}
		parsed, err := url.Parse(address)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return "", errors.New("plugin returned an invalid HTTP(S) playback URL")
		}
		return address, nil
	case <-ctx.Done():
		timer.Stop()
		r.interrupt(ctx.Err())
		return "", ctx.Err()
	}
}

func (r *Resolver) requestHandler(vm *goja.Runtime) (goja.Callable, bool) {
	if r.state == nil {
		return nil, false
	}
	fn, ok := r.state.handlers["request"]
	return fn, ok
}

func (r *Resolver) request(vm *goja.Runtime, state *scriptState, call goja.FunctionCall) goja.Value {
	requestURL := call.Argument(0).String()
	callback, hasCallback := goja.AssertFunction(call.Argument(2))
	promise, resolve, reject := vm.NewPromise()
	options, _ := call.Argument(1).Export().(map[string]interface{})
	go func() {
		resp, err := r.doRequest(state.active, requestURL, options)
		r.loop.RunOnLoop(func(vm *goja.Runtime) {
			if hasCallback {
				if err != nil {
					_, _ = callback(goja.Undefined(), vm.ToValue(map[string]any{"message": err.Error()}), goja.Null())
					_ = reject(err.Error())
					return
				}
				_, _ = callback(goja.Undefined(), goja.Null(), vm.ToValue(resp.jsValue()))
			}
			if err != nil {
				_ = reject(err.Error())
			} else {
				_ = resolve(resp.jsValue())
			}
		})
	}()
	return vm.ToValue(promise)
}

func (r *Resolver) doRequest(ctx context.Context, rawURL string, options map[string]interface{}) (response, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return response{}, errors.New("plugin requests must use HTTPS")
	}
	if strings.Contains(strings.ToLower(parsed.RawQuery), "checkupdate=") {
		return response{StatusCode: http.StatusOK, Status: http.StatusOK, OK: true, Headers: http.Header{}, Body: map[string]any{"code": 0, "data": nil}}, nil
	}
	if r.client == nil {
		return response{}, errors.New("network requests are disabled during validation")
	}
	if ctx == nil {
		ctx = r.ctx
	}
	method, _ := options["method"].(string)
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if text, ok := options["body"].(string); ok {
		body = strings.NewReader(text)
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), parsed.String(), body)
	if err != nil {
		return response{}, err
	}
	if headers, ok := options["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strings.EqualFold(key, "host") || strings.EqualFold(key, "content-length") {
				continue
			}
			req.Header.Set(key, fmt.Sprint(value))
		}
	}
	if rawTimeout, ok := options["timeout"]; ok {
		if timeout, valid := asInt64(rawTimeout); valid && timeout > 0 && timeout < 60000 {
			var cancel context.CancelFunc
			req, cancel = withTimeout(req, time.Duration(timeout)*time.Millisecond)
			defer cancel()
		}
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return response{}, err
	}
	text := string(data)
	var parsedBody any = text
	if strings.Contains(resp.Header.Get("Content-Type"), "json") {
		var value any
		if jsonErr := json.Unmarshal(data, &value); jsonErr == nil {
			parsedBody = value
		}
	}
	return response{StatusCode: resp.StatusCode, Status: resp.StatusCode, OK: resp.StatusCode >= 200 && resp.StatusCode < 300, Headers: resp.Header, Body: parsedBody, BodyText: text}, nil
}

func withTimeout(req *http.Request, timeout time.Duration) (*http.Request, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	return req.WithContext(ctx), cancel
}

func (r *Resolver) interrupt(value any) {
	if r.vm != nil {
		r.vm.Interrupt(value)
	}
}

func (r *Resolver) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()
	r.cancel()
	r.interrupt("plugin closed")
	r.loop.Terminate()
}

func asInt64(value any) (int64, bool) {
	switch n := value.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), n == float64(int64(n))
	default:
		return 0, false
	}
}

func bufferValue(value, encoding string) map[string]any {
	b := []byte(value)
	switch strings.ToLower(encoding) {
	case "base64":
		if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
			b = decoded
		}
	case "hex":
		if decoded, err := hex.DecodeString(strings.TrimPrefix(value, "0x")); err == nil {
			b = decoded
		}
	}
	return map[string]any{"__beansBuffer": true, "base64": base64.StdEncoding.EncodeToString(b), "hex": hex.EncodeToString(b), "text": string(b)}
}

func bufferString(value any, encoding string) string {
	data, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Sprint(value)
	}
	switch strings.ToLower(encoding) {
	case "base64":
		return fmt.Sprint(data["base64"])
	case "hex":
		return fmt.Sprint(data["hex"])
	default:
		return fmt.Sprint(data["text"])
	}
}
