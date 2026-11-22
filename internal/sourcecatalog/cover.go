package sourcecatalog

import (
	"context"
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
		rows, err := Search(ctx, platform, track.Name+" "+track.ArtistNames(), 10)
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
	return "", nil
}
