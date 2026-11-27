package sourcecatalog

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"netease-music-mygo/internal/netease"
)

// Missing Kugou artwork is resolved by the recording's hash, preserving its identity.
func TestCoverUsesKugouRecordingHash(t *testing.T) {
	old := client
	defer func() { client = old }()
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("hash") != "exact-hash" {
			t.Fatalf("wrong recording: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"img":"https://cdn.example/{size}/cover.jpg"}}`))}, nil
	})}
	track := netease.Track{Source: &netease.SourceTrack{Platform: "kg", ID: "77", Hash: "exact-hash"}}
	got, err := Cover(context.Background(), track)
	if err != nil || got != "https://cdn.example/150/cover.jpg" {
		t.Fatalf("%q %v", got, err)
	}
}

// Cancellation stops missing-artwork fallback before it searches additional providers.
func TestCoverHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Cover(ctx, netease.Track{Source: &netease.SourceTrack{Platform: "tx", ID: "mid"}})
	if err != context.Canceled {
		t.Fatalf("%v", err)
	}
}

// A Kuwo search row with no artwork asks the recording-specific cover service
// and upgrades its URL so the UI can load it in a secure context.
func TestCoverUsesKuwoRecordingID(t *testing.T) {
	old := client
	defer func() { client = old }()
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "artistpicserver.kuwo.cn" || r.URL.Query().Get("rid") != "12345" {
			t.Fatalf("wrong Kuwo cover request: %s", r.URL)
		}
		body := `http://img1.kwcdn.kuwo.cn/star/albumcover/500/cover.jpg`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	track := netease.Track{Source: &netease.SourceTrack{Platform: "kw", ID: "12345"}}
	got, err := Cover(context.Background(), track)
	if err != nil || got != "https://img1.kwcdn.kuwo.cn/star/albumcover/500/cover.jpg" {
		t.Fatalf("Cover() = %q, %v", got, err)
	}
}
