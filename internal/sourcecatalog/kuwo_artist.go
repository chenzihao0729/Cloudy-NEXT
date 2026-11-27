package sourcecatalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ArtistAlbum struct {
	ID       string
	Name     string
	CoverURL string
	Date     string
}

type ArtistProfile struct {
	Name        string
	Description string
	PhotoURL    string
	SongTotal   int
	Albums      []ArtistAlbum
}

type ArtistPage struct {
	Songs    []Song
	Total    int
	PhotoURL string
}

func kuwoLegacy(ctx context.Context, values url.Values, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kuwoLegacyURL(values), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36")
	req.Header.Set("Referer", "https://kuwo.cn/")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("kuwo artist: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	return decodeKuwoJSON(body, target)
}

// Kuwo's legacy gateway incorrectly treats stype as positional for some
// endpoints. In particular, artist2music returns a valid-looking empty page
// when url.Values.Encode puts stype after the alphabetically earlier keys.
func kuwoLegacyURL(values url.Values) string {
	query := values.Encode()
	if stype := values.Get("stype"); stype != "" {
		rest := make(url.Values, len(values)-1)
		for key, entries := range values {
			if key != "stype" {
				rest[key] = append([]string(nil), entries...)
			}
		}
		query = "stype=" + url.QueryEscape(stype)
		if encoded := rest.Encode(); encoded != "" {
			query += "&" + encoded
		}
	}
	return "https://search.kuwo.cn/r.s?" + query
}

func KuwoArtist(ctx context.Context, id string) (ArtistProfile, error) {
	common := func(stype string) url.Values {
		return url.Values{"stype": {stype}, "artistid": {id}, "rformat": {"json"}, "encoding": {"utf8"}}
	}
	var info struct {
		Name       string `json:"name"`
		Desc       string `json:"desc"`
		Photo      string `json:"hts_pic"`
		PhotoShort string `json:"pic"`
		SongTotal  string `json:"musicnum"`
	}
	if err := kuwoLegacy(ctx, common("artistinfo"), &info); err != nil {
		return ArtistProfile{}, err
	}
	values := common("albumlist")
	values.Set("pn", "0")
	values.Set("rn", "100")
	var albums struct {
		Rows []struct {
			ID      string `json:"albumid"`
			Name    string `json:"album"`
			NameAlt string `json:"name"`
			Cover   string `json:"hts_img"`
			Image   string `json:"img"`
			Date    string `json:"releasedate"`
		} `json:"albumlist"`
	}
	if err := kuwoLegacy(ctx, values, &albums); err != nil {
		return ArtistProfile{}, err
	}
	profile := ArtistProfile{Name: cleanKuwoText(info.Name), Description: cleanKuwoText(info.Desc), PhotoURL: kuwoCoverURL(info.Photo)}
	if profile.PhotoURL == "" && info.PhotoShort != "" {
		profile.PhotoURL = "https://img1.kuwo.cn/star/starheads/" + strings.TrimLeft(info.PhotoShort, "/")
	}
	profile.SongTotal, _ = strconv.Atoi(info.SongTotal)
	for _, row := range albums.Rows {
		name := row.Name
		if name == "" {
			name = row.NameAlt
		}
		cover := row.Cover
		if cover == "" {
			cover = row.Image
		}
		profile.Albums = append(profile.Albums, ArtistAlbum{ID: row.ID, Name: cleanKuwoText(name), CoverURL: kuwoCoverURL(cover), Date: row.Date})
	}
	return profile, nil
}

// KuwoArtistID resolves an exact singer name independently of song metadata.
// Some legacy song rows carry ARTISTID=0 even though the artist catalog has a
// stable identity, so song search alone cannot open that singer's page.
func KuwoArtistID(ctx context.Context, name string) (string, error) {
	values := url.Values{"all": {name}, "ft": {"artist"}, "pn": {"0"}, "rn": {"20"}, "rformat": {"json"}, "encoding": {"utf8"}}
	var data struct {
		Rows []struct {
			ID   string `json:"ARTISTID"`
			Name string `json:"ARTIST"`
		} `json:"abslist"`
	}
	if err := kuwoLegacy(ctx, values, &data); err != nil {
		return "", err
	}
	for _, row := range data.Rows {
		if row.ID != "" && row.ID != "0" && SameArtist(cleanKuwoText(row.Name), name) {
			return row.ID, nil
		}
	}
	return "", fmt.Errorf("kuwo artist not found")
}

func KuwoArtistSongs(ctx context.Context, id string, offset int) (ArtistPage, error) {
	const pageSize = 30
	values := url.Values{"stype": {"artist2music"}, "artistid": {id}, "pn": {strconv.Itoa(offset / pageSize)}, "rn": {strconv.Itoa(pageSize)}, "rformat": {"json"}, "encoding": {"utf8"}}
	var data struct {
		Total string `json:"total"`
		Rows  []struct {
			ID        string `json:"musicrid"`
			Name      string `json:"name"`
			Artist    string `json:"artist"`
			ArtistIDs string `json:"allartistid"`
			Album     string `json:"album"`
			AlbumID   string `json:"albumid"`
			Duration  string `json:"duration"`
			Cover     string `json:"web_albumpic_short"`
			Image     string `json:"img"`
		} `json:"musiclist"`
	}
	if err := kuwoLegacy(ctx, values, &data); err != nil {
		return ArtistPage{}, err
	}
	page := ArtistPage{}
	page.Total, _ = strconv.Atoi(data.Total)
	for _, row := range data.Rows {
		id := strings.TrimPrefix(row.ID, "MUSIC_")
		if id == "" || row.Name == "" {
			continue
		}
		seconds, _ := strconv.Atoi(row.Duration)
		cover := row.Cover
		if cover == "" {
			cover = row.Image
		}
		page.Songs = append(page.Songs, Song{ID: id, Name: cleanKuwoText(row.Name), Artists: SplitArtists(cleanKuwoText(row.Artist)), ArtistMIDs: splitKuwoIDs(row.ArtistIDs), Album: cleanKuwoText(row.Album), AlbumMID: row.AlbumID, Duration: time.Duration(seconds) * time.Second, CoverURL: kuwoCoverURL(cover)})
	}
	return page, nil
}
