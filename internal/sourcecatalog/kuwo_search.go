package sourcecatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func searchKuwoLegacy(ctx context.Context, query string, limit int) ([]Song, error) {
	values := url.Values{"all": {query}, "ft": {"music"}, "pn": {strconv.Itoa(searchPage(ctx) - 1)}, "rn": {strconv.Itoa(limit)}, "rformat": {"json"}, "encoding": {"utf8"}}
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
	body, err = quotedJSON(body)
	if err != nil {
		return nil, err
	}
	var data struct {
		Rows []struct {
			ID       string `json:"MUSICRID"`
			Name     string `json:"NAME"`
			Artist   string `json:"ARTIST"`
			Album    string `json:"ALBUM"`
			Duration string `json:"DURATION"`
			Cover    string `json:"web_albumpic_short"`
		} `json:"abslist"`
	}
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	var songs []Song
	for _, row := range data.Rows {
		id := strings.TrimPrefix(row.ID, "MUSIC_")
		if id == "" || row.Name == "" {
			continue
		}
		sec, _ := strconv.Atoi(row.Duration)
		cover := ""
		if row.Cover != "" {
			cover = "https://img4.kuwo.cn/star/albumcover/" + row.Cover
		}
		songs = append(songs, Song{ID: id, Name: html.UnescapeString(row.Name), Artists: splitArtists(html.UnescapeString(row.Artist)), Album: html.UnescapeString(row.Album), Duration: time.Duration(sec) * time.Second, CoverURL: cover})
	}
	return songs, nil
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
