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
