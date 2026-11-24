package app

import (
	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
	"testing"
)

// External songs without a social adapter neither request NetEase counts nor expose comments.
func TestExternalSongHidesUnsupportedSocialData(t *testing.T) {
	a := newTestApp()
	// Also cover a malformed/imported external track with a positive numeric ID.
	tr := netease.Track{ID: 123, Source: &netease.SourceTrack{Platform: "tx", ID: "mid"}}
	tt := ui.NewTester(func(c *ui.Context) {
		a.countedHeart(c.Key("heart"), &tr, 16, ui.RGB(100, 100, 100), true)
		a.barCommentButton(c.Key("comments"), &tr, 16, ui.RGB(100, 100, 100), true)
	}, 400, 100)
	tt.Frame()
	if len(a.redCounts) != 0 || a.comments.loading {
		t.Fatal("external ID requested NetEase social data")
	}
	if tt.HasText("0") || commentable(&tr) {
		t.Fatal("unsupported counts/comments were exposed")
	}
	if _, ok := tt.Find("评论"); ok {
		t.Fatal("unsupported comment button shown")
	}
	if !commentable(&netease.Track{ID: 123}) {
		t.Fatal("NetEase comments disabled")
	}
}

// Switching source platforms hides comments and returns an open comments pane to lyrics.
func TestNowPlayingExternalCommentsSwitch(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.nowPlaying = true
	wy := netease.Track{ID: 123, Name: "网易云歌曲", Duration: 200000}
	external := netease.Track{ID: -123, Name: "QQ歌曲", Duration: 200000, Source: &netease.SourceTrack{Platform: "tx", ID: "mid"}}
	a.pl.Cue([]netease.Track{external})
	a.pl.SetLyrics(nil)
	a.npPane = paneComments
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if a.npPane == paneComments || a.comments.loading {
		t.Fatal("external song retained or loaded comments")
	}
	for _, tab := range a.npTabs() {
		if tab.pane == paneComments {
			t.Fatal("external comments tab exposed")
		}
	}
	a.pl.Cue([]netease.Track{wy})
	// v1.3.7 replaces the tab with a drawer opened by the comment button.
	a.comments = commentState{track: wy.ID, loaded: true}
	button := ui.NewTester(func(c *ui.Context) { a.commentCountButton(c, a.pl.Current(), 20, white) }, 200, 80)
	button.Frame()
	if err := button.Click("评论"); err != nil {
		t.Fatal(err)
	}
	button.Frame()
	if !a.npComments {
		t.Fatal("NetEase comment button did not open drawer")
	}
	a.pl.Cue([]netease.Track{external})
	tt.Frame()
	if a.npComments {
		t.Fatal("external track retained comment drawer")
	}
}
