package sourcecatalog

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Kuwo's nonstandard quoted strings are decoded as data without executing response code.
func TestKuwoQuotedResponse(t *testing.T) {
	b, err := quotedJSON([]byte(`{'name':'Can\'t go back','artist':'S.H.E','unicode':'\u7231'}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err = json.Unmarshal(b, &result); err != nil || result["name"] != "Can't go back" || result["unicode"] != "爱" {
		t.Fatalf("%s %v", b, err)
	}
	if _, err := quotedJSON([]byte(`{'x':'unterminated}`)); err == nil {
		t.Fatal("malformed response accepted")
	}
	var english map[string]string
	if err := decodeKuwoJSON([]byte(`{"title":"Can't go back","artist":"Taylor Swift"}`), &english); err != nil || english["title"] != "Can't go back" {
		t.Fatalf("valid English JSON = %+v, %v", english, err)
	}
}

// Kuwo's legacy search sometimes leaves a second Unicode-escape layer in
// artist credits; it must become a real separator before artists are split.
func TestKuwoEscapedArtistSeparator(t *testing.T) {
	for _, credit := range []string{`于冬然\u0026木淋`, `于冬然\\u0026木淋`} {
		got := SplitArtists(cleanKuwoText(credit))
		if strings.Join(got, "|") != "于冬然|木淋" {
			t.Fatalf("credit %q became %q", credit, got)
		}
	}
	if got := kuwoCoverURL(`//img1.kwcdn.kuwo.cn/cover.jpg`); got != "https://img1.kwcdn.kuwo.cn/cover.jpg" {
		t.Fatalf("protocol-relative cover = %q", got)
	}
}

// Kuwo's legacy artist endpoints provide the identity, introduction, albums
// and songs needed by the in-app artist page.
func TestKuwoArtistCatalog(t *testing.T) {
	old := client
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if stype := r.URL.Query().Get("stype"); stype != "" && !strings.HasPrefix(r.URL.RawQuery, "stype=") {
			t.Fatalf("stype is not first in legacy query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("ft") != "artist" && r.URL.Query().Get("artistid") != "336" {
			t.Fatalf("artist id = %q", r.URL.Query().Get("artistid"))
		}
		var body string
		switch {
		case r.URL.Query().Get("ft") == "artist":
			body = `{'abslist':[{'ARTISTID':'336','ARTIST':'周杰伦'}]}`
		case r.URL.Query().Get("stype") == "artistinfo":
			body = `{'name':'周杰伦','desc':'歌手简介','musicnum':'1','hts_pic':'http://img.kwcdn.test/artist.jpg'}`
		case r.URL.Query().Get("stype") == "albumlist":
			body = `{'albumlist':[{'albumid':'1293','album':'叶惠美','hts_img':'//img.kwcdn.test/album.jpg','releasedate':'2003-07-31'}]}`
		case r.URL.Query().Get("stype") == "artist2music":
			body = `{'total':'1','musiclist':[{'musicrid':'228908','name':'晴天','artist':'周杰伦','allartistid':'336','album':'叶惠美','albumid':'1293','duration':'269','web_albumpic_short':'120/cover.jpg'}]}`
		default:
			t.Fatalf("unexpected Kuwo request: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	defer func() { client = old }()
	artistID, err := KuwoArtistID(context.Background(), "周杰伦")
	if err != nil || artistID != "336" {
		t.Fatalf("artist id = %q, %v", artistID, err)
	}
	profile, err := KuwoArtist(context.Background(), "336")
	if err != nil || profile.Name != "周杰伦" || profile.Description != "歌手简介" || len(profile.Albums) != 1 || profile.Albums[0].Name != "叶惠美" || !strings.HasPrefix(profile.PhotoURL, "https://") {
		t.Fatalf("profile = %+v, %v", profile, err)
	}
	page, err := KuwoArtistSongs(context.Background(), "336", 0)
	if err != nil || page.Total != 1 || len(page.Songs) != 1 || page.Songs[0].ID != "228908" || page.Songs[0].AlbumMID != "1293" || strings.Join(page.Songs[0].ArtistMIDs, ",") != "336" {
		t.Fatalf("songs = %+v, %v", page, err)
	}
}

// Kuwo artist songs used to be empty because its legacy gateway requires the
// stype parameter to appear first instead of accepting normal query ordering.
func TestKuwoArtistSongsKeepsLegacyParameterOrder(t *testing.T) {
	values := url.Values{"artistid": {"1610644"}, "stype": {"artist2music"}, "rn": {"30"}}
	address := kuwoLegacyURL(values)
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(parsed.RawQuery, "stype=artist2music&") || parsed.Query().Get("artistid") != "1610644" {
		t.Fatalf("legacy URL = %s", address)
	}
}

// Kuwo's display title carries edition information that NAME omits, and the
// legacy row still retains its album and singer identities when available.
func TestKuwoLegacySearchUsesCompleteTitleAndMetadata(t *testing.T) {
	old := client
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("strategy") != "2012" || r.URL.Query().Get("newver") != "1" {
			t.Fatalf("missing Kuwo catalog strategy: %s", r.URL)
		}
		body := `{'abslist':[{'MUSICRID':'MUSIC_1','NAME':'花海','SONGNAME':'花海&nbsp;(DJ&nbsp;阿若版)','ARTIST':'周杰伦','ARTISTID':'336','ALBUM':'专辑','ALBUMID':'1293','DURATION':'210'}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	defer func() { client = old }()
	rows, err := searchKuwoLegacy(context.Background(), "花海", 10)
	if err != nil || len(rows) != 1 || rows[0].Name != "花海 (DJ 阿若版)" || rows[0].Album != "专辑" || rows[0].AlbumMID != "1293" || strings.Join(rows[0].ArtistMIDs, ",") != "336" {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
}

// Kuwo search rows sometimes omit an official recording's album; the exact
// song-info response fills it while genuinely albumless uploads stay blank.
func TestKuwoLegacySearchFillsMissingAlbumFromSongInfo(t *testing.T) {
	old := client
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		body := `{'abslist':[{'MUSICRID':'MUSIC_455982633','NAME':'座位','SONGNAME':'座位','ARTIST':'于冬然','ARTISTID':'1610644','ALBUM':'','ALBUMID':'0','DURATION':'208'}]}`
		if strings.Contains(r.URL.Path, "songinfoandlrc") {
			body = `{"data":{"songinfo":{"album":"座位","albumId":64625884,"duration":208,"pic":"http://img.test/cover.jpg"}}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	defer func() { client = old }()
	rows, err := searchKuwoLegacy(context.Background(), "座位", 10)
	if err != nil || len(rows) != 1 || rows[0].Album != "座位" || rows[0].AlbumMID != "64625884" || rows[0].CoverURL != "https://img.test/cover.jpg" {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
}

// Paginating external search sends the requested page and preserves minute-based durations.
func TestSearchProviderPageAndDuration(t *testing.T) {
	old := client
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("page") != "3" {
			t.Fatalf("wrong page: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":1,"data":{"info":[]}}`)), Header: make(http.Header)}, nil
	})}
	defer func() { client = old }()
	if _, err := SearchPage(context.Background(), "kg", "song", 20, 3); err != nil {
		t.Fatal(err)
	}
	if got := parseDuration("03:24"); got != 204*time.Second {
		t.Fatalf("duration: %v", got)
	}
}

// Live provider probes report endpoint availability without requiring a paid plugin.
func TestLiveProviderSearch(t *testing.T) {
	if os.Getenv("LIVE_SEARCH") == "" {
		t.Skip("set LIVE_SEARCH to probe public catalogs")
	}
	for _, p := range []string{"tx", "kg", "kw", "mg"} {
		t.Run(p, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			songs, err := SearchPage(ctx, p, "最近还好吗", 20, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(songs) == 0 {
				t.Fatal("no results")
			}
			t.Logf("%d songs; first: %s", len(songs), songs[0].Name)
		})
	}
}

// Live Kuwo probes cover popular, emerging, niche and overseas artists so one
// catalog shape cannot make the native artist page look healthier than it is.
func TestLiveKuwoArtistCatalogSamples(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to probe the Kuwo artist catalog")
	}
	samples := []struct {
		kind string
		name string
	}{
		{"热门华语", "周杰伦"},
		{"新生华语", "于冬然"},
		{"小众华语", "惘闻"},
		{"海外歌手", "Taylor Swift"},
		{"海外歌手", "Adele"},
	}
	for _, sample := range samples {
		t.Run(sample.kind+"/"+sample.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			id, err := KuwoArtistID(ctx, sample.name)
			if err != nil || id == "" {
				t.Fatalf("resolve %q: id=%q err=%v", sample.name, id, err)
			}
			profile, err := KuwoArtist(ctx, id)
			if err != nil || profile.Name == "" {
				t.Fatalf("profile %q (%s): %+v err=%v", sample.name, id, profile, err)
			}
			page, err := KuwoArtistSongs(ctx, id, 0)
			if err != nil || page.Total == 0 || len(page.Songs) == 0 {
				t.Fatalf("songs %q (%s): total=%d rows=%d err=%v", sample.name, id, page.Total, len(page.Songs), err)
			}
			withAlbum := 0
			for _, song := range page.Songs {
				if song.Album != "" {
					withAlbum++
				}
			}
			if withAlbum == 0 {
				t.Fatalf("songs %q (%s) have no album metadata", sample.name, id)
			}
			results, err := Search(ctx, "kw", sample.name, 20)
			if err != nil || len(results) == 0 {
				t.Fatalf("search %q: rows=%d err=%v", sample.name, len(results), err)
			}
			searchAlbums := 0
			for _, song := range results {
				if song.Album != "" {
					searchAlbums++
				}
			}
			if searchAlbums == 0 {
				t.Fatalf("search %q has no album metadata", sample.name)
			}
			t.Logf("%s: id=%s total=%d first-page=%d albums=%d profile-albums=%d search-albums=%d/%d", profile.Name, id, page.Total, len(page.Songs), withAlbum, len(profile.Albums), searchAlbums, len(results))
		})
	}
}
