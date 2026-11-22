package qqmusic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"
)

// Album is a QQ album and its complete song list.
type Album struct {
	Name, Date, Description string
	Artists                 []string
	ArtistMIDs              []string
	Songs                   []Song
}

func AlbumDetail(ctx context.Context, mid string) (Album, error) {
	var album Album
	for offset := 0; ; {
		payload := map[string]any{
			"info":  map[string]any{"module": "music.musichallAlbum.AlbumInfoServer", "method": "GetAlbumDetail", "param": map[string]any{"albumMid": mid}},
			"songs": map[string]any{"module": "music.musichallAlbum.AlbumSongList", "method": "GetAlbumSongList", "param": map[string]any{"albumMid": mid, "begin": offset, "num": 100}},
		}
		b, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://u.y.qq.com/cgi-bin/musicu.fcg?format=json&data="+url.QueryEscape(string(b)), nil)
		if err != nil {
			return Album{}, err
		}
		req.Header.Set("Referer", "https://y.qq.com/")
		resp, err := client.Do(req)
		if err != nil {
			return Album{}, err
		}
		var result struct {
			Code int `json:"code"`
			Info struct {
				Code int `json:"code"`
				Data struct {
					Basic struct {
						Name string `json:"albumName"`
						Date string `json:"publishDate"`
						Desc string `json:"desc"`
					} `json:"basicInfo"`
					Singer struct {
						List []struct {
							MID  string `json:"mid"`
							Name string `json:"name"`
						} `json:"singerList"`
					} `json:"singer"`
				} `json:"data"`
			} `json:"info"`
			Songs struct {
				Code int `json:"code"`
				Data struct {
					Total int `json:"totalNum"`
					List  []struct {
						Info struct {
							MID      string `json:"mid"`
							Name     string `json:"name"`
							Title    string `json:"title"`
							Disc     int    `json:"index_cd"`
							Track    int    `json:"index_album"`
							Interval int    `json:"interval"`
							File     struct {
								MID string `json:"media_mid"`
							} `json:"file"`
							Singers []struct {
								MID  string `json:"mid"`
								Name string `json:"name"`
							} `json:"singer"`
						} `json:"songInfo"`
					} `json:"songList"`
				} `json:"data"`
			} `json:"songs"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&result)
		resp.Body.Close()
		if err != nil {
			return Album{}, err
		}
		if resp.StatusCode != http.StatusOK || result.Code != 0 || result.Info.Code != 0 || result.Songs.Code != 0 {
			return Album{}, fmt.Errorf("QQ album unavailable: HTTP %d, code %d/%d/%d", resp.StatusCode, result.Code, result.Info.Code, result.Songs.Code)
		}
		if offset == 0 {
			album.Name, album.Date, album.Description = result.Info.Data.Basic.Name, result.Info.Data.Basic.Date, result.Info.Data.Basic.Desc
			for _, ar := range result.Info.Data.Singer.List {
				album.Artists = append(album.Artists, ar.Name)
				album.ArtistMIDs = append(album.ArtistMIDs, ar.MID)
			}
		}
		for _, row := range result.Songs.Data.List {
			i := row.Info
			if i.MID == "" {
				continue
			}
			s := Song{Disc: i.Disc, Track: i.Track, MID: i.MID, MediaMID: i.File.MID, Name: fullSongTitle(i.Title, i.Name), Album: album.Name, AlbumMID: mid, Duration: time.Duration(i.Interval) * time.Second, CoverURL: "https://y.gtimg.cn/music/photo_new/T002R300x300M000" + url.PathEscape(mid) + ".jpg"}
			for _, ar := range i.Singers {
				s.Artists = append(s.Artists, ar.Name)
				s.ArtistMIDs = append(s.ArtistMIDs, ar.MID)
			}
			album.Songs = append(album.Songs, s)
		}
		n := len(result.Songs.Data.List)
		offset += n
		if n == 0 || offset >= result.Songs.Data.Total {
			break
		}
	}
	if album.Name == "" {
		return Album{}, fmt.Errorf("QQ album missing name")
	}
	// Preserve endpoint order when positions are absent or tied.
	known := len(album.Songs) > 0
	for _, song := range album.Songs {
		known = known && song.Track > 0
	}
	if known {
		sort.SliceStable(album.Songs, func(i, j int) bool {
			if album.Songs[i].Disc != album.Songs[j].Disc {
				return album.Songs[i].Disc < album.Songs[j].Disc
			}
			return album.Songs[i].Track < album.Songs[j].Track
		})
	}
	return album, nil
}
