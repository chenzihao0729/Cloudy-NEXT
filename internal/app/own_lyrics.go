package app

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

// The listener's own lyrics (1.4.0, a viewer's ask): an .lrc file chosen
// for a song shows in place of NetEase's, on Now Playing, the desktop and
// everywhere lyrics show. A copy is kept beside the settings, so the file
// chosen may move or go.

// ownLyricsKey is what a song's own lyrics are kept under: its NetEase id,
// a file of the listener's own music by its path, or an external source's
// platform identity. The last form keeps the feature available for the
// multi-platform songs added by Cloudy-NEXT without colliding across sources.
func ownLyricsKey(t netease.Track) string {
	switch {
	case t.ProgramID != 0:
		return ""
	case t.ID > 0:
		return fmt.Sprintf("song:%d", t.ID)
	case t.LocalPath != "":
		return ownSongKey(t.LocalPath)
	case t.Source != nil && t.Source.Platform != "":
		id := t.Source.ID
		if id == "" {
			id = t.Source.Hash
		}
		if id != "" {
			return fmt.Sprintf("source:%s:%s", t.Source.Platform, id)
		}
	}
	return ""
}

// ownLyricsDir is where the copies are, beside the settings: none for
// settings never loaded, as in tests.
func (a *app) ownLyricsDir() string {
	if a.settings.path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(a.settings.path), "own-lyrics")
}

// ownLyricsPath is the copy of t's own lyrics in dir, there or not.
func ownLyricsPath(dir string, t netease.Track) string {
	key := ownLyricsKey(t)
	if dir == "" || key == "" {
		return ""
	}
	sum := sha1.Sum([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:10])+".lrc")
}

// readOwnLyrics returns the own lyrics of t kept in dir, if any.
func readOwnLyrics(dir string, t netease.Track) []lyrics.Line {
	path := ownLyricsPath(dir, t)
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return lyrics.FromText(string(b))
}

// useOwnLyrics has the player take a song's own lyrics before any other.
func (a *app) useOwnLyrics() {
	dir := a.ownLyricsDir()
	player.OwnLyrics = func(t netease.Track) []lyrics.Line { return readOwnLyrics(dir, t) }
}

// hasOwnLyrics reports whether lyrics of the listener's own are kept for t.
func (a *app) hasOwnLyrics(t netease.Track) bool {
	path := ownLyricsPath(a.ownLyricsDir(), t)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// chooseLyricsFile asks for an .lrc file; tests replace it.
var chooseLyricsFile = func(parent *mygo.Window) ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      parent,
		Title:       "选择歌词文件",
		ButtonLabel: "使用",
		Filters:     []mygo.FileFilter{{Name: "歌词", Extensions: []string{"lrc", "txt"}}},
	})
}

// ownLyricsActions are the song menu's rows for t's own lyrics: choosing a
// file, and once chosen, going back to the lyrics it had.
func (a *app) ownLyricsActions(t netease.Track) []menuAction {
	if ownLyricsKey(t) == "" || a.ownLyricsDir() == "" {
		return nil
	}
	acts := []menuAction{{label: "使用本地歌词文件", icon: iconLyrics, do: func() { a.pickOwnLyrics(t) }}}
	if a.hasOwnLyrics(t) {
		back := "恢复在线歌词"
		if t.LocalPath != "" {
			back = "移除自选歌词"
		}
		acts = append(acts, menuAction{label: back, icon: iconBack, do: func() { a.dropOwnLyrics(t) }})
	}
	return acts
}

// pickOwnLyrics asks for an .lrc file and keeps it as t's lyrics. The
// dialog waits for the main thread, which shows it: it is asked off it.
func (a *app) pickOwnLyrics(t netease.Track) {
	win, dir := a.win, a.ownLyricsDir()
	go func() {
		paths, err := chooseLyricsFile(win)
		if err != nil || len(paths) == 0 {
			return
		}
		msg, ok := keepOwnLyrics(dir, t, paths[0])
		a.post(func() {
			a.toast = msg
			if ok {
				a.lyricsChanged(t)
			}
			a.redraw()
		})
	}()
}

// keepOwnLyrics copies the lyrics at from into dir as t's, and says how
// that went.
func keepOwnLyrics(dir string, t netease.Track, from string) (string, bool) {
	b, err := os.ReadFile(from)
	if err != nil {
		return "读取歌词文件失败：" + err.Error(), false
	}
	if len(lyrics.FromText(string(b))) == 0 {
		return "这个文件里没有能识别的歌词", false
	}
	path := ownLyricsPath(dir, t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "保存歌词失败：" + err.Error(), false
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "保存歌词失败：" + err.Error(), false
	}
	return "已使用本地歌词：" + t.Name, true
}

// dropOwnLyrics puts t's own lyrics away: it shows its own again.
func (a *app) dropOwnLyrics(t netease.Track) {
	if path := ownLyricsPath(a.ownLyricsDir(), t); path != "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("own lyrics: %v", err)
		}
	}
	a.toast = "已恢复歌曲原来的歌词"
	a.lyricsChanged(t)
}

// lyricsChanged loads the lyrics of t again if it is what plays.
func (a *app) lyricsChanged(t netease.Track) {
	if cur := a.pl.Current(); cur != nil && ownLyricsKey(*cur) == ownLyricsKey(t) {
		a.pl.ReloadLyrics()
	}
}
