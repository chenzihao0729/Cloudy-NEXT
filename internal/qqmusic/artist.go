package qqmusic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type ArtistPage struct {
	Name  string
	Songs []Song
	Total int
}

// ArtistSongs requests songs by QQ singer MID rather than a name search.
func ArtistSongs(ctx context.Context, mid string, offset int) (ArtistPage, error) {
	payload := map[string]any{"comm": map[string]any{"ct": 24, "cv": 0}, "req": map[string]any{"module": "musichall.song_list_server", "method": "GetSingerSongList", "param": map[string]any{"singerMid": mid, "begin": offset, "num": 30, "order": 1}}}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://u.y.qq.com/cgi-bin/musicu.fcg?format=json&data="+url.QueryEscape(string(b)), nil)
	if err != nil {
		return ArtistPage{}, err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	resp, err := client.Do(req)
	if err != nil {
		return ArtistPage{}, err
	}
	defer resp.Body.Close()
	var result struct {
		Code int `json:"code"`
		Req  struct {
			Code int `json:"code"`
			Data struct {
				Total int `json:"totalNum"`
				Songs []struct {
					Info struct {
						MID      string `json:"mid"`
						Name     string `json:"name"`
						Title    string `json:"title"`
						Interval int    `json:"interval"`
						File     struct {
							MID string `json:"media_mid"`
						} `json:"file"`
						Album struct {
							MID  string `json:"mid"`
							Name string `json:"name"`
						} `json:"album"`
						Singers []struct {
							MID  string `json:"mid"`
							Name string `json:"name"`
						} `json:"singer"`
					} `json:"songInfo"`
				} `json:"songList"`
			} `json:"data"`
		} `json:"req"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return ArtistPage{}, err
	}
	if resp.StatusCode != 200 || result.Code != 0 || result.Req.Code != 0 {
		return ArtistPage{}, fmt.Errorf("QQ artist response: HTTP %d, code %d/%d", resp.StatusCode, result.Code, result.Req.Code)
	}
	page := ArtistPage{Total: result.Req.Data.Total}
	for _, row := range result.Req.Data.Songs {
		info := row.Info
		if info.MID == "" {
			continue
		}
		song := Song{MID: info.MID, MediaMID: info.File.MID, Name: fullSongTitle(info.Title, info.Name), Album: info.Album.Name, AlbumMID: info.Album.MID, Duration: time.Duration(info.Interval) * time.Second}
		if info.Album.MID != "" {
			song.CoverURL = "https://y.gtimg.cn/music/photo_new/T002R300x300M000" + url.PathEscape(info.Album.MID) + ".jpg"
		}
		for _, artist := range info.Singers {
			song.Artists = append(song.Artists, artist.Name)
			song.ArtistMIDs = append(song.ArtistMIDs, artist.MID)
		}
		page.Songs = append(page.Songs, song)
	}
	return page, nil
}
