package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/sourcecatalog"
)

var findSearchCover = sourcecatalog.Cover

type savedSearchCover struct {
	URL string
	At  time.Time
}

func searchCoverCachePath(track netease.Track) string {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(track.Source.Platform + "\x00" + track.Source.ID + "\x00" + track.Source.Hash))
	return filepath.Join(dir, "search-covers", hex.EncodeToString(sum[:])+".json")
}

// Called only while a visible row builds; queued jobs are bounded, and all
// replies mutate the current search on the main thread only.
func (a *app) fillSearchCover(track netease.Track) {
	d := a.searches[a.route]
	if d == nil || d.multi == nil || track.Source == nil {
		return
	}
	m := d.multi
	if m.coverBusy == nil {
		m.coverBusy = map[int64]bool{}
		m.coverRetry = map[int64]time.Time{}
		m.coverSlots = make(chan struct{}, 3)
	}
	if m.coverBusy[track.ID] || time.Now().Before(m.coverRetry[track.ID]) {
		return
	}
	select {
	case m.coverSlots <- struct{}{}:
	default:
		return
	}
	m.coverBusy[track.ID] = true
	r := a.route
	go func() {
		defer func() { <-m.coverSlots }()
		ctx, cancel := context.WithTimeout(m.ctx, 8*time.Second)
		defer cancel()
		path := searchCoverCachePath(track)
		var cached savedSearchCover
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &cached)
		}
		cover := cached.URL
		if cached.At.IsZero() || time.Since(cached.At) > 7*24*time.Hour {
			cover, _ = findSearchCover(ctx, track)
			if ctx.Err() == nil && cover != "" && path != "" {
				if b, err := json.Marshal(savedSearchCover{URL: cover, At: time.Now()}); err == nil {
					if os.MkdirAll(filepath.Dir(path), 0700) == nil {
						_ = writeFileAtomic(path, b, 0600)
					}
				}
			}
		}
		a.post(func() {
			if a.searches[r] != d || m.ctx.Err() != nil {
				return
			}
			delete(m.coverBusy, track.ID)
			if cover == "" {
				m.coverRetry[track.ID] = time.Now().Add(time.Minute)
				return
			}
			for i := range d.res.Songs {
				if d.res.Songs[i].ID == track.ID && d.res.Songs[i].Album.PicURL == "" {
					d.res.Songs[i].Album.PicURL = cover
				}
			}
		})
	}()
}
