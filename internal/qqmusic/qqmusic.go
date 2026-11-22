package qqmusic

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
)

const searchEndpoint = "https://c.y.qq.com/soso/fcgi-bin/client_search_cp"

var client = &http.Client{Timeout: 12 * time.Second}

type Song struct {
	Disc       int
	Track      int
	MID        string
	MediaMID   string
	Name       string
	ArtistMIDs []string
	Artists    []string
	Album      string
	AlbumMID   string
	Duration   time.Duration
	CoverURL   string
}

func fullSongTitle(title, name string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}
	return name
}

type searchResponse struct {
	Code int `json:"code"`
	Data struct {
		Song struct {
			List []struct {
				SongMID  string `json:"songmid"`
				MediaMID string `json:"media_mid"`
				Name     string `json:"songname"`
				Album    string `json:"albumname"`
				AlbumMID string `json:"albummid"`
				Seconds  int    `json:"interval"`
				Singers  []struct {
					Name string `json:"name"`
					MID  string `json:"mid"`
				} `json:"singer"`
			} `json:"list"`
		} `json:"song"`
	} `json:"data"`
}

func Search(ctx context.Context, query string, limit int) ([]Song, error) {
	return SearchPage(ctx, query, limit, 1)
}

func SearchPage(ctx context.Context, query string, limit, page int) ([]Song, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("qqmusic: empty search")
	}
	if limit < 1 || limit > 30 {
		limit = 10
	}
	u, _ := url.Parse(searchEndpoint)
	q := u.Query()
	q.Set("format", "json")
	q.Set("p", strconv.Itoa(max(1, page)))
	q.Set("n", strconv.Itoa(limit))
	q.Set("w", query)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 Cloudy-NEXT/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qqmusic search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qqmusic search: HTTP %d", resp.StatusCode)
	}
	var data searchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data); err != nil {
		return nil, fmt.Errorf("qqmusic search: invalid response: %w", err)
	}
	if data.Code != 0 {
		return nil, fmt.Errorf("qqmusic search: response code %d", data.Code)
	}
	out := make([]Song, 0, len(data.Data.Song.List))
	for _, item := range data.Data.Song.List {
		if item.SongMID == "" || item.Name == "" {
			continue
		}
		song := Song{
			MID: item.SongMID, MediaMID: item.MediaMID, Name: item.Name,
			Album: item.Album, AlbumMID: item.AlbumMID,
			Duration: time.Duration(item.Seconds) * time.Second,
		}
		for _, artist := range item.Singers {
			if name := strings.TrimSpace(artist.Name); name != "" {
				song.Artists = append(song.Artists, name)
				song.ArtistMIDs = append(song.ArtistMIDs, artist.MID)
			}
		}
		if item.AlbumMID != "" {
			song.CoverURL = "https://y.gtimg.cn/music/photo_new/T002R300x300M000" + url.PathEscape(item.AlbumMID) + ".jpg"
		}
		out = append(out, song)
	}
	return out, nil
}

func Lyrics(ctx context.Context, songMID string) (string, error) {
	if songMID == "" {
		return "", errors.New("qqmusic: empty song id")
	}
	u, _ := url.Parse("https://c.y.qq.com/lyric/fcgi-bin/fcg_query_lyric_new.fcg")
	q := u.Query()
	q.Set("songmid", songMID)
	q.Set("format", "json")
	q.Set("nobase64", "1")
	q.Set("g_tk", "5381")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("qqmusic lyrics: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("qqmusic lyrics: HTTP %d", resp.StatusCode)
	}
	var data struct {
		Code  int    `json:"code"`
		Lyric string `json:"lyric"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data); err != nil {
		return "", fmt.Errorf("qqmusic lyrics: invalid response: %w", err)
	}
	if data.Code != 0 || data.Lyric == "" {
		return "", errors.New("qqmusic: lyrics unavailable")
	}
	return data.Lyric, nil
}

func Cover(ctx context.Context, coverURL string) ([]byte, error) {
	parsed, err := url.Parse(coverURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "y.gtimg.cn" {
		return nil, errors.New("qqmusic: invalid cover URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qqmusic cover: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qqmusic cover: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("qqmusic: empty cover")
	}
	return b, nil
}
