package sourcecatalog

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"sync"
)

func searchMiguApp(ctx context.Context, query string, limit int) ([]Song, error) {
	values := url.Values{"text": {query}, "pageNo": {strconv.Itoa(searchPage(ctx))}, "pageSize": {strconv.Itoa(limit)}, "searchSwitch": {`{"song":1}`}}
	var data struct {
		Code  string `json:"code"`
		Songs struct {
			Result []struct {
				ID        string `json:"copyrightId"`
				ContentID string `json:"contentId"`
				Name      string `json:"name"`
				Singers   []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"singers"`
				Albums []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"albums"`
				Images []struct {
					URL string `json:"img"`
				} `json:"imgItems"`
				Tags     []string `json:"tags"`
				LyricURL string   `json:"lyricUrl"`
			} `json:"result"`
		} `json:"songResultData"`
	}
	if err := getJSON(ctx, "https://app.c.nf.migu.cn/MIGUM3.0/v1.0/content/search_all.do?"+values.Encode(), "https://music.migu.cn/", &data); err != nil {
		return nil, err
	}
	if data.Code != "000000" {
		return nil, fmt.Errorf("migu search: code %s", data.Code)
	}
	var songs []Song
	for _, row := range data.Songs.Result {
		if row.ID == "" || row.Name == "" {
			continue
		}
		song := Song{ID: row.ID, Name: row.Name, IsCover: slices.Contains(row.Tags, "翻唱"), LyricURL: row.LyricURL}
		for _, artist := range row.Singers {
			song.Artists = append(song.Artists, artist.Name)
			song.ArtistMIDs = append(song.ArtistMIDs, artist.ID)
		}
		if len(row.Albums) > 0 {
			song.Album = row.Albums[0].Name
			song.AlbumMID = row.Albums[0].ID
		}
		if len(row.Images) > 0 {
			song.CoverURL = row.Images[0].URL
		}
		songs = append(songs, song)
	}
	// Search omits length; the exact recording detail supplies it. Bound
	// parallel requests so one page does not flood the provider.
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i := range songs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			var detail struct {
				Resources []struct {
					Length string `json:"length"`
				} `json:"resource"`
			}
			address := "https://app.c.nf.migu.cn/MIGUM3.0/v1.0/content/resourceinfo.do?resourceType=2&copyrightId=" + url.QueryEscape(songs[i].ID)
			if getJSON(ctx, address, "https://music.migu.cn/", &detail) == nil && len(detail.Resources) > 0 {
				songs[i].Duration = parseDuration(detail.Resources[0].Length)
			}
		}()
	}
	wg.Wait()
	return songs, nil
}
