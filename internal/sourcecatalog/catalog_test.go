package sourcecatalog

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// Cross-platform matching rejects version and artist mismatches before using platform identifiers.
func TestMatchRejectsWrongVersionAndArtist(t *testing.T) {
	track := netease.Track{Name: "Can't go back", Duration: 210_000, Artists: []netease.Artist{{Name: "Top Barry"}}, Album: netease.Album{Name: "Album"}}
	songs := []Song{
		{ID: "live-id", Hash: "live-hash", Name: "Can't go back Live", Artists: []string{"Top Barry"}, Album: "Album", Duration: 210 * time.Second},
		{ID: "wrong-id", Hash: "wrong-hash", Name: "Can't go back", Artists: []string{"Other"}, Album: "Album", Duration: 210 * time.Second},
		{ID: "right-id", Hash: "right-hash", Name: "Can't go back", Artists: []string{"Top Barry"}, Album: "Album", Duration: 210 * time.Second},
	}
	got, ok := Match(track, songs)
	if !ok || got.ID != "right-id" || got.Hash != "right-hash" {
		t.Fatalf("Match() = %#v, %v; want the exact-version platform identity", got, ok)
	}
}

// Matching accepts an equivalent Live版 title and platform artist separators while preserving versions.
func TestMatchTrustMeLiveAcrossArtistSeparators(t *testing.T) {
	track := netease.Track{
		Name: "trust me (Live版)", Duration: 222_325,
		Artists: []netease.Artist{{Name: "姚晓棠"}, {Name: "Top Barry"}},
		Album:   netease.Album{Name: "天赐的声音第七季 第8期"},
	}
	songs := []Song{
		{ID: "wrong-version", Name: "trust me", Artists: []string{"姚晓棠/Top Barry"}, Album: track.Album.Name, Duration: 222 * time.Second},
		{ID: "live-id", MID: "live-mid", Name: "trust me (Live)", Artists: []string{"姚晓棠/Top Barry"}, Album: track.Album.Name, Duration: 222 * time.Second},
	}
	got, ok := Match(track, songs)
	if !ok || got.ID != "live-id" || got.MID != "live-mid" {
		t.Fatalf("Match() = %#v, %v; want the matching Live recording", got, ok)
	}
}

// The QQ result shown for MODYSSEY's track matches NetEase by title, artist, album and duration.
func TestMatchMODYSSEYTrackFromScreenshots(t *testing.T) {
	track := netease.Track{
		Name: "타! (CAN YOU HEAR THAT)", Duration: 187_000,
		Artists: []netease.Artist{{Name: "MODYSSEY"}},
		Album:   netease.Album{Name: "2.Multitude: How We Bloom"},
	}
	songs := []Song{{
		ID: "0025bPhf3H0kqc", MID: "0025bPhf3H0kqc", Name: "타! (CAN YOU HEAR THAT)",
		Artists: []string{"MODYSSEY"}, Album: "2.Multitude: How We Bloom", Duration: 187 * time.Second,
	}}
	got, ok := Match(track, songs)
	if !ok || got.MID != "0025bPhf3H0kqc" {
		t.Fatalf("Match() = %#v, %v; want the QQ result visible in the screenshots", got, ok)
	}
}

// Kugou search uses the provider's hash and platform song ID, not the NetEase ID.
func TestKugouSearchReturnsPlatformIdentity(t *testing.T) {
	oldClient := client
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("keyword") != "Can't go back Top Barry" || r.URL.Query().Get("format") != "json" {
			t.Fatalf("Kugou query = %s", r.URL.RawQuery)
		}
		body := `{"status":1,"data":{"info":[{"hash":"kugou-hash","songname":"Can't go back","singername":"Top Barry","album_name":"Album","duration":204,"album_audio_id":7654321}]}}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	t.Cleanup(func() { client = oldClient })
	songs, err := (kugouSearcher{}).Search(context.Background(), "Can't go back Top Barry", 10)
	if err != nil || len(songs) != 1 {
		t.Fatalf("Search() = %#v, %v", songs, err)
	}
	if songs[0].Hash != "kugou-hash" || songs[0].ID != "7654321" || songs[0].Duration != 204*time.Second {
		t.Fatalf("Kugou song = %#v", songs[0])
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
