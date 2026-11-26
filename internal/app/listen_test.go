package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// Tests keep no ledger on disk, never the developer's: one that wants it
// gives listenDir a folder of its own.
func init() {
	listenDir = func() (string, error) { return "", errors.New("tests keep no ledger") }
}

// withPlays gives a the ledger plays, kept since long ago.
func withPlays(a *app, plays ...listenEntry) {
	sortPlays(plays)
	a.listen = listenState{uid: a.listenAccount(), loaded: true, since: 1, plays: plays, gen: 1}
}

// playAt is a play of t at, heard secs, on dev ("" for Cloudy).
func playAt(t netease.Track, at time.Time, secs int, dev string) listenEntry {
	return listenEntry{Track: t, At: at.UnixMilli(), Secs: secs, Dev: dev}
}

// The weeks are NetEase's: Monday to Sunday, turning at Beijing's midnight,
// whatever the clock of the computer.
func TestListenWeekIsBeijings(t *testing.T) {
	la, _ := time.LoadLocation("America/Los_Angeles")
	// Sunday 07:59 in Los Angeles is Sunday 23:59 in Beijing: the week of
	// Monday 11-16. A minute later it is Monday in Beijing.
	sun := time.Date(2026, 11, 22, 7, 59, 0, 0, la)
	if got := weekStartOf(sun); !got.Equal(time.Date(2026, 11, 16, 0, 0, 0, 0, beijing)) {
		t.Fatalf("week of %v: %v", sun, got)
	}
	if got := weekStartOf(sun.Add(time.Minute)); !got.Equal(time.Date(2026, 11, 23, 0, 0, 0, 0, beijing)) {
		t.Fatalf("a minute later: %v", got)
	}
	if weekRange(weekStartOf(sun)) != "11.16 – 11.22" || weekdayOf(sun) != 6 || weekdayOf(sun.Add(time.Minute)) != 0 {
		t.Fatalf("%q %d", weekRange(weekStartOf(sun)), weekdayOf(sun))
	}
	if weekLength(380*60) != "6 小时 20 分" || weekLength(35*60) != "35 分钟" || weekLength(120*60) != "2 小时" {
		t.Fatal(weekLength(380 * 60))
	}
}

// What NetEase's recent plays add: the other devices' plays, once each;
// not Cloudy's own, which it told NetEase of as it ended them.
func TestListenFillsInOtherDevices(t *testing.T) {
	song := netease.Track{ID: 7, Name: "Slow Motion", Duration: 205000}
	other := netease.Track{ID: 8, Name: "風になる", Duration: 249000}
	start := time.Date(2026, 11, 24, 12, 0, 0, 0, beijing)
	ours := playAt(song, start, 205, "")
	recent := []netease.Play{
		// Cloudy's own, told as it ended.
		{Track: song, At: start.Add(205 * time.Second), Device: "osx"},
		// The same song on the phone the next day.
		{Track: song, At: start.Add(24 * time.Hour), Device: "android"},
		{Track: other, At: start.Add(-time.Hour), Device: "pc"},
		// An episode is not a song.
		{Track: netease.Track{ID: 9, ProgramID: 3}, At: start, Device: "iphone"},
	}
	added := newPlays([]listenEntry{ours}, recent)
	if len(added) != 2 || added[0].Dev != "android" || added[0].Secs != 205 || added[1].Track.ID != 8 {
		t.Fatalf("added %+v", added)
	}
	// Asked again, nothing is new.
	if again := newPlays(append([]listenEntry{ours}, added...), recent); len(again) != 0 {
		t.Fatalf("again %+v", again)
	}
}

// A week's report: how long, the songs most played first, each day, the
// singers, where it was played, and the song played most in a day.
func TestListenWeekReport(t *testing.T) {
	a := newTestApp()
	a.profile = &netease.Profile{UserID: 1}
	ar := func(name string) []netease.Artist { return []netease.Artist{{ID: int64(len(name)), Name: name}} }
	x := netease.Track{ID: 1, Name: "Crucified", Artists: ar("Army of Lovers"), Album: netease.Album{ID: 11, Name: "Big Battle of Egos"}, Duration: 213000}
	y := netease.Track{ID: 2, Name: "Cruel Summer", Artists: ar("Taylor Swift"), Album: netease.Album{ID: 12, Name: "Lover"}, Duration: 178000}
	mon := time.Date(2026, 11, 23, 0, 0, 0, 0, beijing)
	withPlays(a,
		playAt(x, mon.Add(9*time.Hour), 213, ""),
		playAt(x, mon.Add(10*time.Hour), 213, ""),
		playAt(x, mon.Add(26*time.Hour), 100, "android"),
		playAt(y, mon.Add(30*time.Hour), 178, "iphone"),
		// The Sunday before, and the Monday after: other weeks.
		playAt(y, mon.Add(-time.Minute), 178, ""),
		playAt(y, mon.AddDate(0, 0, 7), 178, ""),
	)
	// The Sunday before comes first.
	a.listen.plays[1].Src, a.listen.plays[2].Src = "daily", "playlist"
	r := a.report(mon)
	if r.count != 4 || r.secs != 213+213+100+178 || len(r.songs) != 2 || r.songs[0].Track.ID != 1 || r.songs[0].Plays != 3 {
		t.Fatalf("report %+v", r)
	}
	if r.days[0] != 426 || r.days[1] != 278 || r.phone != 2 || r.computer != 2 {
		t.Fatalf("days %v, phone %d, computer %d", r.days, r.phone, r.computer)
	}
	if r.artists[0].Name != "Army of Lovers" || r.artists[0].Plays != 3 || r.albums[0].Name != "Big Battle of Egos" {
		t.Fatalf("artists %+v albums %+v", r.artists, r.albums)
	}
	if r.loop.Track.ID != 1 || r.loop.Plays != 2 || !r.loopDay.Equal(mon) {
		t.Fatalf("loop %+v on %v", r.loop, r.loopDay)
	}
	if len(r.sources) != 2 || r.sources[0].Plays != 1 {
		t.Fatalf("sources %+v", r.sources)
	}
	// Each week once: the one before has the Sunday's play only.
	if before := a.report(mon.AddDate(0, 0, -7)); before.count != 1 || a.report(mon) != r {
		t.Fatalf("week before %d", before.count)
	}
	// A new play is worked out at once.
	a.keepPlay(playAt(y, mon.Add(40*time.Hour), 178, ""))
	if r2 := a.report(mon); r2.count != 5 || r2 == r {
		t.Fatalf("after a play: %d", r2.count)
	}
	days := listenDays(a.listen.plays)
	if n, from := streakTo(days, mon.Add(41*time.Hour)); n != 3 || !from.Equal(mon.AddDate(0, 0, -1)) {
		t.Fatalf("streak %d from %v", n, from)
	}
	if longestStreak(days) != 3 {
		t.Fatalf("longest %d", longestStreak(days))
	}
}

// A song heard in Cloudy goes in the ledger, which keeps it on disk by
// week, per account; one skipped before half a minute does not.
func TestListenKeepsPlays(t *testing.T) {
	dir := t.TempDir()
	old := listenDir
	listenDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { listenDir = old })
	a := newTestApp()
	a.profile = &netease.Profile{UserID: 42}
	song := netease.Track{ID: 5, Name: "英雄", Duration: 202000}
	now := time.Date(2026, 11, 25, 20, 0, 0, 0, beijing)
	a.listen.src = "search"
	a.keepListened(song, 20*time.Second, false, now)
	a.keepListened(song, 90*time.Second, false, now)
	a.keepListened(song, 202*time.Second, true, now.Add(time.Hour))
	a.keepListened(netease.Track{ID: 6, ProgramID: 9, Duration: 600000}, time.Minute, true, now)
	l := a.ledger()
	if len(l.plays) != 2 || l.plays[0].Secs != 90 || l.plays[1].Secs != 202 || l.plays[0].Src != "search" {
		t.Fatalf("plays %+v", l.plays)
	}
	// The writes run off the main thread.
	file := filepath.Join(dir, "42", "week-2026-11-23.json")
	deadline := time.Now().Add(5 * time.Second)
	for {
		plays, since := readLedger(42)
		if len(plays) == 2 && since > 0 {
			break
		}
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(file)
			t.Fatalf("on disk %d plays, since %d: %s", len(plays), since, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Another account has its own.
	a.profile = &netease.Profile{UserID: 43}
	if l := a.ledger(); l.uid != 43 || len(l.plays) != 0 {
		t.Fatalf("account 43: %+v", l.plays)
	}
}
