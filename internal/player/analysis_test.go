package player

import (
	"context"
	"io"
	"math"
	"testing"
)

// A tone in the left channel alone shows in the left's spectrum at its
// pitch; the same tone in both is one channel (correlation 1), and in
// opposite phase, -1. Nothing is measured while no one asks.
func TestAnalysisChannelsAndCorrelation(t *testing.T) {
	const rate = 44100
	tone := func(l, r float32) []float32 {
		out := make([]float32, fftSize*4*2)
		for i := 0; i < len(out)/2; i++ {
			v := float32(0.5 * math.Sin(2*math.Pi*440*float64(i)/rate))
			out[2*i], out[2*i+1] = v*l, v*r
		}
		return out
	}
	s := newSpectrum(rate, 0)
	s.feed(0, tone(1, 0))
	if s.analyzer() != nil {
		t.Fatal("measured with no one asking")
	}
	SetAnalyzing(true)
	defer SetAnalyzing(false)
	s.feed(fftSize*4, tone(1, 0))
	an, ok := s.analyzer().at(5)
	if !ok {
		t.Fatal("chunk 5 not measured")
	}
	bin, band := 440*fftSize/rate, 0
	for b := range AnBands {
		if s.analyzer().edges[b] <= bin && bin < s.analyzer().edges[b+1] {
			band = b
		}
	}
	if an.L[band] < 0.6 || an.R[band] > 0.05 {
		t.Errorf("440 Hz in the left: band %d L %.2f R %.2f", band, an.L[band], an.R[band])
	}
	if an.RmsL < 0.3 || an.RmsR != 0 || an.Peak < 0.49 {
		t.Errorf("rms %.2f %.2f peak %.2f", an.RmsL, an.RmsR, an.Peak)
	}
	s.feed(fftSize*8, tone(1, 1))
	if an, _ := s.analyzer().at(9); an.Corr < 0.99 {
		t.Errorf("same in both: correlation %.2f", an.Corr)
	}
	s.feed(fftSize*12, tone(1, -1))
	if an, _ := s.analyzer().at(13); an.Corr > -0.99 {
		t.Errorf("opposed: correlation %.2f", an.Corr)
	}
	lows := make([]float32, 6)
	if n := s.analyzer().lows(15, lows); n != 6 {
		t.Errorf("%d lows measured in a row, want 6", n)
	}
}

// sineDecoder is a decoder of a tone growing louder from silence over
// its length, for the waveform.
type sineDecoder struct {
	n, at int
}

func (d *sineDecoder) SampleRate() int         { return 8000 }
func (d *sineDecoder) Channels() int           { return 2 }
func (d *sineDecoder) SetPosition(int64) error { return nil }
func (d *sineDecoder) Len() int64              { return int64(d.n) }
func (d *sineDecoder) Read(buf []float32) (int, error) {
	k := 0
	for k+1 < len(buf) && d.at < d.n {
		v := float32(float64(d.at) / float64(d.n) * math.Sin(float64(d.at)*0.3))
		buf[k], buf[k+1] = v, v
		k += 2
		d.at++
	}
	if d.at >= d.n {
		return k, io.EOF
	}
	return k, nil
}

// The waveform of a track swelling from silence rises along it, and is
// whole at its end.
func TestWaveformRisesWithTheTrack(t *testing.T) {
	var w waveform
	measureWave(context.Background(), &sineDecoder{n: 8000 * 20}, 20000, &w)
	if w.filled != WaveBuckets {
		t.Fatalf("%d of %d measured", w.filled, WaveBuckets)
	}
	if w.v[2] >= w.v[100] || w.v[100] >= w.v[WaveBuckets-2] {
		t.Errorf("not rising: %.2f %.2f %.2f", w.v[2], w.v[100], w.v[WaveBuckets-2])
	}
}
