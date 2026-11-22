package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The side buttons of a mouse go back and forward between pages.
func TestBackAndForward(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tracks := make([]netease.Track, 60)
	for i := range tracks {
		tracks[i] = netease.Track{ID: int64(i + 1), Name: "一张相片"}
	}
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "飙升榜", Tracks: tracks}
	tt := ui.NewTester(a.view, 1200, 800)
	charts, list := route{page: pageCharts}, route{page: pagePlaylist, id: 42}
	a.open(charts)
	tt.Frame()
	a.pageScroll.Y = 140
	a.open(list)
	tt.Frame()
	a.pageScroll.Y = 300

	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route != charts {
		t.Fatalf("back went to %+v", a.route)
	}
	if a.pageScroll.Y != 140 {
		t.Fatalf("back restored scroll %v, want 140", a.pageScroll.Y)
	}
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != list {
		t.Fatalf("forward went to %+v", a.route)
	}
	if a.pageScroll.Y != 300 {
		t.Fatalf("forward restored scroll %v, want 300", a.pageScroll.Y)
	}

	// Back closes Now Playing before it leaves the page under it.
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一张相片"}})
	a.nowPlaying = true
	tt.Frame()
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.nowPlaying || a.route != list {
		t.Fatalf("back with Now Playing open: open %v, page %+v", a.nowPlaying, a.route)
	}

	// A new page leaves nothing to go forward to.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	a.open(route{page: pageHome})
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != (route{page: pageHome}) {
		t.Fatalf("forward after a new page went to %+v", a.route)
	}
}

// Search lists must publish their actual viewport so artist navigation restores it on Back.
func TestSearchArtistBackRestoresScroll(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	r := route{page: pageSearch, query: "测试", kind: netease.SearchSongs}
	tracks := make([]netease.Track, 80)
	for i := range tracks {
		tracks[i] = netease.Track{ID: int64(i + 1), Name: "歌曲"}
	}
	a.searches[r] = &searchData{page: paged{loaded: true}, res: netease.SearchResult{Songs: tracks}}
	a.qqArtists = map[string]*qqArtistData{"singer": {page: paged{loaded: true}, profileLoaded: true}}
	a.open(r)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	a.trackList.ScrollTo(30, ui.Start)
	tt.Frame()
	tt.Frame()
	y := a.pageScroll.Y
	if y <= 0 {
		t.Fatal("list did not publish its scroll position")
	}
	a.openPage(route{page: pageQQArtist, sourceID: "singer", query: "歌手"})
	tt.Frame()
	a.back()
	tt.Frame()
	tt.Frame()
	if a.route != r || a.pageScroll.Y != y {
		t.Fatalf("restored %v, want %v", a.pageScroll.Y, y)
	}
}
