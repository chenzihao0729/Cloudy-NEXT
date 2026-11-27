package app

import (
	"os"
	"path/filepath"
	"testing"

	"netease-music-mygo/internal/netease"
)

// An .lrc chosen for a song is kept as its lyrics; a file of no lyrics is
// refused, and a song of none has none.
func TestOwnLyricsKeptForTheSong(t *testing.T) {
	dir := t.TempDir()
	song := netease.Track{ID: 42, Name: "宝船"}
	good := filepath.Join(dir, "mine.lrc")
	os.WriteFile(good, []byte("[00:01.00]第一句\n[00:05.00]第二句\n"), 0o644)
	bad := filepath.Join(dir, "empty.txt")
	os.WriteFile(bad, []byte("  \n"), 0o644)
	keep := filepath.Join(dir, "own-lyrics")

	if _, ok := keepOwnLyrics(keep, song, bad); ok {
		t.Error("kept a file of no lyrics")
	}
	if lines := readOwnLyrics(keep, song); lines != nil {
		t.Fatalf("lyrics of its own before any was chosen: %v", lines)
	}
	if msg, ok := keepOwnLyrics(keep, song, good); !ok {
		t.Fatalf("not kept: %s", msg)
	}
	os.Remove(good) // the copy is what counts
	lines := readOwnLyrics(keep, song)
	if len(lines) != 2 || lines[0].Text != "第一句" {
		t.Fatalf("own lyrics read back as %+v", lines)
	}
	if other := readOwnLyrics(keep, netease.Track{ID: 43}); other != nil {
		t.Errorf("another song took them: %v", other)
	}
	local := netease.Track{LocalPath: `D:\音乐\a.flac`}
	if ownLyricsKey(local) == "" || ownLyricsKey(netease.Track{ID: 1, ProgramID: 9}) != "" {
		t.Error("a file of one's own has no key, or an episode has one")
	}
	// Cloudy-NEXT's external search results use negative synthetic IDs; their
	// real platform identity must still support a separate local LRC.
	external := netease.Track{ID: -1, Source: &netease.SourceTrack{Platform: "tx", ID: "qq-mid"}}
	if key := ownLyricsKey(external); key != "source:tx:qq-mid" {
		t.Errorf("external song key = %q", key)
	}
}

// Under each line on Now Playing shows the one or the other: its
// translation or how it reads, as the segments choose, and the bar's
// translation button turns reading off.
func TestLyricUnderIsOneOrTheOther(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	if a.lyricUnder() != 1 {
		t.Fatalf("by default %d, want the translation", a.lyricUnder())
	}
	a.setLyricUnder(2)
	if !a.settings.LyricRoman || a.settings.LyricTrans {
		t.Fatalf("音译 chosen: trans %v roman %v", a.settings.LyricTrans, a.settings.LyricRoman)
	}
	a.toggleLyricTrans()
	if !a.settings.LyricTrans || a.settings.LyricRoman || a.lyricUnder() != 1 {
		t.Errorf("the bar's button left trans %v roman %v", a.settings.LyricTrans, a.settings.LyricRoman)
	}
	a.setLyricUnder(0)
	if a.settings.LyricTrans || a.settings.LyricRoman {
		t.Error("原文 left something under the lines")
	}
}

// With no background, the desktop's translation is in the color of the
// words: white, it went into the white pages behind it.
func TestPlainDeskTranslationFollowsTheColor(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.DeskColor = "#1e88e5"
	look := a.deskLookOf("plain")
	if look.second.R == look.second.G && look.second.G == look.second.B {
		t.Errorf("the translation stays grey %v while the words are %v", look.second, look.sung)
	}
}

// Standing lyrics keep their whole capsule of tools on the screen, the
// lock at its end with it.
func TestStandingDeskKeepsItsWholeTools(t *testing.T) {
	a := newTestApp()
	k := a.deskKeep(true, deskVW)
	if k.W < 220 {
		t.Errorf("standing, %v of the window is kept: the tools are wider", k)
	}
}
