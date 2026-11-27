package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/sourcecatalog"
	"netease-music-mygo/internal/sourceplugin"
)

// A catalog search that cannot find a parenthesized language label retries
// the base title and resolves the otherwise identical recording.
func TestPluginSearchFallsBackFromLanguageEditionSuffix(t *testing.T) {
	realSearch := searchSource
	defer func() { searchSource = realSearch }()
	var queries []string
	searchSource = func(_ context.Context, platform, query string, _ int) ([]sourcecatalog.Song, error) {
		queries = append(queries, platform+":"+query)
		if query == "你的意义 歌手" {
			return []sourcecatalog.Song{{ID: "tx-id", MID: "tx-id", Name: "你的意义", Artists: []string{"歌手"}, Album: "你的意义", Duration: 4 * time.Minute}}, nil
		}
		return nil, nil
	}
	m := newSourceManager()
	script := `module.exports={sources:{tx:{name:"QQ音乐"}},musicUrl:async(source,info)=>"https://audio.example/"+info.id};`
	info, err := sourceplugin.Validate(script)
	if err != nil {
		t.Fatal(err)
	}
	m.configure(script, info, true)
	defer m.close()
	track := netease.Track{ID: 7, Name: "你的意义(中文版)", Artists: []netease.Artist{{Name: "歌手"}}, Album: netease.Album{Name: "你的意义"}, Duration: 240_000}
	stream, err := m.Resolve(context.Background(), track, "exhigh", nil)
	if err != nil || stream.URL != "https://audio.example/tx-id" {
		t.Fatalf("fallback resolve = %+v, %v; queries %q", stream, err, queries)
	}
	if !strings.Contains(strings.Join(queries, "\n"), "tx:你的意义 歌手") {
		t.Fatalf("base-title query was not tried: %q", queries)
	}
}

// Source preference follows login and VIP status while respecting the source toggle.
func TestPreferSourceByAccount(t *testing.T) {
	track := netease.Track{ID: 1, Fee: 1}
	a := &app{source: newSourceManager()}
	a.source.configure("plugin", sourceplugin.Info{}, true)
	if !a.preferSource(track) || !a.canPlayTrack(track) || a.preferSource(netease.Track{ID: 2}) {
		t.Fatal("logged-out playback should use the source only for VIP tracks")
	}
	a.profile = &netease.Profile{VIPType: 0}
	if !a.preferSource(track) || a.preferSource(netease.Track{ID: 2}) || !a.canPlayTrack(track) {
		t.Fatal("a free account should prefer the source only for VIP tracks")
	}
	a.profile = &netease.Profile{VIPType: 11}
	if a.preferSource(track) || !a.canPlayTrack(track) {
		t.Fatal("a VIP account should keep using NetEase")
	}
	a.source.setEnabled(false)
	a.profile = nil
	if a.preferSource(track) || a.canPlayTrack(track) {
		t.Fatal("a disabled source should not be preferred")
	}
}

// A song removed from NetEase is still enabled when an alternate source exists.
func TestPreferSourceForNoCopyright(t *testing.T) {
	track := netease.Track{ID: 7, Privilege: &netease.Privilege{St: -200}}
	a := &app{source: newSourceManager()}
	a.source.configure("plugin", sourceplugin.Info{}, true)
	if !a.preferSource(track) || !a.canPlayTrack(track) {
		t.Fatal("no-copyright track should use the installed alternate source")
	}
	a.profile = &netease.Profile{VIPType: 11}
	if !a.preferSource(track) || !a.canPlayTrack(track) {
		t.Fatal("a VIP account should still use alternate sources for no-copyright tracks")
	}
}

// A matched platform's ID replaces the NetEase ID before invoking its plugin source.
func TestPluginMusicInfoUsesMatchedPlatformIdentity(t *testing.T) {
	track := netease.Track{ID: 123, Name: "Can't go back"}
	info := pluginMusicInfo(track, "Top Barry", "kg", sourcecatalog.Song{ID: "kg-id", Hash: "kg-hash", Name: track.Name, Artists: []string{"Top Barry"}})
	if info["id"] != "kg-id" || info["songId"] != "kg-id" || info["hash"] != "kg-hash" {
		t.Fatalf("plugin info = %#v", info)
	}
	if _, exists := info["songmid"]; exists {
		t.Fatalf("Kugou plugin info contains a QQ/NetEase songmid: %#v", info)
	}

	qq := pluginMusicInfo(track, "Top Barry", "tx", sourcecatalog.Song{ID: "qq-mid", MID: "qq-mid", Name: track.Name, Artists: []string{"Top Barry"}})
	if qq["id"] != "qq-mid" || qq["songmid"] != "qq-mid" {
		t.Fatalf("QQ plugin info = %#v", qq)
	}
	if _, exists := qq["hash"]; exists {
		t.Fatalf("QQ plugin info contains an empty Kugou hash that masks songmid: %#v", qq)
	}
}
