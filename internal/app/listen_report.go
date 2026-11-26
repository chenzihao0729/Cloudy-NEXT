package app

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"netease-music-mygo/internal/netease"
)

// What the ledger tells of a week, and of all of it.

// songStat is a song of a week, and how often and long it was heard.
type songStat struct {
	Track netease.Track
	Plays int
	Secs  int
}

// nameStat is a singer, an album or a source of a week: how often its
// songs were played, how many of them, and a cover to show it by.
type nameStat struct {
	ID    int64
	Name  string
	Plays int
	Songs int
	Cover string
}

// weekReport is one week of the ledger, Monday to Sunday by Beijing's
// clock.
type weekReport struct {
	start time.Time // Monday, Beijing's midnight
	plays []listenEntry

	secs  int // heard, all told
	count int // plays
	songs []songStat
	// artists and albums, the most played first.
	artists, albums []nameStat
	days            [7]int  // seconds heard each day
	hours           [24]int // plays by the hour they started, on this computer's clock
	phone, computer int
	sources         []nameStat // where Cloudy's plays came from, the most first
	// late is the play that started latest at night, if one did after 22:00.
	late *listenEntry
	// loop is the song played most in one day, on loopDay, if twice or more.
	loop    songStat
	loopDay time.Time
	// fresh is how many songs were heard the first time that week, known
	// once the ledger reaches a month before it.
	fresh      int
	freshKnown bool
	// partial says the week began before Cloudy kept every play: only the
	// last play of each song shows.
	partial bool
}

// end is the Monday after the week.
func (r *weekReport) end() time.Time { return r.start.AddDate(0, 0, 7) }

// tracks is the week's songs, the most played first.
func (r *weekReport) tracks() []netease.Track {
	out := make([]netease.Track, len(r.songs))
	for i := range r.songs {
		out[i] = r.songs[i].Track
	}
	return out
}

// isPhone reports whether NetEase's device name is a phone's or a tablet's.
func isPhone(dev string) bool {
	switch dev {
	case "android", "iphone", "ios", "ipad", "harmony", "harmonyos", "wp", "aphone":
		return true
	}
	return false
}

// playKey tells songs apart in a report: by NetEase's id, or a file's path.
func playKey(t *netease.Track) string {
	if t.ID > 0 {
		return "id:" + idText(t.ID)
	}
	return "file:" + t.LocalPath
}

func idText(n int64) string { return strconv.FormatInt(n, 10) }

// weekPlays is the plays of all within [start, end), all being the
// earliest first.
func weekPlays(all []listenEntry, start, end time.Time) []listenEntry {
	from, to := start.UnixMilli(), end.UnixMilli()
	i, _ := slices.BinarySearchFunc(all, from, func(e listenEntry, t int64) int { return cmpInt(e.At, t) })
	j, _ := slices.BinarySearchFunc(all, to, func(e listenEntry, t int64) int { return cmpInt(e.At, t) })
	return all[i:j]
}

func cmpInt(x, y int64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// buildReport works out the week starting at start from all the ledger's
// plays, the earliest first; since is when the ledger began.
func buildReport(all []listenEntry, start time.Time, since int64) *weekReport {
	r := &weekReport{start: start}
	r.plays = weekPlays(all, start, r.end())
	r.partial = since > start.UnixMilli()
	r.freshKnown = since <= start.AddDate(0, 0, -28).UnixMilli()
	songs := map[string]*songStat{}
	var order []string
	type dayKey struct {
		song string
		day  int
	}
	perDay := map[dayKey]int{}
	artists := map[string]*nameStat{}
	artistSongs := map[string]map[string]bool{}
	albums := map[string]*nameStat{}
	albumSongs := map[string]map[string]bool{}
	sources := map[string]int{}
	lateScore := -1
	for i := range r.plays {
		e := &r.plays[i]
		k := playKey(&e.Track)
		s := songs[k]
		if s == nil {
			s = &songStat{Track: e.Track}
			songs[k] = s
			order = append(order, k)
		}
		s.Plays++
		s.Secs += e.Secs
		r.count++
		r.secs += e.Secs
		day := int(e.at().Sub(start) / (24 * time.Hour))
		if day >= 0 && day < 7 {
			r.days[day] += e.Secs
		}
		perDay[dayKey{k, day}]++
		local := e.at().In(time.Local)
		r.hours[local.Hour()]++
		// Minutes after six in the evening, until six in the morning: the
		// later at night, the more; from ten it is late.
		if score := (local.Hour()*60 + local.Minute() - 18*60 + 1440) % 1440; score < 12*60 && score > lateScore {
			lateScore = score
			if score >= 4*60 {
				r.late = e
			}
		}
		if isPhone(e.Dev) {
			r.phone++
		} else {
			r.computer++
		}
		if e.Dev == "" && e.Src != "" {
			sources[e.Src]++
		}
		if len(e.Track.Artists) > 0 && e.Track.Artists[0].Name != "" {
			ar := e.Track.Artists[0]
			n := artists[ar.Name]
			if n == nil {
				n = &nameStat{ID: ar.ID, Name: ar.Name, Cover: e.Track.Album.PicURL}
				artists[ar.Name] = n
				artistSongs[ar.Name] = map[string]bool{}
			}
			n.Plays++
			artistSongs[ar.Name][k] = true
		}
		if al := e.Track.Album; al.Name != "" {
			key := al.Name + "\x00" + idText(al.ID)
			n := albums[key]
			if n == nil {
				n = &nameStat{ID: al.ID, Name: al.Name, Cover: al.PicURL}
				albums[key] = n
				albumSongs[key] = map[string]bool{}
			}
			n.Plays++
			albumSongs[key][k] = true
		}
	}
	for _, k := range order {
		r.songs = append(r.songs, *songs[k])
	}
	slices.SortStableFunc(r.songs, func(x, y songStat) int {
		if x.Plays != y.Plays {
			return y.Plays - x.Plays
		}
		return y.Secs - x.Secs
	})
	for name, n := range artists {
		n.Songs = len(artistSongs[name])
		r.artists = append(r.artists, *n)
	}
	for key, n := range albums {
		n.Songs = len(albumSongs[key])
		r.albums = append(r.albums, *n)
	}
	byPlays := func(x, y nameStat) int {
		if x.Plays != y.Plays {
			return y.Plays - x.Plays
		}
		if x.Songs != y.Songs {
			return y.Songs - x.Songs
		}
		return strings.Compare(x.Name, y.Name)
	}
	slices.SortFunc(r.artists, byPlays)
	slices.SortFunc(r.albums, byPlays)
	for src, n := range sources {
		label := listenSources[src]
		if label == "" {
			label = listenSources["other"]
		}
		r.sources = append(r.sources, nameStat{Name: label, Plays: n})
	}
	slices.SortFunc(r.sources, byPlays)
	for k, n := range perDay {
		if n >= 2 && (n > r.loop.Plays || n == r.loop.Plays && start.AddDate(0, 0, k.day).After(r.loopDay)) {
			r.loop = *songs[k.song]
			r.loop.Plays = n
			r.loopDay = start.AddDate(0, 0, k.day)
		}
	}
	if r.freshKnown {
		before := map[string]bool{}
		for i := range all {
			if all[i].At >= start.UnixMilli() {
				break
			}
			before[playKey(&all[i].Track)] = true
		}
		for _, k := range order {
			if !before[k] {
				r.fresh++
			}
		}
	}
	return r
}

// secsUntil is how long was heard in the week before at.
func (r *weekReport) secsUntil(at time.Time) int {
	n := 0
	for i := range r.plays {
		if r.plays[i].at().Before(at) {
			n += r.plays[i].Secs
		}
	}
	return n
}

// report is the week starting at start, worked out once per change of the
// ledger.
func (a *app) report(start time.Time) *weekReport {
	l := a.ledger()
	if l.reports == nil || l.repGen != l.gen {
		l.reports, l.repGen = map[int64]*weekReport{}, l.gen
	}
	if r := l.reports[start.Unix()]; r != nil {
		return r
	}
	r := buildReport(l.plays, start, l.since)
	l.reports[start.Unix()] = r
	return r
}

// listenWeeks is the weeks the ledger has plays in, from its first to the
// one now is in, the earliest first.
func listenWeeks(all []listenEntry, now time.Time) []time.Time {
	last := weekStartOf(now)
	if len(all) == 0 {
		return []time.Time{last}
	}
	var out []time.Time
	for w := weekStartOf(all[0].at()); !w.After(last); w = w.AddDate(0, 0, 7) {
		out = append(out, w)
	}
	return out
}

// listenDays is the seconds heard each of Beijing's days, by its start.
func listenDays(all []listenEntry) map[int64]int {
	days := map[int64]int{}
	for i := range all {
		days[dayStartOf(all[i].at()).Unix()] += all[i].Secs
	}
	return days
}

// streakTo is how many days in a row up to now's had plays, counting from
// yesterday when today has none yet, and the first of them.
func streakTo(days map[int64]int, now time.Time) (int, time.Time) {
	day := dayStartOf(now)
	if days[day.Unix()] == 0 {
		day = day.AddDate(0, 0, -1)
	}
	n := 0
	first := day
	for days[day.Unix()] > 0 {
		n++
		first = day
		day = day.AddDate(0, 0, -1)
	}
	return n, first
}

// longestStreak is the most days in a row that had plays.
func longestStreak(days map[int64]int) int {
	best := 0
	for d := range days {
		day := time.Unix(d, 0).In(beijing)
		if days[day.AddDate(0, 0, -1).Unix()] > 0 {
			continue
		}
		n := 0
		for days[day.Unix()] > 0 {
			n++
			day = day.AddDate(0, 0, 1)
		}
		best = max(best, n)
	}
	return best
}

// rankedArtists adds up NetEase's ranking by singer, the most played first.
func rankedArtists(songs []netease.Ranked) []nameStat {
	byName := map[string]*nameStat{}
	var out []nameStat
	var names []string
	for _, s := range songs {
		if len(s.Track.Artists) == 0 || s.Track.Artists[0].Name == "" {
			continue
		}
		ar := s.Track.Artists[0]
		n := byName[ar.Name]
		if n == nil {
			n = &nameStat{ID: ar.ID, Name: ar.Name, Cover: s.Track.Album.PicURL}
			byName[ar.Name] = n
			names = append(names, ar.Name)
		}
		n.Plays += max(1, s.Plays)
		n.Songs++
	}
	for _, name := range names {
		out = append(out, *byName[name])
	}
	slices.SortStableFunc(out, func(x, y nameStat) int { return y.Plays - x.Plays })
	return out
}
