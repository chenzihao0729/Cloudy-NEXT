package player

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"
)

// The waveform of a track: how loud it is along its length, in
// WaveBuckets stretches, for the progress bar to show where it swells and
// where it is quiet. It is measured from a second decoder of the same
// download, from its start, on a goroutine, the first time the progress
// bar asks for it (Wave), filling in as it goes; the last few are kept, a
// track played again showing its own at once.

// WaveBuckets is how many stretches a waveform has.
const WaveBuckets = 240

type waveform struct {
	mu     sync.Mutex
	v      [WaveBuckets]float32
	filled int // how many stretches, from the start, are measured
}

// waveKeep is how many tracks' waveforms are kept.
const waveKeep = 24

// waving lets one waveform be measured at a time.
var waving = make(chan struct{}, 1)

// Wave returns the waveform of the track playing, each stretch its RMS,
// 1 at full scale, for it to be drawn against its loudest, and how many
// from the start are measured yet; none at all for a track with nothing to measure from.
// The first ask starts measuring it.
func (p *Player) Wave() (v [WaveBuckets]float32, filled int) {
	t := p.Current()
	if t == nil || p.stream == nil {
		return v, 0
	}
	key := loudKey(*t, p.infoPath())
	if key == "" {
		return v, 0
	}
	w := p.waves[key]
	if w == nil {
		w = p.startWave(key, t.Duration)
		if w == nil {
			return v, 0
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.v, w.filled
}

func (p *Player) infoPath() string {
	if p.info != nil {
		return p.info.Path
	}
	return ""
}

// startWave starts measuring the waveform of the track playing, kept under
// key, and returns it, or nil if there is nothing to measure it from.
func (p *Player) startWave(key string, durationMs int64) *waveform {
	dl, _ := p.stream.source()
	if dl == nil || durationMs <= 0 {
		return nil
	}
	if p.waves == nil {
		p.waves = map[string]*waveform{}
	}
	if len(p.waves) >= waveKeep {
		for k := range p.waves {
			if k != p.waveKey {
				delete(p.waves, k)
				break
			}
		}
	}
	w := &waveform{}
	p.waves[key] = w
	if p.waveStop != nil {
		p.waveStop()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.waveStop, p.waveKey = cancel, key
	go func() {
		defer cancel()
		select {
		case waving <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-waving }()
		// Let playback fill its decode buffer first.
		timer := time.NewTimer(800 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
		s, err := openSharedContext(ctx, dl, durationMs)
		if err != nil {
			return
		}
		defer s.Close()
		measureWave(ctx, s, durationMs, w)
	}()
	return w
}

// measureWave decodes s to its end, filling w as it goes.
func measureWave(ctx context.Context, dec audio.Decoder, durationMs int64, w *waveform) {
	ch := max(1, dec.Channels())
	total := float64(durationMs) / 1000 * float64(dec.SampleRate())
	if total <= 0 {
		return
	}
	per := total / WaveBuckets
	buf := make([]float32, 8192*ch)
	var frame float64
	var sum float64
	var n int
	bucket := 0
	finish := func() {
		v := float32(0)
		if n > 0 {
			v = float32(math.Sqrt(sum / float64(n)))
		}
		w.mu.Lock()
		w.v[bucket] = v
		w.filled = bucket + 1
		w.mu.Unlock()
		sum, n = 0, 0
		bucket++
	}
	for reads := 0; ; reads++ {
		if reads%16 == 0 && ctx.Err() != nil {
			return
		}
		k, err := dec.Read(buf)
		for i := 0; i+ch <= k; i += ch {
			var m float32
			for c := range ch {
				m += buf[i+c]
			}
			m /= float32(ch)
			sum += float64(m * m)
			n++
			frame++
			if frame >= per*float64(bucket+1) && bucket < WaveBuckets-1 {
				finish()
			}
		}
		if err != nil || k == 0 {
			break
		}
	}
	for bucket < WaveBuckets {
		finish()
	}
}
