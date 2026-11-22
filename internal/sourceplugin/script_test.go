package sourceplugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Both supported host contracts can initialize and return a playback URL.
func TestValidateAndResolveHostContracts(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{
			name:   "cerumusic",
			script: `module.exports = { pluginInfo: { name: "paid" }, sources: { tx: { name: "QQ" } }, musicUrl: async (source, info, quality) => "https://audio.example/" + info.songmid };`,
		},
		{
			name:   "lx",
			script: `const { EVENT_NAMES, on, send } = globalThis.lx; send(EVENT_NAMES.inited, { sources: { tx: { name: "QQ" } } }); on(EVENT_NAMES.request, async ({ action, source, info }) => action === "musicUrl" ? "https://audio.example/" + info.musicInfo.songmid : Promise.reject(new Error("unsupported")));`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := Validate(tt.script)
			if err != nil {
				t.Fatal(err)
			}
			if len(info.Sources) != 1 || info.Sources["tx"] != "QQ" {
				t.Fatalf("plugin sources = %#v", info.Sources)
			}
			resolver, _, err := NewResolver(tt.script)
			if err != nil {
				t.Fatal(err)
			}
			defer resolver.Close()
			got, err := resolver.Resolve(context.Background(), "tx", map[string]any{"songmid": "song-1"}, "320k")
			if err != nil || got != "https://audio.example/song-1" {
				t.Fatalf("Resolve() = %q, %v", got, err)
			}
		})
	}
}

// Ceru-style Promise requests resolve through the host bridge and keep JSON bodies structured.
func TestCeruRequestBridge(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" {
			t.Errorf("request path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"url":"https://audio.example/qq"}`))
	}))
	defer server.Close()
	script := `module.exports = { musicUrl: async () => { const response = await cerumusic.request("` + server.URL + `/api", { method: "GET" }); return response.body.url; } };`
	resolver, _, err := NewResolver(script)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	resolver.client = server.Client()
	got, err := resolver.Resolve(context.Background(), "tx", map[string]any{"songmid": "qq-song"}, "320k")
	if err != nil || got != "https://audio.example/qq" {
		t.Fatalf("Resolve() = %q, %v", got, err)
	}
}

// Scripts without a supported playback entry are rejected at validation.
func TestValidateRejectsUnsupportedScript(t *testing.T) {
	if _, err := Validate(`module.exports = { init() {} };`); err == nil {
		t.Fatal("unsupported script validated")
	}
}

// Some providers return HTTP stream URLs even when their parsing API uses HTTPS.
func TestResolveAllowsHTTPPlaybackURL(t *testing.T) {
	resolver, _, err := NewResolver(`module.exports = { musicUrl: () => "http://audio.example/song" };`)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	got, err := resolver.Resolve(context.Background(), "tx", map[string]any{"songmid": "song-1"}, "320k")
	if err != nil || got != "http://audio.example/song" {
		t.Fatalf("Resolve() = %q, %v", got, err)
	}
}

// Plugin console messages reach the host log handler with credential-like text redacted.
func TestConsoleLogsAreCapturedAndRedacted(t *testing.T) {
	resolver, _, err := NewResolver(`module.exports = { musicUrl: () => { console.info("resolve", "apiKey=secret-value"); return "https://audio.example/song"; } };`)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	var level, message string
	resolver.SetLogHandler(func(gotLevel, gotMessage string) { level, message = gotLevel, gotMessage })
	if _, err := resolver.Resolve(context.Background(), "tx", map[string]any{}, "320k"); err != nil {
		t.Fatal(err)
	}
	if level != "info" || !strings.Contains(message, "resolve") || strings.Contains(message, "secret-value") {
		t.Fatalf("captured log = %q %q", level, message)
	}
}
