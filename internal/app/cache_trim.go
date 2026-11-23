package app

import (
	"slices"
	"time"

	"netease-music-mygo/internal/netease"
)

// How many pages of each kind the app keeps loaded, those shown least
// recently going first: browsing for hours would otherwise keep every
// playlist's songs, artist's catalog and search's results it opened, for
// as long as it runs. Going back to one let go loads it again.
const (
	maxCachedPlaylists = 48
	maxCachedArtists   = 24
	maxCachedAlbums    = 32
	maxCachedUsers     = 16
	maxCachedPodcasts  = 24
	maxCachedSearches  = 16
	maxNavHistory      = 64
	maxAutoTagCache    = 256
)

// pageKey is the page a cached entry is shown on, for pageSeen.
func pageKey(page pageKind, id int64) route { return route{page: page, id: id} }

// seePage notes that r shows now, for trimPageCaches to keep it longest.
func (a *app) seePage(r route) {
	if a.pageSeen == nil {
		a.pageSeen = map[route]time.Time{}
	}
	if r.page != pageSearch {
		r = pageKey(r.page, r.id)
	}
	a.pageSeen[r] = time.Now()
}

// trimMap drops the entries of m shown least recently until no more than
// limit remain, keeping those keep says to.
func trimMap[K comparable, V any](m map[K]V, limit int, seen func(K) time.Time, keep func(K, V) bool) {
	if len(m) <= limit {
		return
	}
	type entry struct {
		k K
		t time.Time
	}
	var drop []entry
	for k, v := range m {
		if !keep(k, v) {
			drop = append(drop, entry{k, seen(k)})
		}
	}
	// Oldest first; never shown counts as oldest.
	slices.SortFunc(drop, func(x, y entry) int { return x.t.Compare(y.t) })
	for _, e := range drop {
		if len(m) <= limit {
			break
		}
		delete(m, e.k)
	}
}

// trimPageCaches bounds the pages kept loaded, keeping the page showing,
// those loading, the liked songs, the signed-in user's page and the
// playlist being selected in.
func (a *app) trimPageCaches() {
	seen := func(page pageKind) func(int64) time.Time {
		return func(id int64) time.Time { return a.pageSeen[pageKey(page, id)] }
	}
	showing := func(page pageKind, id int64) bool { return a.route.page == page && a.route.id == id }
	before := len(a.playlists)
	trimMap(a.playlists, maxCachedPlaylists, seen(pagePlaylist), func(id int64, _ *netease.Playlist) bool {
		return showing(pagePlaylist, id) || a.isLikedPlaylist(id) || a.loadingPl[id] || a.batch.on && a.batch.playlist == id
	})
	if len(a.playlists) != before {
		for id := range a.playlistAt {
			if a.playlists[id] == nil {
				delete(a.playlistAt, id)
			}
		}
		for id := range a.plErr {
			if a.playlists[id] == nil && !showing(pagePlaylist, id) {
				delete(a.plErr, id)
			}
		}
	}
	trimMap(a.artists, maxCachedArtists, seen(pageArtist), func(id int64, d *artistData) bool {
		return showing(pageArtist, id) || d != nil && (d.loading || d.latestLoading)
	})
	trimMap(a.albums, maxCachedAlbums, seen(pageAlbum), func(id int64, d *albumData) bool {
		return showing(pageAlbum, id) || d != nil && d.loading
	})
	trimMap(a.users, maxCachedUsers, seen(pageUser), func(id int64, d *userData) bool {
		return showing(pageUser, id) || a.profile != nil && a.profile.UserID == id || d != nil && (d.loading > 0 || d.subsLoading > 0)
	})
	trimMap(a.podcasts, maxCachedPodcasts, seen(pagePodcast), func(id int64, d *podcastData) bool {
		return showing(pagePodcast, id) || d != nil && (d.loading || d.page.loading)
	})
	trimMap(a.searches, maxCachedSearches, func(r route) time.Time { return a.pageSeen[r] }, func(r route, d *searchData) bool {
		return r == a.route || r == a.searched || d != nil && d.page.loading
	})
	// And what was seen of the pages let go.
	for r := range a.pageSeen {
		var kept bool
		switch r.page {
		case pagePlaylist:
			kept = a.playlists[r.id] != nil
		case pageArtist:
			kept = a.artists[r.id] != nil
		case pageAlbum:
			kept = a.albums[r.id] != nil
		case pageUser:
			kept = a.users[r.id] != nil
		case pagePodcast:
			kept = a.podcasts[r.id] != nil
		case pageSearch:
			kept = a.searches[r] != nil
		}
		if !kept && r != a.route && r != pageKey(a.route.page, a.route.id) {
			delete(a.pageSeen, r)
		}
	}
}

// pushHistory adds the page left to the back history, which keeps only
// the last maxNavHistory.
func (a *app) pushHistory(r route) {
	a.history = append(a.history, r)
	a.historyScroll = append(a.historyScroll, a.pageScroll.Y)
	if n := len(a.history) - maxNavHistory; n > 0 {
		a.history = append(a.history[:0], a.history[n:]...)
		a.historyScroll = append(a.historyScroll[:0], a.historyScroll[n:]...)
	}
}

// trimAutoCache keeps the tags read for choosing effects to
// maxAutoTagCache songs, down to three quarters once over.
func (a *app) trimAutoCache() {
	if len(a.auto.cache) <= maxAutoTagCache {
		return
	}
	for id := range a.auto.cache {
		if len(a.auto.cache) <= maxAutoTagCache*3/4 {
			break
		}
		if id != a.auto.song && id != a.auto.asked {
			delete(a.auto.cache, id)
		}
	}
}
