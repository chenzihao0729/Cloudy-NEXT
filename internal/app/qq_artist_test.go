package app

import (
	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/qqmusic"
	"netease-music-mygo/internal/sourcecatalog"
	"testing"
)

// Clicking a QQ artist routes by singer MID and Back restores the search route.
func TestQQArtistLinkAndBack(t *testing.T) {
	old := openQQSongArtist
	defer func() { openQQSongArtist = old }()
	openQQSongArtist = func(a *app, song netease.Track, ar netease.Artist) {
		a.openPage(route{page: pageQQArtist, sourceID: ar.SourceMID, query: ar.Name})
	}
	a := newTestApp()
	a.settings = defaultSettings()
	search := route{page: pageSearch, query: "晴天", kind: netease.SearchSongs}
	a.route = search
	a.searches[search] = &searchData{}
	a.qqArtists = map[string]*qqArtistData{"jay-mid": {page: paged{loaded: true}}}
	song := providerTrack("tx", sourcecatalog.Song{ID: "song", Name: "晴天", Artists: []string{"周杰伦"}, ArtistMIDs: []string{"jay-mid"}})
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		a.trackRows(c, []netease.Track{song}, true, func(*ui.Context) {})
	}, 1200, 800)
	tt.Frame()
	if err := tt.Click("周杰伦"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageQQArtist || a.route.sourceID != "jay-mid" {
		t.Fatalf("wrong artist route: %+v", a.route)
	}
	if len(a.history) == 0 || a.history[len(a.history)-1] != search {
		t.Fatal("search route was not preserved for Back")
	}
	if song.Artists[0].ID != 0 {
		t.Fatal("QQ singer ID assigned to NetEase namespace")
	}
}

// Selecting an album opens its in-app page and preserves the artist route.
// Search album clicks previously rendered as plain text for external tracks.
func TestSearchSourceAlbumClick(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.route = route{page: pageSearch, query: "歌曲"}
	a.qqAlbums = map[string]*qqAlbumData{"album-mid": {album: &qqmusic.Album{Name: "专辑"}}}
	song := providerTrack("tx", sourcecatalog.Song{ID: "song", Name: "歌曲", Artists: []string{"歌手"}, Album: "搜索专辑测试", AlbumMID: "album-mid"})
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		if a.route.page == pageSearch {
			a.trackRows(c, []netease.Track{song}, true, func(*ui.Context) {})
		}
	}, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索专辑测试"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageQQAlbum || a.route.sourceID != "album-mid" {
		t.Fatalf("wrong album route: %+v", a.route)
	}
	a.back()
	if a.route.page != pageSearch {
		t.Fatal("Back did not restore search")
	}
}

// Selecting an album opens its in-app page and preserves the artist route.
func TestQQArtistAlbumNavigation(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	r := route{page: pageQQArtist, sourceID: "singer", query: "歌手"}
	a.route = r
	a.qqArtists = map[string]*qqArtistData{"singer": {tab: 1, profileLoaded: true, page: paged{loaded: true}, profile: qqmusic.ArtistProfile{Albums: []qqmusic.ArtistAlbum{{MID: "album", Name: "测试专辑"}}}}}
	a.qqAlbums = map[string]*qqAlbumData{"album": {album: &qqmusic.Album{Name: "测试专辑"}}}
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		if a.route.page == pageQQArtist {
			a.qqArtistPage(c)
		}
	}, 1200, 800)
	tt.Frame()
	if err := tt.Click("测试专辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageQQAlbum || a.route.sourceID != "album" {
		t.Fatalf("wrong route: %+v", a.route)
	}
	if len(a.history) == 0 || a.history[len(a.history)-1] != r {
		t.Fatal("artist route not preserved")
	}
	a.back()
	if a.route != r {
		t.Fatalf("Back did not restore artist: %+v", a.route)
	}
}
