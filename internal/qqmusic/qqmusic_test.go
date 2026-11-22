package qqmusic

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// QQ search responses retain the identifiers and metadata needed by matching, lyrics and covers.
func TestSearchDecodesSongMetadata(t *testing.T) {
	old := client
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("w") != "晴天 周杰伦" || r.Header.Get("Referer") != "https://y.qq.com/" {
			t.Fatalf("search request = %s, headers=%v", r.URL, r.Header)
		}
		body := `{"code":0,"data":{"song":{"list":[{"songmid":"song-mid","media_mid":"media-mid","songname":"晴天","albumname":"叶惠美","albummid":"album-mid","interval":269,"singer":[{"name":"周杰伦"}]}]}}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	t.Cleanup(func() { client = old })
	songs, err := Search(context.Background(), "晴天 周杰伦", 10)
	if err != nil || len(songs) != 1 {
		t.Fatalf("Search() = %#v, %v", songs, err)
	}
	got := songs[0]
	if got.MID != "song-mid" || got.MediaMID != "media-mid" || got.Duration != 269*time.Second || got.CoverURL != "https://y.gtimg.cn/music/photo_new/T002R300x300M000album-mid.jpg" {
		t.Fatalf("decoded song = %#v", got)
	}
}
