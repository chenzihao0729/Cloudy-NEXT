package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/player"
)

// cacheFolderName is the folder used when the listener chooses a parent
// directory for the cache. The default is beside the installed application.
const cacheFolderName = "Cache"

// songCacheDir is where the songs played are kept: in the folder the
// listener chose, or else in Cache beside the installed application.
func songCacheDir(folder string) string {
	if folder != "" {
		return filepath.Join(folder, cacheFolderName)
	}
	return filepath.Join(defaultAppFolder(), cacheFolderName)
}

// newSongCache returns the cache of songs played, in songCacheDir, or nil
// without one.
func newSongCache(folder string, mb int) *player.Cache {
	dir := songCacheDir(folder)
	if dir == "" {
		return nil
	}
	return player.NewCache(dir, int64(mb)<<20)
}

// openLoudness opens what the player measured of how loud songs are, kept
// in loudness.json in the user data directory: a few bytes a song, which
// clearing the cache leaves, so they need not be measured again.
func openLoudness() *player.Loudness {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return player.OpenLoudness("")
	}
	return player.OpenLoudness(filepath.Join(dir, "loudness.json"))
}

// pickCacheFolder asks for the folder to keep the songs played in.
func (a *app) pickCacheFolder() {
	start := filepath.Dir(songCacheDir(a.settings.CacheFolder))
	go func() {
		dir, err := chooseFolder(a.win, "选择歌曲缓存的位置", start)
		if err != nil || dir == "" {
			return
		}
		a.post(func() { a.setCacheFolder(dir) })
	}()
}

// setCacheFolder keeps the songs played in dir from now, in a folder of
// their own there; "" goes back to the app's cache directory. What was kept
// where they were is dropped.
func (a *app) setCacheFolder(dir string) {
	dir = strings.Trim(strings.TrimSpace(dir), `"`)
	if dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		dir = filepath.Clean(dir)
		if strings.EqualFold(filepath.Base(dir), cacheFolderName) {
			dir = filepath.Dir(dir) // the cache's own folder, chosen again
		}
	}
	if dir == a.settings.CacheFolder {
		return
	}
	if dir != "" {
		if err := os.MkdirAll(filepath.Join(dir, cacheFolderName), 0o700); err != nil {
			a.toast = "无法在这里保存缓存，请换一个文件夹"
			a.redraw()
			return
		}
	}
	a.settings.CacheFolder = dir
	a.changed()
	if cache := a.pl.Cache; cache != nil {
		to := songCacheDir(dir)
		a.cacheUsed = 0
		go func() {
			cache.SetDir(to)
			a.measureCache()
		}()
	}
	// The pictures go along, into a folder of their own beside the songs'.
	if disk := a.art.disk; disk != nil {
		to := otherCacheDir(dir)
		a.otherUsed = 0
		go func() {
			disk.SetDir(to)
			a.measureOtherCache()
		}()
	}
	if dir == "" {
		a.toast = "歌曲缓存已恢复到默认位置"
	} else {
		a.toast = "歌曲缓存已移到新位置，原来的缓存已清除"
	}
	a.redraw()
}

// openCacheFolder shows the folder the songs played are kept in.
func (a *app) openCacheFolder() {
	dir := songCacheDir(a.settings.CacheFolder)
	err := os.MkdirAll(dir, 0o700)
	if err == nil {
		err = mygo.Shell.OpenPath(dir)
	}
	if err != nil {
		a.toast = "无法打开缓存文件夹"
	}
}

// applyCacheLimit gives the cache the limit the settings say.
func (a *app) applyCacheLimit() {
	if cache := a.pl.Cache; cache != nil {
		limit := int64(a.settings.CacheMB) << 20
		go func() {
			cache.SetLimit(limit)
			a.measureCache()
		}()
	}
}

// measureCache finds how much the cache takes, on a goroutine, for the
// settings to show.
func (a *app) measureCache() {
	cache := a.pl.Cache
	if cache == nil {
		return
	}
	go func() {
		n := cache.Size()
		a.post(func() {
			a.cacheUsed = n
			if a.settingsView.open {
				a.redraw()
			}
		})
	}()
}

// applyOtherCacheLimit gives the pictures' pool the limit the settings say.
func (a *app) applyOtherCacheLimit() {
	if disk := a.art.disk; disk != nil {
		limit := int64(a.settings.OtherCacheMB) << 20
		go func() {
			disk.SetLimit(limit)
			a.measureOtherCache()
		}()
	}
}

// measureOtherCache finds how much the pictures' pool takes, on a
// goroutine, for the settings to show.
func (a *app) measureOtherCache() {
	disk := a.art.disk
	if disk == nil {
		return
	}
	go func() {
		n := disk.Size()
		a.post(func() {
			a.otherUsed = n
			if a.settingsView.open {
				a.redraw()
			}
		})
	}()
}

// clearOtherCache drops the pictures kept on disk; those shown stay.
func (a *app) clearOtherCache() {
	if disk := a.art.disk; disk != nil {
		a.otherUsed = 0
		go func() {
			disk.Clear()
			a.measureOtherCache()
		}()
	}
}

func (a *app) clearCache() {
	if cache := a.pl.Cache; cache != nil {
		a.cacheUsed = 0
		go func() {
			cache.Clear()
			a.measureCache()
		}()
	}
}

// cacheUsage says how much the cache takes, once measured.
func (a *app) cacheUsage() string { return usage(a.cacheUsed) }

// usage says how much n bytes of a cache are, -1 not yet measured.
func usage(n int64) string {
	switch {
	case n < 0:
		return "正在计算已用空间…"
	case n >= 1<<30:
		return fmt.Sprintf("已用 %.1f GB", float64(n)/(1<<30))
	default:
		return fmt.Sprintf("已用 %d MB", n>>20)
	}
}
