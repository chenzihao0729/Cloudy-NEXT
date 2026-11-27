package app

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/sourcecatalog"
)

var searchSourcePage = sourcecatalog.SearchPage
var searchNetEasePage = netease.SearchPageContext
var searchPlatforms = []string{"wy", "tx", "kg", "kw", "mg"}

func platformName(p string) string {
	switch p {
	case "wy":
		return "网易云音乐"
	case "tx":
		return "QQ音乐"
	case "kg":
		return "酷狗音乐"
	case "kw":
		return "酷我音乐"
	case "mg":
		return "咪咕音乐"
	}
	return p
}

type providerSearch struct {
	next int
	more bool
	err  string
}
type multiSearch struct {
	coverBusy  map[int64]bool
	coverRetry map[int64]time.Time
	coverSlots chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	providers  map[string]*providerSearch
	seen       map[int64]bool
}

func (a *app) startMultiSearch(r route, d *searchData) {
	ctx, cancel := context.WithCancel(context.Background())
	d.multi = &multiSearch{ctx: ctx, cancel: cancel, providers: map[string]*providerSearch{}, seen: map[int64]bool{}}
	for _, p := range searchPlatforms {
		d.multi.providers[p] = &providerSearch{next: 1, more: true}
	}
	a.moreMultiSearch(r, d)
}

// External IDs occupy a deterministic negative namespace, preserving legacy
// numeric queue/cache identities without sending provider IDs to NetEase APIs.
func providerTrack(platform string, song sourcecatalog.Song) netease.Track {
	key := song.ID
	if song.Hash != "" {
		key += ":" + song.Hash
	}
	sum := sha256.Sum256([]byte(platform + "\x00" + key))
	id := -int64(binary.BigEndian.Uint64(sum[:8])&((1<<62)-1)) - 1
	t := netease.Track{ID: id, Name: song.Name, Duration: song.Duration.Milliseconds(), Album: netease.Album{Name: song.Album, PicURL: song.CoverURL}, Source: &netease.SourceTrack{Platform: platform, IsCover: song.IsCover, LyricURL: song.LyricURL, ID: song.ID, MID: song.MID, MediaMID: song.MediaMID, Hash: song.Hash}}
	for i, name := range song.Artists {
		mid := ""
		if i < len(song.ArtistMIDs) {
			mid = song.ArtistMIDs[i]
		}
		t.Artists = append(t.Artists, netease.Artist{Name: name, SourceMID: mid})
	}
	t.Artists = expandedArtists(t.Artists)
	t.Source.AlbumMID = song.AlbumMID
	return t
}

type searchRequestPage struct {
	platform string
	page     int
}

type searchAnswer struct {
	platform string
	songs    []netease.Track
	err      error
}

// Each load appends a sorted batch; existing rows never move under the pointer.
func (a *app) moreMultiSearch(r route, d *searchData) {
	if d.page.loading {
		return
	}
	m := d.multi
	d.page.loading, d.page.err = true, ""
	var requests []searchRequestPage
	for _, p := range searchPlatforms {
		state := m.providers[p]
		if state.more {
			requests = append(requests, searchRequestPage{p, state.next})
		}
	}
	if len(requests) == 0 {
		d.page.loading = false
		return
	}
	go func() {
		answers := make(chan searchAnswer, len(requests))
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		for _, req := range requests {
			go func() {
				answer := searchAnswer{platform: req.platform}
				if req.platform == "wy" {
					res, err := searchNetEasePage(ctx, r.query, netease.SearchSongs, 20, (req.page-1)*20)
					answer.songs, answer.err = res.Songs, err
				} else {
					rows, err := searchSourcePage(ctx, req.platform, r.query, 20, req.page)
					answer.err = err
					for _, row := range rows {
						answer.songs = append(answer.songs, providerTrack(req.platform, row))
					}
				}
				for i := range answer.songs {
					position := (req.page-1)*20 + i
					answer.songs[i].SearchOrder = position
					if answer.songs[i].Source != nil {
						answer.songs[i].Source.SearchOrder = position
					}
				}
				answers <- answer
			}()
		}
		remaining := make(map[string]searchRequestPage, len(requests))
		for _, req := range requests {
			remaining[req.platform] = req
		}
		for len(remaining) > 0 {
			select {
			case answer := <-answers:
				req, ok := remaining[answer.platform]
				if !ok {
					continue
				}
				delete(remaining, answer.platform)
				final := len(remaining) == 0
				a.post(func() {
					if a.searches[r] != d || m.ctx.Err() != nil {
						return
					}
					applySearchAnswers(r.query, d, []searchRequestPage{req}, map[string]searchAnswer{answer.platform: answer})
					d.page.loading = !final
					sortSearchBatch(r.query, d.res.Songs)
				})
			case <-ctx.Done():
				pending := make([]searchRequestPage, 0, len(remaining))
				for _, req := range remaining {
					pending = append(pending, req)
				}
				a.post(func() {
					if a.searches[r] != d || m.ctx.Err() != nil {
						return
					}
					applySearchAnswers(r.query, d, pending, nil)
				})
				return
			}
		}
	}()
}

// Without authoritative original credits we only demote explicit alternate
// versions, never label an unmarked recording as an original.
func searchRank(query string, t netease.Track) int {
	q := sourcecatalog.Normalize(query)
	title := sourcecatalog.Normalize(t.Name)
	rank := 2
	if q == title || q == sourcecatalog.Normalize(t.Name+t.ArtistNames()) {
		rank = 0
	} else if strings.Contains(title, q) || strings.Contains(q, title) {
		rank = 1
	}
	tags := sourcecatalog.VersionTags(t.Name)
	if (tags != "" || strings.Contains(t.Album.Name, "翻唱") || t.Source != nil && t.Source.IsCover) && sourcecatalog.VersionTags(query) == "" {
		rank += 3
	}
	// Incomplete legacy rows are often user uploads that dropped the edition
	// suffix and album. Keep them available, but below catalogued recordings.
	if t.Source != nil && t.Source.Platform == "kw" && strings.TrimSpace(t.Album.Name) == "" {
		rank += 2
	}
	return rank
}

// Explicit cover credits are evidence about the credited artist, not proof
// that an unmarked track is original. Only compare credits for the same title.
var coverCredit = regexp.MustCompile(`(?i)[（(]\s*cover\s*[:：.\s]*([^）)]+)[）)]`)

func creditedArtists(songs []netease.Track) map[string][]string {
	credits := map[string][]string{}
	for _, song := range songs {
		for _, match := range coverCredit.FindAllStringSubmatch(song.Name, -1) {
			title := sourcecatalog.Normalize(coverCredit.ReplaceAllString(song.Name, ""))
			credits[title] = append(credits[title], strings.TrimSpace(match[1]))
		}
	}
	return credits
}
func sortSearchBatch(query string, songs []netease.Track) {
	credits := creditedArtists(songs)
	rank := func(t netease.Track) int {
		r := searchRank(query, t) * 2
		if sourcecatalog.VersionTags(query) == "" && sourcecatalog.VersionTags(t.Name) == "" {
			for _, credited := range credits[sourcecatalog.Normalize(t.Name)] {
				for _, artist := range t.Artists {
					if sourcecatalog.SameArtist(credited, artist.Name) {
						return r - 1
					}
				}
			}
		}
		return r
	}
	platform := func(t netease.Track) int {
		if t.Source == nil {
			return 0
		}
		for i, p := range searchPlatforms {
			if p == t.Source.Platform {
				return i
			}
		}
		return len(searchPlatforms)
	}
	sort.SliceStable(songs, func(i, j int) bool {
		a, b := rank(songs[i]), rank(songs[j])
		if a != b {
			return a < b
		}
		// Fuse independent provider ranks. Platform priority is a tie breaker,
		// not a reason to put every NetEase cover above other catalogs' top hit.
		left, right := searchConsensus(songs[i]), searchConsensus(songs[j])
		if left != right {
			return left > right
		}
		return platform(songs[i]) < platform(songs[j])
	})
}

func searchConsensus(track netease.Track) float64 {
	positions := map[string]int{}
	for _, ref := range searchTrackSources(track) {
		position := ref.SearchOrder
		if ref.Platform == "wy" && track.Source == nil {
			position = track.SearchOrder
		}
		if old, ok := positions[ref.Platform]; !ok || position < old {
			positions[ref.Platform] = position
		}
	}
	score := 0.0
	for _, platform := range searchPlatforms {
		if position, ok := positions[platform]; ok {
			score += 1 / float64(10+max(0, position))
		}
	}
	return score
}

func applySearchAnswers(query string, d *searchData, requests []searchRequestPage, received map[string]searchAnswer) {
	m := d.multi

	d.page.loading, d.page.loaded, d.page.more = false, true, false
	var batch []netease.Track
	failures := 0
	for _, req := range requests {
		state := m.providers[req.platform]
		answer, ok := received[req.platform]
		if !ok || answer.err != nil {
			state.err = platformName(req.platform) + "搜索失败"
			failures++
			continue
		}
		state.err = ""
		state.next++
		state.more = len(answer.songs) >= 20
		added := 0
		for _, t := range answer.songs {
			if !usefulSearchResult(t) {
				continue
			}
			if !m.seen[t.ID] {
				m.seen[t.ID] = true
				batch = append(batch, t)
				added++
			}
		}
		// Some public endpoints ignore their page parameter; don't keep
		// requesting the same full page forever.
		if added == 0 && req.page > 1 {
			state.more = false
		}
	}
	sortSearchBatch(query, batch)
	d.res.Songs = mergeSearchRecordings(d.res.Songs, batch)
	d.res.Total = len(d.res.Songs)
	for _, state := range m.providers {
		d.page.more = d.page.more || state.more
	}
	for _, state := range m.providers {
		if state.err != "" {
			failures++
		}
	}
	if failures > 0 {
		d.page.err = "部分平台搜索失败，可重试加载；其他结果仍可播放"
	}
}

// Short-video catalogs expose generated UGC sounds as if they were songs.
// This exact form is not a playable music result; ordinary soundtrack titles
// containing “原声” remain visible.
func usefulSearchResult(track netease.Track) bool {
	name := strings.TrimSpace(track.Name)
	return !(strings.HasPrefix(name, "@") && strings.HasSuffix(name, "创作的原声"))
}

// Only merge confidently identical recordings; missing metadata is not evidence.
func sameSearchRecording(a, b netease.Track) bool {
	if !sameSearchTitle(a.Name, b.Name) {
		return false
	}
	if a.Album.Name == "" || b.Album.Name == "" || sourcecatalog.Normalize(a.Album.Name) != sourcecatalog.Normalize(b.Album.Name) || a.Duration <= 0 || b.Duration <= 0 {
		return false
	}
	delta := a.Duration - b.Duration
	if delta < 0 {
		delta = -delta
	}
	if delta > 3000 {
		return false
	}
	leftArtists, rightArtists := trackArtistNames(a), trackArtistNames(b)
	if len(leftArtists) == 0 || len(leftArtists) != len(rightArtists) {
		return false
	}
	used := make([]bool, len(rightArtists))
	for _, artist := range leftArtists {
		found := false
		for j, other := range rightArtists {
			if !used[j] && sourcecatalog.SameArtist(artist, other) {
				used[j] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// A trailing subtitle may be omitted by one catalog. Strip it only when the
// other title is exactly the base; two different subtitles remain distinct.
var searchSubtitle = regexp.MustCompile(`\s*[（(]([^()（）]+)[）)]\s*$`)
var searchEdition = regexp.MustCompile(`(?i)live|现场|伴奏|instrumental|remix|\bdj\b|翻唱|cover|demo|纯音乐|acoustic|不插电|重制|重录|remaster|remastering|remastered|re-record|录音室|studio|单声道|立体声|mono|stereo|加速|减速|sped|slowed|版本|版`)

func sameSearchTitle(left, right string) bool {
	a, b := sourcecatalog.NormalizeTitle(left), sourcecatalog.NormalizeTitle(right)
	if a == "" || b == "" || sourcecatalog.VersionTags(left) != sourcecatalog.VersionTags(right) {
		return false
	}
	if a == b {
		return true
	}
	for _, pair := range [][2]string{{left, right}, {right, left}} {
		match := searchSubtitle.FindStringSubmatchIndex(pair[0])
		if match == nil || searchEdition.MatchString(pair[0][match[2]:match[3]]) {
			continue
		}
		base := sourcecatalog.NormalizeTitle(pair[0][:match[0]])
		if base != "" && base == sourcecatalog.NormalizeTitle(pair[1]) {
			return true
		}
	}
	return false
}

func searchTrackSources(t netease.Track) []netease.SourceTrack {
	refs := append([]netease.SourceTrack(nil), t.Sources...)
	if t.Source != nil {
		refs = append(refs, *t.Source)
	} else if t.ID > 0 {
		refs = append(refs, netease.SourceTrack{Platform: "wy", ID: fmt.Sprint(t.ID), SearchOrder: t.SearchOrder})
	}
	var out []netease.SourceTrack
	for _, platform := range searchPlatforms {
		for _, ref := range refs {
			if ref.Platform != platform {
				continue
			}
			duplicate := false
			for _, old := range out {
				if old.Platform == ref.Platform && old.ID == ref.ID && old.Hash == ref.Hash {
					duplicate = true
					break
				}
			}
			if !duplicate {
				out = append(out, ref)
			}
		}
	}
	return out
}

func mergeSearchRecordings(existing, incoming []netease.Track) []netease.Track {
	for _, song := range incoming {
		merged := false
		for i := range existing {
			if !sameSearchRecording(existing[i], song) {
				continue
			}
			refs := append(searchTrackSources(existing[i]), searchTrackSources(song)...)
			// Provider responses arrive concurrently. Always retain the NetEase
			// identity when present, regardless of which response arrived first.
			// This drives official playback, comments, and account actions.
			if existing[i].Source != nil && song.Source == nil && song.ID > 0 {
				previous := existing[i]
				existing[i] = song
				if existing[i].Album.PicURL == "" {
					existing[i].Album.PicURL = previous.Album.PicURL
				}
				for j := range existing[i].Artists {
					for _, artist := range previous.Artists {
						if sourcecatalog.SameArtist(existing[i].Artists[j].Name, artist.Name) && existing[i].Artists[j].SourceMID == "" {
							existing[i].Artists[j].SourceMID = artist.SourceMID
						}
					}
				}
			}
			existing[i].Sources = refs
			existing[i].Sources = searchTrackSources(existing[i])
			for j := range existing[i].Artists {
				for _, artist := range song.Artists {
					if sourcecatalog.SameArtist(existing[i].Artists[j].Name, artist.Name) && existing[i].Artists[j].SourceMID == "" {
						existing[i].Artists[j].SourceMID = artist.SourceMID
					}
				}
			}
			if existing[i].Album.PicURL == "" {
				existing[i].Album.PicURL = song.Album.PicURL
			}
			merged = true
			break
		}
		if !merged {
			existing = append(existing, song)
		}
	}
	return existing
}
