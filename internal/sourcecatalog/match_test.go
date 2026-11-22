package sourcecatalog

import (
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// A result is accepted only when title, artist, version and duration identify the intended track.
func TestMatchRejectsWrongVersionsAndSongs(t *testing.T) {
	track := netease.Track{
		Name: "晴天", Duration: 269000,
		Artists: []netease.Artist{{Name: "周杰伦"}},
		Album:   netease.Album{Name: "叶惠美"},
	}
	songs := []Song{
		{ID: "live", Name: "晴天 Live", Artists: []string{"周杰伦"}, Album: "叶惠美", Duration: 269 * time.Second},
		{ID: "wrong", Name: "晴天", Artists: []string{"其他歌手"}, Album: "叶惠美", Duration: 269 * time.Second},
		{ID: "right", Name: "晴天", Artists: []string{"周杰伦"}, Album: "叶惠美", Duration: 269 * time.Second},
	}
	got, ok := Match(track, songs)
	if !ok || got.ID != "right" {
		t.Fatalf("Match() = %#v, %v; want right version", got, ok)
	}
}

// Live edition suffixes and combined artist fields match equivalent metadata from another platform.
func TestMatchNormalizesLiveEditionAndArtistSeparators(t *testing.T) {
	track := netease.Track{
		Name: "trust me (Live版)", Duration: 222325,
		Artists: []netease.Artist{{Name: "姚晓棠"}, {Name: "Top Barry"}},
		Album:   netease.Album{Name: "天赐的声音第七季 第8期"},
	}
	songs := []Song{{
		ID: "live-id", Name: "trust me (Live)", Artists: []string{"姚晓棠/Top Barry"},
		Album: track.Album.Name, Duration: 222 * time.Second,
	}}
	got, ok := Match(track, songs)
	if !ok || got.ID != "live-id" {
		t.Fatalf("Match() = %#v, %v; want the exact live-version match", got, ok)
	}
}

// Punctuation and whitespace normalize, while mismatched artist credits require album and duration evidence.
func TestMatchDifferentArtistCreditUsesAlbumAndDuration(t *testing.T) {
	track := netease.Track{
		Name: "爱，请问怎么走", Duration: 260_000,
		Artists: []netease.Artist{{Name: "黄丽玲"}},
		Album:   netease.Album{Name: "寂寞不痛"},
	}
	songs := []Song{
		{ID: "wrong-album", Name: "爱 请问怎么走", Artists: []string{"A-Lin"}, Album: "其他专辑", Duration: 260 * time.Second},
		{ID: "wrong-duration", Name: "爱,请问怎么走", Artists: []string{"A-Lin"}, Album: "寂寞不痛", Duration: 280 * time.Second},
		{ID: "right", Name: "爱 请问怎么走", Artists: []string{"A-Lin"}, Album: "寂寞不痛", Duration: 260 * time.Second},
	}
	got, ok := Match(track, songs)
	if !ok || got.ID != "right" {
		t.Fatalf("Match() = %#v, %v; want album and duration-confirmed candidate", got, ok)
	}
}
