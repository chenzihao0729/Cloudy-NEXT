package sourcecatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var escapedUnicode = regexp.MustCompile(`\\+u([0-9a-fA-F]{4})`)

// Kuwo's legacy endpoint sometimes returns a JSON string containing another
// escaped Unicode layer, for example "\\u0026" instead of "&".
func cleanKuwoText(value string) string {
	value = escapedUnicode.ReplaceAllStringFunc(value, func(escape string) string {
		hex := escape[len(escape)-4:]
		code, err := strconv.ParseUint(hex, 16, 16)
		if err != nil || code >= 0xd800 && code <= 0xdfff {
			return escape
		}
		return string(rune(code))
	})
	return strings.ReplaceAll(html.UnescapeString(value), "\u00a0", " ")
}

func kuwoCoverURL(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return ""
	case strings.HasPrefix(value, "//"):
		return "https:" + value
	case strings.HasPrefix(value, "http://"):
		return "https://" + strings.TrimPrefix(value, "http://")
	case strings.HasPrefix(value, "https://"):
		return value
	default:
		return "https://img4.kuwo.cn/star/albumcover/" + strings.TrimLeft(value, "/")
	}
}

func searchKuwoLegacy(ctx context.Context, query string, limit int) ([]Song, error) {
	// Without the catalog strategy flags, singer-name searches are dominated by
	// user uploads with ARTISTID=0 and no album. The official desktop catalog
	// returns the canonical recordings first and still supports pagination.
	values := url.Values{
		"all": {query}, "ft": {"music"}, "pn": {strconv.Itoa(searchPage(ctx) - 1)}, "rn": {strconv.Itoa(limit)},
		"rformat": {"json"}, "encoding": {"utf8"}, "client": {"kt"}, "uid": {"0"},
		"ver": {"kwplayer_ar_9.2.2.1"}, "vipver": {"1"}, "show_copyright_off": {"1"},
		"newver": {"1"}, "cluster": {"0"}, "strategy": {"2012"}, "mobi": {"1"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://search.kuwo.cn/r.s?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("kuwo search: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var data struct {
		Rows []struct {
			ID       string `json:"MUSICRID"`
			Name     string `json:"NAME"`
			SongName string `json:"SONGNAME"`
			Artist   string `json:"ARTIST"`
			ArtistID string `json:"allartistid"`
			Primary  string `json:"ARTISTID"`
			Album    string `json:"ALBUM"`
			AlbumID  string `json:"ALBUMID"`
			Duration string `json:"DURATION"`
			Cover    string `json:"web_albumpic_short"`
		} `json:"abslist"`
	}
	if err = decodeKuwoJSON(body, &data); err != nil {
		return nil, err
	}
	var songs []Song
	for _, row := range data.Rows {
		id := strings.TrimPrefix(row.ID, "MUSIC_")
		if id == "" || row.Name == "" {
			continue
		}
		sec, _ := strconv.Atoi(row.Duration)
		name := cleanKuwoText(row.SongName)
		if name == "" {
			name = cleanKuwoText(row.Name)
		}
		artist := cleanKuwoText(row.Artist)
		album := cleanKuwoText(row.Album)
		artistIDs := splitKuwoIDs(row.ArtistID)
		if len(artistIDs) == 0 {
			artistIDs = splitKuwoIDs(row.Primary)
		}
		songs = append(songs, Song{ID: id, Name: name, Artists: SplitArtists(artist), ArtistMIDs: artistIDs, Album: album, AlbumMID: row.AlbumID, Duration: time.Duration(sec) * time.Second, CoverURL: kuwoCoverURL(row.Cover)})
	}
	return enrichKuwoMetadata(ctx, songs), nil
}

func decodeKuwoJSON(body []byte, target any) error {
	// Catalog strategy responses are valid JSON and may legitimately contain
	// apostrophes in English titles. Only invoke the legacy single-quote parser
	// when the whole document is not already JSON.
	if json.Valid(body) {
		return json.Unmarshal(body, target)
	}
	converted, err := quotedJSON(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(converted, target)
}

// The legacy search catalog often omits album fields even for an official
// recording. Kuwo's mobile song-info endpoint exposes the canonical album for
// the exact music ID, so fill only missing metadata without guessing from a
// same-title recording on another platform.
func enrichKuwoMetadata(ctx context.Context, songs []Song) []Song {
	var wg sync.WaitGroup
	workers := make(chan struct{}, 6)
	for i := range songs {
		if songs[i].ID == "" || songs[i].Album != "" && songs[i].AlbumMID != "" {
			continue
		}
		wg.Add(1)
		go func(song *Song) {
			defer wg.Done()
			select {
			case workers <- struct{}{}:
				defer func() { <-workers }()
			case <-ctx.Done():
				return
			}
			info, err := kuwoSongInfo(ctx, song.ID)
			if err != nil {
				return
			}
			if song.Album == "" {
				song.Album = info.Album
			}
			if song.AlbumMID == "" || song.AlbumMID == "0" {
				song.AlbumMID = info.AlbumMID
			}
			if song.CoverURL == "" {
				song.CoverURL = info.CoverURL
			}
			if song.Duration <= 0 {
				song.Duration = info.Duration
			}
		}(&songs[i])
	}
	wg.Wait()
	return songs
}

func kuwoSongInfo(ctx context.Context, id string) (Song, error) {
	address := "https://m.kuwo.cn/newh5/singles/songinfoandlrc?musicId=" + url.QueryEscape(id)
	var payload struct {
		Data struct {
			Song struct {
				Album    string          `json:"album"`
				AlbumID  json.RawMessage `json:"albumId"`
				Duration json.RawMessage `json:"duration"`
				Cover    string          `json:"pic"`
			} `json:"songinfo"`
		} `json:"data"`
	}
	if err := getJSON(ctx, address, "https://m.kuwo.cn/", &payload); err != nil {
		return Song{}, err
	}
	duration, _ := strconv.Atoi(rawID(payload.Data.Song.Duration))
	return Song{
		Album:    cleanKuwoText(payload.Data.Song.Album),
		AlbumMID: rawID(payload.Data.Song.AlbumID),
		Duration: time.Duration(duration) * time.Second,
		CoverURL: kuwoCoverURL(payload.Data.Song.Cover),
	}, nil
}

func splitKuwoIDs(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '&' || r == ',' || r == '，' || r == ';' || r == '|'
	})
	out := parts[:0]
	for _, id := range parts {
		if id = strings.TrimSpace(id); id != "" && id != "0" {
			out = append(out, id)
		}
	}
	return out
}

// Kuwo returns a JSON-shaped document with single-quoted string tokens.
// Decode only string tokens; never execute a server response as JavaScript.
func quotedJSON(input []byte) ([]byte, error) {
	var out strings.Builder
	for i := 0; i < len(input); {
		if input[i] != '\'' {
			out.WriteByte(input[i])
			i++
			continue
		}
		i++
		var token strings.Builder
		closed := false
		for i < len(input) {
			ch := input[i]
			i++
			if ch == '\'' {
				closed = true
				break
			}
			if ch == '\\' && i < len(input) {
				next := input[i]
				i++
				if next == '\'' {
					token.WriteByte('\'')
				} else {
					token.WriteByte('\\')
					token.WriteByte(next)
				}
			} else {
				token.WriteByte(ch)
			}
		}
		if !closed {
			return nil, fmt.Errorf("kuwo search: unterminated string")
		}
		// Preserve JSON escape sequences such as \uXXXX and escaped backslashes.
		raw := "\"" + strings.ReplaceAll(token.String(), "\"", "\\\"") + "\""
		var decoded string
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return nil, err
		}
		encoded, _ := json.Marshal(decoded)
		out.Write(encoded)
	}
	return []byte(out.String()), nil
}
