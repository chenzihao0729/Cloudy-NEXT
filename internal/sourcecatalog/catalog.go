// Package sourcecatalog searches and strictly matches tracks on plugin platforms.
package sourcecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/qqmusic"
)

type Song struct {
	AlbumMID   string
	IsCover    bool
	LyricURL   string
	ID         string
	MID        string
	MediaMID   string
	Hash       string
	Name       string
	ArtistMIDs []string
	Artists    []string
	Album      string
	Duration   time.Duration
	CoverURL   string
}

type pageKey struct{}

// SearchPage searches a provider page while fallback matching keeps using page one.
func SearchPage(ctx context.Context, platform, query string, limit, page int) ([]Song, error) {
	return Search(context.WithValue(ctx, pageKey{}, max(1, page)), platform, query, limit)
}
func searchPage(ctx context.Context) int {
	page, _ := ctx.Value(pageKey{}).(int)
	return max(1, page)
}

type Searcher interface {
	Search(context.Context, string, int) ([]Song, error)
}

var client = &http.Client{Timeout: 12 * time.Second}

var providers = map[string]Searcher{
	"tx": qqSearcher{},
	"kg": kugouSearcher{},
	"kw": kuwoSearcher{},
	"mg": miguSearcher{},
	"qs": qishuiSearcher{},
}

func Search(ctx context.Context, platform, query string, limit int) ([]Song, error) {
	searcher := providers[platform]
	if searcher == nil {
		return nil, fmt.Errorf("unsupported music platform %q", platform)
	}
	return searcher.Search(ctx, query, limit)
}

func Match(track netease.Track, songs []Song) (Song, bool) {
	if strings.TrimSpace(track.Name) == "" || len(track.Artists) == 0 {
		return Song{}, false
	}
	var best Song
	bestScore := 0.0
	for _, song := range songs {
		if NormalizeTitle(track.Name) != NormalizeTitle(song.Name) || VersionTags(track.Name) != VersionTags(song.Name) {
			continue
		}
		artistOK := false
		for _, a := range track.Artists {
			for _, b := range song.Artists {
				if SameArtist(a.Name, b) {
					artistOK = true
				}
			}
		}
		albumMatch := track.Album.Name != "" && song.Album != "" && Normalize(track.Album.Name) == Normalize(song.Album)
		durationKnown := track.Duration > 0 && song.Duration > 0
		delta := time.Duration(0)
		if durationKnown {
			delta = track.Length() - song.Duration
			if delta < 0 {
				delta = -delta
			}
		}
		// When platform artist credits differ, require both album and a close
		// duration so a same-title cover or unrelated recording is not selected.
		if !artistOK && (!albumMatch || !durationKnown || delta > 8*time.Second) {
			continue
		}
		score := 0.7
		if artistOK {
			score = 0.8
		}
		if albumMatch {
			score += 0.1
		}
		if durationKnown {
			switch {
			case delta <= 3*time.Second:
				score += 0.1
			case delta <= 8*time.Second:
				score += 0.05
			case delta > 15*time.Second:
				continue
			}
		}
		if score > bestScore {
			best, bestScore = song, score
		}
	}
	return best, bestScore >= 0.85
}

type qqSearcher struct{}

func (qqSearcher) Search(ctx context.Context, query string, limit int) ([]Song, error) {
	rows, err := qqmusic.SearchPage(ctx, query, limit, searchPage(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]Song, 0, len(rows))
	for _, row := range rows {
		out = append(out, Song{AlbumMID: row.AlbumMID, ID: row.MID, MID: row.MID, MediaMID: row.MediaMID, Name: row.Name, Artists: row.Artists, ArtistMIDs: row.ArtistMIDs, Album: row.Album, Duration: row.Duration, CoverURL: row.CoverURL})
	}
	return out, nil
}

type kugouSearcher struct{}

func (kugouSearcher) Search(ctx context.Context, query string, limit int) ([]Song, error) {
	// Kugou's old search endpoint requires a signature; this public mobile endpoint does not.
	values := url.Values{"format": {"json"}, "keyword": {query}, "page": {strconv.Itoa(searchPage(ctx))}, "pagesize": {strconv.Itoa(limit)}, "showtype": {"1"}}
	var payload struct {
		Status int `json:"status"`
		Data   struct {
			Info []struct {
				Hash       string `json:"hash"`
				Name       string `json:"songname"`
				Singer     string `json:"singername"`
				SingerID   string `json:"singer_id"`
				Album      string `json:"album_name"`
				AlbumID    string `json:"album_id"`
				Duration   int    `json:"duration"`
				AudioID    int64  `json:"album_audio_id"`
				AudioIDAlt int64  `json:"audio_id"`
				Cover      string `json:"img"`
				Transfer   struct {
					Cover string `json:"union_cover"`
				} `json:"trans_param"`
			} `json:"info"`
		} `json:"data"`
	}
	err := getJSON(ctx, "https://mobileservice.kugou.com/api/v3/search/song?"+values.Encode(), "http://m.kugou.com/", &payload)
	if err != nil {
		return nil, err
	}
	if payload.Status != 1 {
		return nil, fmt.Errorf("kugou search: status %d", payload.Status)
	}
	out := make([]Song, 0, len(payload.Data.Info))
	for _, row := range payload.Data.Info {
		if row.Hash == "" || row.Name == "" {
			continue
		}
		id := row.AudioID
		if id == 0 {
			id = row.AudioIDAlt
		}
		cover := row.Cover
		if cover == "" {
			cover = row.Transfer.Cover
		}
		out = append(out, Song{ID: strconv.FormatInt(id, 10), Hash: row.Hash, Name: row.Name, Artists: SplitArtists(row.Singer), ArtistMIDs: splitProviderIDs(row.SingerID), Album: row.Album, AlbumMID: row.AlbumID, Duration: time.Duration(row.Duration) * time.Second, CoverURL: providerImageURL(cover, 400)})
	}
	return out, nil
}

type kuwoSearcher struct{}

func (kuwoSearcher) Search(ctx context.Context, query string, limit int) ([]Song, error) {
	values := url.Values{"key": {query}, "pn": {strconv.Itoa(searchPage(ctx))}, "rn": {strconv.Itoa(limit)}, "httpsStatus": {"1"}}
	var payload struct {
		Code string `json:"code"`
		Data struct {
			List []struct {
				RID      json.RawMessage `json:"rid"`
				Name     string          `json:"name"`
				Artist   string          `json:"artist"`
				ArtistID json.RawMessage `json:"artistid"`
				Album    string          `json:"album"`
				AlbumMID json.RawMessage `json:"albumid"`
				Duration string          `json:"songTimeMinutes"`
				AlbumID  string          `json:"albumpic"`
			} `json:"list"`
		} `json:"data"`
	}
	err := getJSON(ctx, "https://kuwo.cn/api/www/search/searchMusicBykeyWord?"+values.Encode(), "https://kuwo.cn/", &payload)
	if err != nil {
		return nil, err
	}
	if payload.Code != "200" {
		return searchKuwoLegacy(ctx, query, limit)
	}
	out := make([]Song, 0, len(payload.Data.List))
	for _, row := range payload.Data.List {
		id := rawID(row.RID)
		if id == "" || row.Name == "" {
			continue
		}
		out = append(out, Song{ID: id, Name: cleanKuwoText(row.Name), Artists: SplitArtists(cleanKuwoText(row.Artist)), ArtistMIDs: splitKuwoIDs(rawID(row.ArtistID)), Album: cleanKuwoText(row.Album), AlbumMID: rawID(row.AlbumMID), Duration: parseDuration(row.Duration), CoverURL: kuwoCoverURL(row.AlbumID)})
	}
	return out, nil
}

type miguSearcher struct{}

func (miguSearcher) Search(ctx context.Context, query string, limit int) ([]Song, error) {
	return searchMiguApp(ctx, query, limit)
}

type qishuiSearcher struct{}

func (qishuiSearcher) Search(context.Context, string, int) ([]Song, error) {
	return nil, errors.New("汽水音乐搜索需要登录态，暂不支持自动匹配")
}

func getJSON(ctx context.Context, address, referer string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36")
	req.Header.Set("Referer", referer)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("music search: HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); err != nil {
		return err
	}
	return nil
}

// SplitArtists separates combined credit fields returned by providers.
// Catalogs use ASCII/full-width commas and the Chinese enumeration comma
// interchangeably, sometimes in the same response.
func SplitArtists(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', '，', '、', '､', '&', '＆', '/', ';', '；':
			return true
		}
		return false
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitProviderIDs(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '&' || r == '、' || r == '|' || r == ';'
	})
}

func rawID(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return text
	}
	return strings.Trim(string(value), `"`)
}

func parseDuration(value string) time.Duration {
	seconds, _ := strconv.Atoi(strings.TrimSpace(value))
	if seconds == 0 {
		parts := strings.Split(value, ":")
		if len(parts) == 2 || len(parts) == 3 {
			total := 0
			for _, part := range parts {
				n, err := strconv.Atoi(part)
				if err != nil || n < 0 {
					return 0
				}
				total = total*60 + n
			}
			return time.Duration(total) * time.Second
		}
	}
	return time.Duration(seconds) * time.Second
}
