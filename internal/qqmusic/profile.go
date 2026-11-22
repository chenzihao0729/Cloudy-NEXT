package qqmusic

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type ArtistAlbum struct {
	MID  string `json:"albumMid"`
	Name string `json:"albumName"`
	Date string `json:"publishDate"`
}
type ArtistProfile struct {
	Description string
	Albums      []ArtistAlbum
}

func SingerProfile(ctx context.Context, mid string) (ArtistProfile, error) {
	payload := map[string]any{
		"info":   map[string]any{"module": "music.musichallSinger.SingerInfoInter", "method": "GetSingerDetail", "param": map[string]any{"singer_mids": []string{mid}, "wiki_singer": 1}},
		"albums": map[string]any{"module": "music.musichallAlbum.AlbumListServer", "method": "GetAlbumList", "param": map[string]any{"singerMid": mid, "begin": 0, "num": 80, "order": 0}},
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "GET", "https://u.y.qq.com/cgi-bin/musicu.fcg?format=json&data="+url.QueryEscape(string(b)), nil)
	if err != nil {
		return ArtistProfile{}, err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	resp, err := client.Do(req)
	if err != nil {
		return ArtistProfile{}, err
	}
	defer resp.Body.Close()
	var result struct {
		Code int
		Info struct {
			Code int
			Data struct {
				Singers []struct {
					Wiki string `json:"wiki"`
				} `json:"singer_list"`
			}
		}
		Albums struct {
			Code int
			Data struct {
				Total int           `json:"total"`
				List  []ArtistAlbum `json:"albumList"`
			}
		}
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return ArtistProfile{}, err
	}
	if result.Code != 0 || result.Info.Code != 0 || result.Albums.Code != 0 {
		return ArtistProfile{}, fmt.Errorf("QQ profile unavailable")
	}
	profile := ArtistProfile{Albums: result.Albums.Data.List}
	for len(profile.Albums) < result.Albums.Data.Total {
		more, err := singerAlbumsPage(ctx, mid, len(profile.Albums))
		if err != nil {
			return ArtistProfile{}, err
		}
		if len(more) == 0 {
			break
		}
		profile.Albums = append(profile.Albums, more...)
	}
	if len(result.Info.Data.Singers) > 0 {
		var wiki struct {
			Description string `xml:"desc"`
		}
		if xml.Unmarshal([]byte(result.Info.Data.Singers[0].Wiki), &wiki) == nil {
			profile.Description = wiki.Description
		}
	}
	return profile, nil
}

func singerAlbumsPage(ctx context.Context, mid string, offset int) ([]ArtistAlbum, error) {
	payload := map[string]any{"albums": map[string]any{"module": "music.musichallAlbum.AlbumListServer", "method": "GetAlbumList", "param": map[string]any{"singerMid": mid, "begin": offset, "num": 80, "order": 0}}}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://u.y.qq.com/cgi-bin/musicu.fcg?format=json&data="+url.QueryEscape(string(b)), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Code   int `json:"code"`
		Albums struct {
			Code int `json:"code"`
			Data struct {
				List []ArtistAlbum `json:"albumList"`
			} `json:"data"`
		} `json:"albums"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || result.Code != 0 || result.Albums.Code != 0 {
		return nil, fmt.Errorf("QQ albums unavailable: HTTP %d", resp.StatusCode)
	}
	return result.Albums.Data.List, nil
}
