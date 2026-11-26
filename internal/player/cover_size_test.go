package player

import (
	"testing"

	"netease-music-mygo/internal/netease"
)

// The system's media controls get the cover at the size chosen in 系统媒体
// 封面清晰度: 600 standard, 1080, or as NetEase keeps it (no param).
func TestCoverURLFollowsCoverSize(t *testing.T) {
	defer CoverSize.Store(CoverSize.Load())
	tr := netease.Track{Album: netease.Album{PicURL: "http://p1.music.126.net/x/1.jpg"}}
	for _, c := range []struct {
		n    int32
		want string
	}{
		{600, "https://p1.music.126.net/x/1.jpg?param=600y600"},
		{1080, "https://p1.music.126.net/x/1.jpg?param=1080y1080"},
		{0, "https://p1.music.126.net/x/1.jpg"},
	} {
		CoverSize.Store(c.n)
		if got := coverURL(tr); got != c.want {
			t.Errorf("CoverSize %d: %s, want %s", c.n, got, c.want)
		}
	}
}
