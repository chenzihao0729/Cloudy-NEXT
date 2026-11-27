package sourcecatalog

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Kugou artist metadata and songs retain the singer, album and playback identities.
func TestKugouArtistCatalog(t *testing.T) {
	old := client
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case strings.Contains(r.URL.Path, "getSongInfo.php"):
			body = `{"errcode":0,"authors":[{"author_id":3520,"author_name":"周杰伦"}]}`
		case strings.HasSuffix(r.URL.Path, "/singer/info"):
			body = `{"status":1,"data":{"singername":"周杰伦","profile":"简介","imgurl":"http://img.test/{size}/artist.jpg","songcount":1}}`
		case strings.HasSuffix(r.URL.Path, "/singer/album"):
			body = `{"status":1,"data":{"info":[{"albumid":966846,"albumname":"叶惠美","publishtime":"2003-07-31 00:00:00"}]}}`
		case strings.HasSuffix(r.URL.Path, "/singer/song"):
			body = `{"status":1,"data":{"total":1,"info":[{"hash":"HASH","filename":"周杰伦 - 晴天","album_name":"叶惠美","album_id":"966846","duration":269,"album_audio_id":32100650,"trans_param":{"union_cover":"http://img.test/{size}/cover.jpg"}}]}}`
		default:
			t.Fatalf("unexpected Kugou request: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	defer func() { client = old }()
	id, err := KugouArtistID(context.Background(), "HASH", "周杰伦")
	if err != nil || id != "3520" {
		t.Fatalf("artist id = %q, %v", id, err)
	}
	profile, err := KugouArtist(context.Background(), id)
	if err != nil || profile.Name != "周杰伦" || profile.Description != "简介" || len(profile.Albums) != 1 || !strings.HasPrefix(profile.PhotoURL, "https://") {
		t.Fatalf("profile = %+v, %v", profile, err)
	}
	page, err := KugouArtistSongs(context.Background(), id, 0)
	if err != nil || len(page.Songs) != 1 || page.Songs[0].Hash != "HASH" || page.Songs[0].AlbumMID != "966846" || page.Songs[0].ArtistMIDs[0] != id {
		t.Fatalf("songs = %+v, %v", page, err)
	}
}

// Live Migu probes cover popular, emerging, niche and overseas artists and
// exercise the nested song-list shape returned by the current mobile API.
func TestLiveMiguArtistCatalogSamples(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to probe the Migu artist catalog")
	}
	samples := []struct {
		kind string
		name string
	}{
		{"热门华语", "周杰伦"},
		{"新生华语", "于冬然"},
		{"小众华语", "声音碎片"},
		{"海外歌手", "Taylor Swift"},
		{"海外歌手", "Adele"},
	}
	for _, sample := range samples {
		t.Run(sample.kind+"/"+sample.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			rows, err := Search(ctx, "mg", sample.name, 10)
			if err != nil {
				t.Fatalf("search %q: %v", sample.name, err)
			}
			id := ""
			for _, row := range rows {
				for i, artist := range row.Artists {
					if SameArtist(artist, sample.name) && i < len(row.ArtistMIDs) {
						id = row.ArtistMIDs[i]
						break
					}
				}
				if id != "" {
					break
				}
			}
			if id == "" {
				t.Fatalf("search %q did not expose a singer ID", sample.name)
			}
			profile, err := MiguArtist(ctx, id)
			if err != nil || profile.Description == "" {
				t.Fatalf("profile %q (%s): albums=%d description=%t err=%v", sample.name, id, len(profile.Albums), profile.Description != "", err)
			}
			page, err := MiguArtistSongs(ctx, id, 0)
			if err != nil || len(page.Songs) == 0 || page.PhotoURL == "" {
				t.Fatalf("songs %q (%s): rows=%d photo=%t err=%v", sample.name, id, len(page.Songs), page.PhotoURL != "", err)
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
			t.Logf("%s: id=%s first-page=%d albums=%d/%d profile-albums=%d", sample.name, id, len(page.Songs), withAlbum, len(page.Songs), len(profile.Albums))
		})
	}
}

// Migu artist metadata, albums and songs are decoded from the native singer endpoints.
func TestMiguArtistCatalog(t *testing.T) {
	old := client
	expectedSongPage := "1"
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case strings.Contains(r.URL.Path, "/singer/index/"):
			body = `{"code":"000000","data":{"contents":[{"view":"ZJ-Singer-Intro-Scroll","contents":[{"view":"ZJ-Singer-Intro-Item","txt2":"咪咕简介"}]}]}}`
		case strings.Contains(r.URL.Path, "/singer/album/"):
			body = `{"code":"000000","data":{"contents":[{"view":"ZJ-Album-Item","resId":"7949","img":"/album.webp","txt":"七里香","txt3":"2004-08-03"}]}}`
		case strings.Contains(r.URL.Path, "/singer/song/"):
			if r.URL.Query().Get("pageNo") != expectedSongPage {
				t.Fatalf("Migu song page = %q, want %q", r.URL.Query().Get("pageNo"), expectedSongPage)
			}
			body = `{"code":"000000","data":{"header":{},"contents":[{"view":"ZJ-Singer-Song-Scroll","contents":[{"view":"ZJ-Singer-Song-Item","songItem":{"copyrightId":"60054701941","songName":"我的地盘","album":"七里香","albumId":"7949","duration":245,"img1":"/cover.webp","lrcUrl":"https://lyric.test","singerList":[{"id":"112","name":"周杰伦","img":"/artist.webp"}]}}]}]}}`
		default:
			t.Fatalf("unexpected Migu request: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	defer func() { client = old }()
	profile, err := MiguArtist(context.Background(), "112")
	if err != nil || profile.Description != "咪咕简介" || len(profile.Albums) != 1 || profile.Albums[0].ID != "7949" {
		t.Fatalf("profile = %+v, %v", profile, err)
	}
	page, err := MiguArtistSongs(context.Background(), "112", 0)
	if err != nil || len(page.Songs) != 1 || page.Songs[0].ID != "60054701941" || page.Songs[0].ArtistMIDs[0] != "112" || page.PhotoURL == "" {
		t.Fatalf("songs = %+v, %v", page, err)
	}
	expectedSongPage = "2"
	if _, err := MiguArtistSongs(context.Background(), "112", 50); err != nil {
		t.Fatal(err)
	}
}
