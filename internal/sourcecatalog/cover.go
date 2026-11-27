package sourcecatalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"netease-music-mygo/internal/netease"
)

// Cover first asks the recording's own provider, then accepts only a strict
// same-recording match with known album and duration from another catalog.
func Cover(ctx context.Context, track netease.Track) (string, error) {
	if track.Source == nil {
		return "", nil
	}
	ref := track.Source
	if ref.Platform == "kg" && ref.Hash != "" {
		var response struct {
			Data struct {
				Image string `json:"img"`
			} `json:"data"`
		}
		if err := getJSON(ctx, "https://m.kugou.com/app/i/getSongInfo.php?cmd=playInfo&hash="+url.QueryEscape(ref.Hash), "https://m.kugou.com/", &response); err == nil && response.Data.Image != "" {
			return strings.ReplaceAll(response.Data.Image, "{size}", "150"), nil
		}
	}
	if ref.Platform == "kw" && ref.ID != "" {
		if cover, err := kuwoCover(ctx, ref.ID); err == nil && cover != "" {
			return cover, nil
		}
	}
	platforms := []string{ref.Platform, "tx", "kg", "kw", "mg"}
	seen := map[string]bool{}
	for _, platform := range platforms {
		if seen[platform] {
			continue
		}
		seen[platform] = true
		if err := ctx.Err(); err != nil {
			return "", err
		}
		queries := []string{track.Name + " " + track.ArtistNames()}
		if base := SearchTitle(track.Name); base != "" && base != track.Name {
			queries = append(queries, base+" "+track.ArtistNames())
		}
		for _, query := range queries {
			rows, err := Search(ctx, platform, query, 10)
			if err != nil {
				continue
			}
			for _, song := range rows {
				if song.CoverURL == "" {
					continue
				}
				if platform == ref.Platform && song.ID == ref.ID && (ref.Hash == "" || song.Hash == ref.Hash) {
					return song.CoverURL, nil
				}
				if track.Album.Name == "" || song.Album == "" || track.Duration <= 0 || song.Duration <= 0 {
					continue
				}
				if Normalize(track.Album.Name) != Normalize(song.Album) {
					continue
				}
				delta := track.Length() - song.Duration
				if delta < 0 {
					delta = -delta
				}
				if delta.Seconds() > 3 {
					continue
				}
				if _, ok := Match(track, []Song{song}); ok {
					return song.CoverURL, nil
				}
			}
		}
	}
	return "", nil
}

func kuwoCover(ctx context.Context, id string) (string, error) {
	address := "https://artistpicserver.kuwo.cn/pic.web?type=rid_pic&pictype=url&size=500&rid=" + url.QueryEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kuwo cover: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(string(body))
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "//") {
		return "", nil
	}
	cover := kuwoCoverURL(raw)
	parsed, err := url.Parse(cover)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
		return "", nil
	}
	return cover, nil
}
