package sourcecatalog

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const miguImageBase = "https://d.musicapp.migu.cn"

func miguImageURL(value string) string {
	if strings.HasPrefix(value, "/") {
		return miguImageBase + value
	}
	return providerImageURL(value, 400)
}

func MiguArtist(ctx context.Context, id string) (ArtistProfile, error) {
	var index struct {
		Code string `json:"code"`
		Data struct {
			Contents []struct {
				View  string `json:"view"`
				Items []struct {
					View  string `json:"view"`
					Text  string `json:"txt"`
					Desc  string `json:"txt2"`
					Image string `json:"img"`
				} `json:"contents"`
			} `json:"contents"`
		} `json:"data"`
	}
	address := "https://app.c.nf.migu.cn/bmw/singer/index/v1.0?singerId=" + url.QueryEscape(id)
	if err := getJSON(ctx, address, "https://music.migu.cn/", &index); err != nil {
		return ArtistProfile{}, err
	}
	if index.Code != "000000" {
		return ArtistProfile{}, fmt.Errorf("migu artist: code %s", index.Code)
	}
	profile := ArtistProfile{}
	for _, section := range index.Data.Contents {
		for _, item := range section.Items {
			if item.View == "ZJ-Singer-Intro-Item" {
				profile.Description = strings.TrimSpace(item.Desc)
			}
		}
	}
	var albums struct {
		Code string `json:"code"`
		Data struct {
			Contents []struct {
				View   string `json:"view"`
				ID     string `json:"resId"`
				Image  string `json:"img"`
				Name   string `json:"txt"`
				Artist string `json:"txt2"`
				Date   string `json:"txt3"`
			} `json:"contents"`
		} `json:"data"`
	}
	address = "https://app.c.nf.migu.cn/bmw/singer/album/v1.0?singerId=" + url.QueryEscape(id) + "&pageNo=1"
	if err := getJSON(ctx, address, "https://music.migu.cn/", &albums); err != nil {
		return ArtistProfile{}, err
	}
	for _, album := range albums.Data.Contents {
		if album.View == "ZJ-Album-Item" {
			profile.Albums = append(profile.Albums, ArtistAlbum{ID: album.ID, Name: album.Name, CoverURL: miguImageURL(album.Image), Date: album.Date})
		}
	}
	return profile, nil
}

func MiguArtistSongs(ctx context.Context, id string, offset int) (ArtistPage, error) {
	// The singer endpoint returns 50 songs per page. Using the search page size
	// here skipped page two after the first response was appended.
	const pageSize = 50
	pageNo := offset/pageSize + 1
	type singer struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Image string `json:"img"`
	}
	type songItem struct {
		ID       string   `json:"copyrightId"`
		Name     string   `json:"songName"`
		Album    string   `json:"album"`
		AlbumID  string   `json:"albumId"`
		Duration int      `json:"duration"`
		Cover    string   `json:"img1"`
		LyricURL string   `json:"lrcUrl"`
		Singers  []singer `json:"singerList"`
	}
	type content struct {
		View     string    `json:"view"`
		Song     songItem  `json:"songItem"`
		Children []content `json:"contents"`
	}
	var data struct {
		Code string `json:"code"`
		Data struct {
			Header struct {
				Next string `json:"nextPageUrl"`
			} `json:"header"`
			Contents []content `json:"contents"`
		} `json:"data"`
	}
	address := "https://app.c.nf.migu.cn/bmw/singer/song/v1.0?type=1&singerId=" + url.QueryEscape(id) + "&pageNo=" + strconv.Itoa(pageNo)
	if err := getJSON(ctx, address, "https://music.migu.cn/", &data); err != nil {
		return ArtistPage{}, err
	}
	if data.Code != "000000" {
		return ArtistPage{}, fmt.Errorf("migu artist songs: code %s", data.Code)
	}
	page := ArtistPage{}
	var items []content
	var collect func([]content)
	collect = func(contents []content) {
		for _, item := range contents {
			if item.View == "ZJ-Singer-Song-Item" {
				items = append(items, item)
			}
			collect(item.Children)
		}
	}
	collect(data.Data.Contents)
	for _, item := range items {
		if item.View != "ZJ-Singer-Song-Item" || item.Song.ID == "" {
			continue
		}
		song := Song{ID: item.Song.ID, Name: item.Song.Name, Album: item.Song.Album, AlbumMID: item.Song.AlbumID, Duration: time.Duration(item.Song.Duration) * time.Second, CoverURL: miguImageURL(item.Song.Cover), LyricURL: item.Song.LyricURL}
		for _, singer := range item.Song.Singers {
			song.Artists = append(song.Artists, singer.Name)
			song.ArtistMIDs = append(song.ArtistMIDs, singer.ID)
			if singer.ID == id && singer.Image != "" {
				page.PhotoURL = miguImageURL(singer.Image)
			}
		}
		page.Songs = append(page.Songs, song)
	}
	page.Total = offset + len(page.Songs)
	if data.Data.Header.Next != "" {
		page.Total++
	}
	return page, nil
}
