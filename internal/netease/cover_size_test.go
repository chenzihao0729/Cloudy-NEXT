package netease

import (
	"net/url"
	"strings"
	"testing"
)

// Cover sizing preserves foreign CDN queries instead of appending a NetEase-only parameter.
func TestCoverSizingByProvider(t *testing.T) {
	foreign := "https://cdn.example/cover.jpg?token=abc&size=200"
	if got := Sized(foreign, 96); got != foreign {
		t.Fatalf("foreign query changed: %s", got)
	}
	qq := Sized("https://y.gtimg.cn/music/photo_new/T002R300x300M000album.jpg", 96)
	if !strings.Contains(qq, "T002R150x150") || strings.Contains(qq, "param=") {
		t.Fatal(qq)
	}
	wy := Sized("https://p1.music.126.net/cover.jpg?token=abc", 96)
	u, err := url.Parse(wy)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("token") != "abc" || u.Query().Get("param") != "96y96" {
		t.Fatal(wy)
	}
}
