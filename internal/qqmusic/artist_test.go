package qqmusic

import (
	"context"
	"os"
	"testing"
)

// The QQ singer endpoint returns playable recording identities for a real singer MID.
func TestQQArtistLive(t *testing.T) {
	if os.Getenv("LIVE_QQ_ARTIST") == "" {
		t.Skip("set LIVE_QQ_ARTIST")
	}
	page, err := ArtistSongs(context.Background(), "0025NhlN2yWrP4", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Songs) == 0 {
		t.Fatal("no singer songs returned")
	}
	t.Logf("%d songs, total %d, first %s", len(page.Songs), page.Total, page.Songs[0].Name)
	profile, err := SingerProfile(context.Background(), "0025NhlN2yWrP4")
	if err != nil || profile.Description == "" || len(profile.Albums) == 0 {
		t.Fatalf("missing profile/albums: %+v %v", profile, err)
	}
}

// A QQ album loads its details and song identifiers for in-app playback.
func TestQQAlbumLive(t *testing.T) {
	if os.Getenv("LIVE_QQ_ARTIST") == "" {
		t.Skip("set LIVE_QQ_ARTIST")
	}
	al, err := AlbumDetail(context.Background(), "0041WVfh2vtlJE")
	if err != nil {
		t.Fatal(err)
	}
	if al.Name == "" || len(al.Artists) == 0 || len(al.Songs) == 0 || al.Songs[0].MID == "" || len(al.Songs[0].ArtistMIDs) == 0 {
		t.Fatalf("incomplete album: %+v", al)
	}
	t.Logf("%s: %d songs", al.Name, len(al.Songs))
}
