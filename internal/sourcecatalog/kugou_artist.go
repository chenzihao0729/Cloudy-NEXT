package sourcecatalog

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func providerImageURL(value string, size int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "{size}", strconv.Itoa(size)))
	if strings.HasPrefix(value, "//") {
		return "https:" + value
	}
	if strings.HasPrefix(value, "http://") {
		return "https://" + strings.TrimPrefix(value, "http://")
	}
	return value
}

func KugouArtistID(ctx context.Context, hash, artistName string) (string, error) {
	var response struct {
		Error   int `json:"errcode"`
		Authors []struct {
			ID   int64  `json:"author_id"`
			Name string `json:"author_name"`
		} `json:"authors"`
	}
	address := "https://m.kugou.com/app/i/getSongInfo.php?cmd=playInfo&hash=" + url.QueryEscape(hash)
	if err := getJSON(ctx, address, "https://m.kugou.com/", &response); err != nil {
		return "", err
	}
	for _, author := range response.Authors {
		if author.ID > 0 && SameArtist(author.Name, artistName) {
			return strconv.FormatInt(author.ID, 10), nil
		}
	}
	return "", fmt.Errorf("kugou artist not found")
}

func KugouArtist(ctx context.Context, id string) (ArtistProfile, error) {
	var info struct {
		Status int `json:"status"`
		Data   struct {
			Name        string `json:"singername"`
			Description string `json:"profile"`
			Photo       string `json:"imgurl"`
			SongTotal   int    `json:"songcount"`
		} `json:"data"`
	}
	values := url.Values{"singerid": {id}}
	if err := getJSON(ctx, "https://mobileservice.kugou.com/api/v3/singer/info?"+values.Encode(), "https://m.kugou.com/", &info); err != nil {
		return ArtistProfile{}, err
	}
	var albums struct {
		Status int `json:"status"`
		Data   struct {
			Rows []struct {
				ID    int64  `json:"albumid"`
				Name  string `json:"albumname"`
				Cover string `json:"imgurl"`
				Date  string `json:"publishtime"`
			} `json:"info"`
		} `json:"data"`
	}
	values.Set("page", "1")
	values.Set("pagesize", "100")
	if err := getJSON(ctx, "https://mobileservice.kugou.com/api/v3/singer/album?"+values.Encode(), "https://m.kugou.com/", &albums); err != nil {
		return ArtistProfile{}, err
	}
	profile := ArtistProfile{Name: info.Data.Name, Description: info.Data.Description, PhotoURL: providerImageURL(info.Data.Photo, 400), SongTotal: info.Data.SongTotal}
	for _, album := range albums.Data.Rows {
		date := album.Date
		if len(date) >= 10 {
			date = date[:10]
		}
		profile.Albums = append(profile.Albums, ArtistAlbum{ID: strconv.FormatInt(album.ID, 10), Name: album.Name, CoverURL: providerImageURL(album.Cover, 400), Date: date})
	}
	return profile, nil
}

func KugouArtistSongs(ctx context.Context, id string, offset int) (ArtistPage, error) {
	const pageSize = 30
	values := url.Values{"singerid": {id}, "page": {strconv.Itoa(offset/pageSize + 1)}, "pagesize": {strconv.Itoa(pageSize)}}
	var data struct {
		Status int `json:"status"`
		Data   struct {
			Total int `json:"total"`
			Rows  []struct {
				Hash     string `json:"hash"`
				Filename string `json:"filename"`
				Album    string `json:"album_name"`
				AlbumID  string `json:"album_id"`
				Duration int    `json:"duration"`
				AudioID  int64  `json:"album_audio_id"`
				AudioAlt int64  `json:"audio_id"`
				Cover    string `json:"imgurl"`
				Transfer struct {
					Cover string `json:"union_cover"`
				} `json:"trans_param"`
			} `json:"info"`
		} `json:"data"`
	}
	if err := getJSON(ctx, "https://mobileservice.kugou.com/api/v3/singer/song?"+values.Encode(), "https://m.kugou.com/", &data); err != nil {
		return ArtistPage{}, err
	}
	page := ArtistPage{Total: data.Data.Total}
	for _, row := range data.Data.Rows {
		artist, name, ok := strings.Cut(row.Filename, " - ")
		if !ok {
			name = row.Filename
		}
		audioID := row.AudioID
		if audioID == 0 {
			audioID = row.AudioAlt
		}
		cover := row.Cover
		if cover == "" {
			cover = row.Transfer.Cover
		}
		page.Songs = append(page.Songs, Song{ID: strconv.FormatInt(audioID, 10), Hash: row.Hash, Name: strings.TrimSpace(name), Artists: SplitArtists(artist), ArtistMIDs: []string{id}, Album: row.Album, AlbumMID: row.AlbumID, Duration: time.Duration(row.Duration) * time.Second, CoverURL: providerImageURL(cover, 400)})
	}
	return page, nil
}
