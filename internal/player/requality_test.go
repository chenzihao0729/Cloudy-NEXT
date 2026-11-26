package player

import (
	"context"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// A level chosen while a song plays takes at once: the song loads again at
// it, from where it was, still playing, and the same song (#60).
func TestRequalityReloadsWhatPlaysAtOnce(t *testing.T) {
	quietNavigation(t)
	st := retryAudio(t)
	var asked []string
	songURL = func(ctx context.Context, id int64, level string) (*netease.Stream, error) {
		asked = append(asked, level)
		s := *st
		s.Level = level
		return &s, nil
	}
	level := "exhigh"
	p, ch := navigationPlayer(t, []netease.Track{{ID: 1, Duration: 5000}, {ID: 2}})
	p.Quality = func() string { return level }
	p.Jump(0)
	finishNavigation(t, p, ch)
	p.Seek(1500 * time.Millisecond)
	level = "lossless"
	p.Requality()
	finishNavigation(t, p, ch)
	if len(asked) < 2 || asked[len(asked)-1] != "lossless" {
		t.Fatalf("levels asked: %v", asked)
	}
	if p.Current().ID != 1 || !p.Playing() || !p.Ready() {
		t.Fatalf("after the change: current=%d playing=%v ready=%v", p.Current().ID, p.Playing(), p.Ready())
	}
	if at := p.Position(); at < time.Second {
		t.Errorf("started over at %v, not from where it was", at)
	}
}
