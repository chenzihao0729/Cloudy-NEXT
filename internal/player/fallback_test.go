package player

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// External search songs must never pass their synthetic ID to NetEase's playback API.
func TestExternalSongBypassesNetEase(t *testing.T) {
	old := songURL
	defer func() { songURL = old }()
	songURL = func(context.Context, int64, string) (*netease.Stream, error) {
		t.Fatal("external ID reached NetEase")
		return nil, nil
	}
	track := netease.Track{ID: -99, Source: &netease.SourceTrack{Platform: "tx", ID: "mid"}}
	p := &Player{Fallback: func(_ context.Context, tr netease.Track, _ string, _ []string) (*netease.Stream, error) {
		if tr.Source.ID != "mid" {
			t.Fatal("missing provider identity")
		}
		return &netease.Stream{Source: "qq", SourceID: "mid"}, nil
	}}
	st, err := p.streamInfo(context.Background(), track, "exhigh")
	if err != nil || st.SourceID != "mid" {
		t.Fatalf("%+v %v", st, err)
	}
	p.Fallback = nil
	if _, err := p.streamInfo(context.Background(), track, "exhigh"); err == nil {
		t.Fatal("external song without resolver succeeded")
	}
}

// An unavailable track uses the configured source only when account policy prefers it.
func TestFallbackOnlyWhenSelectedByPolicy(t *testing.T) {
	old := songURL
	t.Cleanup(func() { songURL = old })
	track := netease.Track{ID: 4}
	called := 0
	p := &Player{PreferFallback: func(netease.Track) bool { return false }, Fallback: func(context.Context, netease.Track, string, []string) (*netease.Stream, error) {
		called++
		return &netease.Stream{URL: "https://audio.example/track", Type: "mp3", Level: "exhigh", Source: "qq"}, nil
	}}
	for _, err := range []error{errors.New("network timeout"), netease.ErrUnavailable} {
		songURL = func(context.Context, int64, string) (*netease.Stream, error) { return nil, err }
		_, gotErr := p.streamInfo(context.Background(), track, "exhigh")
		if !errors.Is(gotErr, err) {
			t.Fatalf("official playback error changed to %v", gotErr)
		}
	}
	if called != 0 {
		t.Fatalf("fallback called %d times, want none", called)
	}
}

// Playback retries a 403 track with the failed source excluded from plugin resolution.
func TestForbiddenPluginSourceIsExcludedOnRetry(t *testing.T) {
	track := netease.Track{ID: 44, Name: "song"}
	got := make(chan []string, 1)
	p := &Player{
		post:  func(fn func()) { fn() },
		queue: []netease.Track{track}, order: []int{0}, at: 0, track: &track, playing: true,
		PreferFallback: func(netease.Track) bool { return true },
		Fallback: func(_ context.Context, _ netease.Track, _ string, excluded []string) (*netease.Stream, error) {
			got <- append([]string(nil), excluded...)
			return nil, netease.ErrUnavailable
		},
	}
	p.opened(p.gen, track, nil, nil, &sourceHTTPError{source: "qq", err: &streamHTTPError{code: http.StatusForbidden, status: "403 Forbidden"}}, 0, 1, 0)
	select {
	case excluded := <-got:
		if len(excluded) != 1 || excluded[0] != "qq" {
			t.Fatalf("excluded sources = %v, want [qq]", excluded)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("403 did not trigger another source resolution")
	}
	p.stop()
}

// A 403 from a selected plugin URL is tagged with its source so playback can resolve the next platform.
func TestPluginForbiddenCarriesSourceForFailover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Forbidden", http.StatusForbidden)
	}))
	defer server.Close()
	p := &Player{PreferFallback: func(netease.Track) bool { return true }, Fallback: func(context.Context, netease.Track, string, []string) (*netease.Stream, error) {
		return &netease.Stream{URL: server.URL, Type: "mp3", Level: "exhigh", Source: "qq"}, nil
	}}
	_, _, err := p.openContextPreferred(context.Background(), netease.Track{ID: 12}, "exhigh", true, nil)
	var sourceErr *sourceHTTPError
	var status *streamHTTPError
	if !errors.As(err, &sourceErr) || sourceErr.source != "qq" || !errors.As(err, &status) || status.code != http.StatusForbidden {
		t.Fatalf("403 error = %#v; want source-tagged QQ failure", err)
	}
}

// A selected source is preferred before NetEase only for tracks the policy selects.
func TestPreferFallbackBeforeNetEase(t *testing.T) {
	old := songURL
	t.Cleanup(func() { songURL = old })
	neteaseCalls, fallbackCalls := 0, 0
	songURL = func(context.Context, int64, string) (*netease.Stream, error) {
		neteaseCalls++
		return &netease.Stream{URL: "https://netease.example/song", Type: "mp3", Level: "exhigh"}, nil
	}
	p := &Player{
		PreferFallback: func(track netease.Track) bool { return track.ID == 1 },
		Fallback: func(context.Context, netease.Track, string, []string) (*netease.Stream, error) {
			fallbackCalls++
			return &netease.Stream{URL: "https://source.example/song", Type: "mp3", Level: "exhigh", Source: "qq"}, nil
		},
	}
	got, err := p.streamInfo(context.Background(), netease.Track{ID: 1}, "exhigh")
	if err != nil || got == nil || got.Source != "qq" || neteaseCalls != 0 {
		t.Fatalf("preferred stream = %#v, %v; NetEase calls %d", got, err, neteaseCalls)
	}
	got, err = p.streamInfo(context.Background(), netease.Track{ID: 2}, "exhigh")
	if err != nil || got == nil || got.Source != "" || neteaseCalls != 1 || fallbackCalls != 1 {
		t.Fatalf("normal stream = %#v, %v; NetEase calls %d, fallback calls %d", got, err, neteaseCalls, fallbackCalls)
	}
}

// An explicit NetEase no-copyright response falls through to the installed source.
func TestNoCopyrightFallsBackToSource(t *testing.T) {
	old := songURL
	t.Cleanup(func() { songURL = old })
	songURL = func(context.Context, int64, string) (*netease.Stream, error) {
		return nil, netease.ErrNoCopyright
	}
	called := 0
	p := &Player{Fallback: func(context.Context, netease.Track, string, []string) (*netease.Stream, error) {
		called++
		return &netease.Stream{URL: "https://source.example/song", Type: "mp3", Source: "kg"}, nil
	}}
	got, err := p.streamInfo(context.Background(), netease.Track{ID: 8}, "exhigh")
	if err != nil || got == nil || got.Source != "kg" || called != 1 {
		t.Fatalf("fallback = %#v, %v, called %d", got, err, called)
	}
}
