package app

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
)

// 听歌账本: every song the listener plays in Cloudy, each time it is
// played, kept per account and per week in the user data directory; and
// what they play on their other devices, from NetEase's recent plays, filled
// in as Cloudy runs. The weeks are NetEase's: Monday to Sunday by Beijing's
// clock, its days turning at Beijing's midnight.
//
// NetEase's own week ranking is no help here: it is the last seven days and
// the day so far, never Monday to Sunday, and it drops the plays Cloudy
// tells it once Beijing's day has turned (backlog H5).

// listenEntry is one play of a song.
type listenEntry struct {
	Track netease.Track `json:"t"`
	// At is when it started, in Cloudy; for other devices, when NetEase
	// says they played it. Unix milliseconds.
	At int64 `json:"at"`
	// Secs is how long it was heard; for other devices, how long it is.
	Secs int `json:"s"`
	// Dev is the device NetEase names (android, iphone, pc, osx…), or ""
	// for Cloudy on this computer.
	Dev string `json:"d,omitempty"`
	// Src is where Cloudy played it from (listenSources), if known.
	Src string `json:"src,omitempty"`
}

func (e *listenEntry) at() time.Time { return time.UnixMilli(e.At) }

// listenState is the signed-in account's ledger, and NetEase's figures of
// all their listening.
type listenState struct {
	uid    int64
	loaded bool
	plays  []listenEntry // all of them, the earliest first
	// since is when Cloudy began keeping the account's plays, Unix ms;
	// before it there are only the last play of each song.
	since int64
	gen   int // changes with every play kept, for what is worked out from them

	syncAt  time.Time // when NetEase's recent plays were last asked for
	syncing bool
	ticking bool // a sync every listenSyncEvery is set up

	// src is where the queue playing came from, and srcGen the player's
	// queue it is of.
	src    string
	srcGen int

	// NetEase's own: the all-time ranking and the listening total.
	all        []netease.Ranked
	total      netease.ListenTotal
	allAt      time.Time
	allLoading bool

	reports map[int64]*weekReport // worked out, by week start, for gen
	repGen  int
}

const (
	// listenAfter is how long a song is heard before it counts, as NetEase
	// counts it.
	listenAfter = 30 * time.Second
	// listenSyncEvery is how often other devices' plays are filled in while
	// Cloudy runs.
	listenSyncEvery = 20 * time.Minute
	// listenSame is how close a play NetEase lists comes to one Cloudy kept
	// of the same song for both to be the one: Cloudy's own, told to it.
	listenSame = 15 * time.Minute
)

// weekStartOf is the Monday, Beijing's midnight, of the week t is in.
func weekStartOf(t time.Time) time.Time {
	b := t.In(beijing)
	y, m, d := b.Date()
	back := (int(b.Weekday()) + 6) % 7
	return time.Date(y, m, d-back, 0, 0, 0, 0, beijing)
}

// dayStartOf is Beijing's midnight starting the day t is in.
func dayStartOf(t time.Time) time.Time {
	y, m, d := t.In(beijing).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, beijing)
}

// listenDir is where the accounts' ledgers are kept; tests replace it.
var listenDir = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "listening"), nil
}

// listenAccount is whose ledger the plays go in: the signed-in account's,
// or nobody's (0).
func (a *app) listenAccount() int64 {
	if a.profile == nil {
		return 0
	}
	return a.profile.UserID
}

// ledger is the signed-in account's plays, read from disk the first time.
func (a *app) ledger() *listenState {
	l := &a.listen
	uid := a.listenAccount()
	if l.loaded && l.uid == uid {
		return l
	}
	src, srcGen, ticking := l.src, l.srcGen, l.ticking
	*l = listenState{uid: uid, loaded: true, src: src, srcGen: srcGen, ticking: ticking}
	plays, since := readLedger(uid)
	if since == 0 {
		// The first time for this account: what Cloudy remembers playing
		// here, the last play of each song, as a start.
		since = time.Now().UnixMilli()
		a.loadHistory()
		plays = seedLedger(a.recent.played)
		writeLedgerMeta(uid, since)
		if len(plays) > 0 {
			writeLedgerWeeks(uid, plays, weeksOf(plays))
		}
	}
	l.plays, l.since = plays, since
	l.gen++
	return l
}

// readLedger reads an account's plays, the earliest first, and when the
// ledger began, 0 if it has not.
func readLedger(uid int64) ([]listenEntry, int64) {
	dir, err := listenDir()
	if err != nil {
		return nil, 0
	}
	dir = filepath.Join(dir, strconv.FormatInt(uid, 10))
	var meta struct {
		Since int64 `json:"since"`
	}
	if b, err := os.ReadFile(filepath.Join(dir, "meta.json")); err == nil {
		json.Unmarshal(b, &meta)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "week-*.json"))
	var plays []listenEntry
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var week []listenEntry
		if json.Unmarshal(b, &week) == nil {
			plays = append(plays, week...)
		}
	}
	sortPlays(plays)
	return plays, meta.Since
}

func sortPlays(plays []listenEntry) {
	slices.SortStableFunc(plays, func(x, y listenEntry) int {
		switch {
		case x.At < y.At:
			return -1
		case x.At > y.At:
			return 1
		}
		return 0
	})
}

// seedLedger turns the songs played here, the latest first, each once and
// kept as they ended, into plays.
func seedLedger(played []playedSong) []listenEntry {
	var out []listenEntry
	for _, p := range played {
		if !countsAsSong(&p.Track) {
			continue
		}
		secs := int(p.Track.Duration / 1000)
		out = append(out, listenEntry{Track: keptTrack(p.Track), At: p.At - int64(secs)*1000, Secs: secs})
	}
	sortPlays(out)
	return out
}

// countsAsSong reports whether t is a song the ledger keeps: not a
// podcast's episode.
func countsAsSong(t *netease.Track) bool {
	return t.ProgramID == 0 && (t.ID > 0 || t.LocalPath != "")
}

// keptTrack is t as the ledger keeps it, without what changes or is long.
func keptTrack(t netease.Track) netease.Track {
	t.Privilege, t.Reason, t.Description = nil, "", ""
	return t
}

func weekFile(start time.Time) string {
	return "week-" + start.Format("2006-01-02") + ".json"
}

// weeksOf is the weeks plays are in.
func weeksOf(plays []listenEntry) map[int64]bool {
	weeks := map[int64]bool{}
	for i := range plays {
		weeks[weekStartOf(plays[i].at()).Unix()] = true
	}
	return weeks
}

// ledgerWrites keeps each file to the latest version written, writes
// running off the main thread.
var ledgerWrites struct {
	sync.Mutex
	gen  int64
	last map[string]int64
}

// writeLedgerWeeks writes the weeks of plays named, off the main thread.
func writeLedgerWeeks(uid int64, plays []listenEntry, weeks map[int64]bool) {
	dir, err := listenDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, strconv.FormatInt(uid, 10))
	byWeek := map[int64][]listenEntry{}
	for i := range plays {
		w := weekStartOf(plays[i].at()).Unix()
		if weeks[w] {
			byWeek[w] = append(byWeek[w], plays[i])
		}
	}
	for w := range weeks {
		b, err := json.Marshal(byWeek[w])
		if err != nil {
			continue
		}
		path := filepath.Join(dir, weekFile(time.Unix(w, 0).In(beijing)))
		ledgerWrites.Lock()
		ledgerWrites.gen++
		gen := ledgerWrites.gen
		ledgerWrites.Unlock()
		go func() {
			w := &ledgerWrites
			w.Lock()
			defer w.Unlock()
			if w.last == nil {
				w.last = map[string]int64{}
			}
			if gen <= w.last[path] {
				return
			}
			if err := writeFileAtomic(path, b, 0o600); err != nil {
				log.Printf("listening: %v", err)
				return
			}
			w.last[path] = gen
		}()
	}
}

func writeLedgerMeta(uid int64, since int64) {
	dir, err := listenDir()
	if err != nil {
		return
	}
	b, _ := json.Marshal(map[string]int64{"since": since})
	writeFileAtomic(filepath.Join(dir, strconv.FormatInt(uid, 10), "meta.json"), b, 0o600)
}

// keepPlay adds a play to the ledger, and writes its week.
func (a *app) keepPlay(e listenEntry) {
	l := a.ledger()
	l.plays = append(l.plays, e)
	if n := len(l.plays); n > 1 && l.plays[n-2].At > e.At {
		sortPlays(l.plays)
	}
	l.gen++
	writeLedgerWeeks(l.uid, l.plays, map[int64]bool{weekStartOf(e.at()).Unix(): true})
}

// keepListened keeps a song played in Cloudy, once heard long enough.
func (a *app) keepListened(t netease.Track, played time.Duration, ended bool, now time.Time) {
	if !countsAsSong(&t) || (!ended && played < listenAfter) {
		return
	}
	secs := int(played.Seconds())
	if ended && t.Duration > 0 {
		secs = int(t.Duration / 1000)
	}
	a.keepPlay(listenEntry{Track: keptTrack(t), At: now.Add(-time.Duration(secs) * time.Second).UnixMilli(), Secs: secs, Src: a.listen.src})
}

// Where the queue playing came from, as the ledger keeps it, and what the
// report calls it.
var listenSources = map[string]string{
	"fm":       "私人 FM",
	"heart":    "心动模式",
	"style":    "风格推荐",
	"daily":    "每日推荐",
	"playlist": "歌单",
	"liked":    "我喜欢",
	"album":    "专辑",
	"artist":   "歌手页",
	"search":   "搜索",
	"local":    "本地与云盘",
	"recent":   "最近播放",
	"week":     "听歌周报",
	"home":     "首页",
	"other":    "其他",
}

// noteQueueSource notes where a queue that just took the place of
// another came from: the radio it is, or the page it was played on.
func (a *app) noteQueueSource() {
	if a.pl == nil {
		return
	}
	gen := a.pl.RadioGeneration()
	l := &a.listen
	if gen == l.srcGen && l.src != "" {
		return
	}
	l.srcGen = gen
	l.src = a.queueSource()
}

func (a *app) queueSource() string {
	switch r := a.pl.Radio(); {
	case strings.HasPrefix(r, "fm:"):
		return "fm"
	case r == heartRadio:
		return "heart"
	case isRoam(r):
		return "style"
	case r != "":
		return "other"
	}
	if len(a.daily) > 0 && a.queueIs(a.daily) {
		return "daily"
	}
	switch a.route.page {
	case pageDaily, pageHistoryDaily:
		return "daily"
	case pagePlaylist:
		if pl := a.playlists[a.route.id]; pl != nil && pl.SpecialType == 5 {
			return "liked"
		}
		return "playlist"
	case pageAlbum:
		return "album"
	case pageArtist:
		return "artist"
	case pageSearch:
		return "search"
	case pageDownloads, pageLocal, pageCloud:
		return "local"
	case pageRecent:
		return "recent"
	case pageWeek:
		return "week"
	case pageStyle:
		return "style"
	case pageHome:
		return "home"
	}
	return "other"
}

// recentPlaysFor is NetEase's recent plays, asked for in a sync; tests
// replace it.
var recentPlaysFor = func() ([]netease.Play, error) { return netease.RecentPlays(1000) }

// syncListening fills in the plays of the listener's other devices from
// NetEase's recent plays: at once if now, else if the last was a while
// ago. It goes on every listenSyncEvery while Cloudy runs.
func (a *app) syncListening(now bool) {
	if a.profile == nil {
		return
	}
	l := a.ledger()
	if l.syncing || (!now && time.Since(l.syncAt) < listenSyncEvery) {
		return
	}
	if !l.ticking && a.win != nil {
		l.ticking = true
		var tick func()
		tick = func() {
			time.AfterFunc(listenSyncEvery, func() {
				a.post(func() { a.syncListening(false) })
				tick()
			})
		}
		tick()
	}
	l.syncing, l.syncAt = true, time.Now()
	uid, gen := l.uid, a.accountGen
	go func() {
		recent, err := recentPlaysFor()
		a.post(func() {
			l := &a.listen
			if gen != a.accountGen || l.uid != uid {
				return
			}
			l.syncing = false
			if err != nil {
				log.Printf("listening: recent plays: %v", err)
				return
			}
			a.mergeRecent(recent)
		})
	}()
}

// mergeRecent adds the plays recent lists that the ledger has not.
func (a *app) mergeRecent(recent []netease.Play) {
	l := a.ledger()
	added := newPlays(l.plays, recent)
	if len(added) == 0 {
		return
	}
	l.plays = append(l.plays, added...)
	sortPlays(l.plays)
	l.gen++
	writeLedgerWeeks(l.uid, l.plays, weeksOf(added))
}

// newPlays is the plays recent lists that are not in plays yet: neither
// filled in before, nor Cloudy's own, which it told NetEase of.
func newPlays(plays []listenEntry, recent []netease.Play) []listenEntry {
	type key struct{ id, at int64 }
	seen := map[key]bool{}
	ours := map[int64][]listenEntry{}
	for _, p := range plays {
		if p.Dev != "" {
			seen[key{p.Track.ID, p.At}] = true
		} else {
			ours[p.Track.ID] = append(ours[p.Track.ID], p)
		}
	}
	var out []listenEntry
	for _, r := range recent {
		t := r.Track
		if !countsAsSong(&t) || t.ID <= 0 {
			continue
		}
		at := r.At.UnixMilli()
		if seen[key{t.ID, at}] {
			continue
		}
		mine := false
		for _, o := range ours[t.ID] {
			end := o.At + int64(o.Secs)*1000
			if at >= o.At-int64(time.Minute/time.Millisecond) && at <= end+int64(listenSame/time.Millisecond) {
				mine = true
				break
			}
		}
		if mine {
			continue
		}
		dev := strings.ToLower(r.Device)
		if dev == "" {
			dev = "other"
		}
		seen[key{t.ID, at}] = true
		out = append(out, listenEntry{Track: keptTrack(t), At: at, Secs: int(t.Duration / 1000), Dev: dev})
	}
	return out
}

// Tests replace NetEase's figures of all the listener's listening.
var (
	allRanking   = netease.AllRanking
	listenTotals = netease.Listened
)

// loadAllListening asks NetEase for the all-time ranking and the listening
// total, unless asked a little while ago.
func (a *app) loadAllListening() {
	l := a.ledger()
	if a.profile == nil || l.allLoading || time.Since(l.allAt) < 10*time.Minute {
		return
	}
	l.allLoading = true
	uid, gen := a.profile.UserID, a.accountGen
	go func() {
		all, err := allRanking(uid)
		total, _ := listenTotals(uid)
		a.post(func() {
			l := &a.listen
			if gen != a.accountGen || l.uid != uid {
				return
			}
			l.allLoading = false
			if err != nil {
				return
			}
			l.all, l.total, l.allAt = all, total, time.Now()
		})
	}()
}
