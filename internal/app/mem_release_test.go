package app

import (
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// Browsing many playlists keeps the ones shown last, the one showing, the
// liked songs and one loading, and lets the others go.
func TestTrimPageCachesKeepsRecentAndNeeded(t *testing.T) {
	a := newTestApp()
	a.playlistAt = map[int64]time.Time{}
	a.playlists[1] = &netease.Playlist{ID: 1, SpecialType: 5} // liked, never shown
	a.loadingPl[2] = true
	a.playlists[2] = &netease.Playlist{ID: 2}
	for id := int64(10); id < 10+maxCachedPlaylists+20; id++ {
		visit(a, route{page: pagePlaylist, id: id})
		a.playlists[id] = &netease.Playlist{ID: id}
		a.playlistAt[id] = time.Now()
		time.Sleep(time.Microsecond)
	}
	// The last one shown is put in after its show: once more to trim.
	a.trimPageCaches()
	if len(a.playlists) > maxCachedPlaylists {
		t.Fatalf("kept %d playlists, want at most %d", len(a.playlists), maxCachedPlaylists)
	}
	last := int64(10 + maxCachedPlaylists + 19)
	for _, id := range []int64{1, 2, last, last - 1, last - 5} {
		if a.playlists[id] == nil {
			t.Errorf("playlist %d let go", id)
		}
	}
	if a.playlists[10] != nil || a.playlists[11] != nil {
		t.Error("the playlists shown first were kept")
	}
	for id := range a.playlistAt {
		if a.playlists[id] == nil {
			t.Errorf("when playlist %d loaded kept after it went", id)
		}
	}
	for r := range a.pageSeen {
		if r.page == pagePlaylist && a.playlists[r.id] == nil && r != a.route {
			t.Errorf("seen time of %v kept after it went", r)
		}
	}
}

// Searches go the same way, the one showing and the one the sidebar
// goes back to staying.
func TestTrimSearchesKeepsShowing(t *testing.T) {
	a := newTestApp()
	var first route
	for i := range maxCachedSearches + 10 {
		r := route{page: pageSearch, query: string(rune('a' + i)), kind: netease.SearchSongs}
		if i == 0 {
			first = r
		}
		visit(a, r)
		a.searched = r
		a.searches[r] = &searchData{}
	}
	a.trimPageCaches()
	if len(a.searches) > maxCachedSearches {
		t.Fatalf("kept %d searches", len(a.searches))
	}
	if a.searches[a.route] == nil {
		t.Error("the search showing went")
	}
	if a.searches[first] != nil {
		t.Error("the first search stayed")
	}
}

// Trimming navigation history keeps each retained page's saved scroll paired with it.
func TestHistoryBounded(t *testing.T) {
	a := newTestApp()
	for id := range int64(maxNavHistory + 30) {
		a.pageScroll.Y = float32(id + 1)
		a.pushHistory(route{page: pageArtist, id: id + 1})
	}
	if len(a.history) != maxNavHistory {
		t.Fatalf("history has %d pages, want %d", len(a.history), maxNavHistory)
	}
	if got := a.history[len(a.history)-1]; got.id != maxNavHistory+30 {
		t.Fatalf("the last page left is %v", got)
	}
	if len(a.historyScroll) != len(a.history) {
		t.Fatal("scroll history was not bounded with routes")
	}
	for i, r := range a.history {
		if a.historyScroll[i] != float32(r.id) {
			t.Fatalf("route %d paired with scroll %v", r.id, a.historyScroll[i])
		}
	}
}

func TestAutoCacheBounded(t *testing.T) {
	a := newTestApp()
	a.auto.cache = map[int64][2][]string{}
	a.auto.song = 7
	for id := range int64(maxAutoTagCache + 1) {
		a.auto.cache[id] = [2][]string{}
	}
	a.trimAutoCache()
	if n := len(a.auto.cache); n > maxAutoTagCache*3/4 {
		t.Fatalf("kept %d songs' tags", n)
	}
	if _, ok := a.auto.cache[7]; !ok {
		t.Fatal("the playing song's tags went")
	}
}

// A cover put in again counts once, and trim keeps those asked for last.
func TestArtCacheReaddAndTrim(t *testing.T) {
	c := newArtCache(func(func()) {})
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	c.add("x", &artwork{size: 100, used: now})
	c.add("x", &artwork{size: 100, used: now})
	if c.size != 100 {
		t.Fatalf("size %d after the same cover twice, want 100", c.size)
	}
	for i := range 10 {
		c.add(string(rune('a'+i)), &artwork{size: 100, used: now.Add(time.Duration(i+1) * time.Second)})
	}
	c.trim(300)
	if c.size > 300 {
		t.Fatalf("size %d after trim to 300", c.size)
	}
	for _, k := range []string{"h", "i", "j"} {
		if c.art[k] == nil {
			t.Errorf("cover %s asked for last went", k)
		}
	}
	if c.art["x"] != nil {
		t.Error("the cover asked for first stayed")
	}
}

// A release asked for when nothing much would go, or soon after the last,
// is let be; one for a view gone always runs.
func TestReleaseWorth(t *testing.T) {
	a := newTestApp()
	a.releaseAfter(pageDelay, 1<<62)
	a.releaseDue()
	if a.mem.count != 0 || a.mem.pending {
		t.Fatalf("a release worth nothing ran: count %d pending %v", a.mem.count, a.mem.pending)
	}
	// A blur's release, then a view going before it runs: it runs.
	a.releaseAfter(blurDelay, 1<<62)
	a.releaseSoon()
	if a.mem.worth != 0 {
		t.Fatalf("worth %d after releaseSoon, want 0", a.mem.worth)
	}
	a.releaseDue()
	if a.mem.count != 1 {
		t.Fatalf("count %d, want 1", a.mem.count)
	}
	// Soon after: a page change's is let be, however much would go.
	a.releaseAfter(pageDelay, 1)
	a.releaseDue()
	if a.mem.count != 1 {
		t.Fatal("a page change's release ran within eventGap of the last")
	}
	// Now Playing moving holds it until it stops.
	a.mem.last = time.Time{}
	a.npK = 0.5
	a.releaseSoon()
	a.releaseDue()
	if a.mem.count != 1 || !a.mem.pending {
		t.Fatal("released while Now Playing moved")
	}
	a.npK = 0
	a.releaseDue()
	if a.mem.count != 2 {
		t.Fatal("not released once Now Playing stopped")
	}
}

func TestQuietCheckWaitsGap(t *testing.T) {
	a := newTestApp()
	a.mem.last = time.Now()
	a.quietCheck()
	if a.mem.count != 0 {
		t.Fatal("quietCheck ran within quietGap")
	}
}

// Hidden to the tray, the covers beyond hiddenArtLimit go, those asked for
// last staying.
func TestReleaseHiddenTrimsArt(t *testing.T) {
	a := newTestApp()
	now := time.Now()
	for i := range 40 {
		a.art.add(string(rune('A'+i)), &artwork{size: 1 << 20, used: now.Add(time.Duration(i) * time.Second)})
	}
	before := a.art.size
	a.releaseHidden(false)
	if a.art.size != before {
		t.Fatalf("minimized: covers %d MB, want %d as before", a.art.size>>20, before>>20)
	}
	a.releaseHidden(true)
	if a.art.size > hiddenArtLimit {
		t.Fatalf("hidden: covers %d MB, want at most %d", a.art.size>>20, hiddenArtLimit>>20)
	}
	if a.art.art[string(rune('A'+39))] == nil {
		t.Fatal("the cover asked for last went")
	}
	if !a.mem.pending || a.mem.worth != 0 {
		t.Fatal("hiding asked for no release")
	}
}

// visit shows r as show does, without loading what it shows.
func visit(a *app, r route) {
	a.route = r
	a.seePage(r)
	a.trimPageCaches()
	time.Sleep(time.Microsecond)
}
