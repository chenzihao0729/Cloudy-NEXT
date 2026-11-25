package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/egoist/mygo/ui"
	"image/png"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/sourcecatalog"
	"netease-music-mygo/internal/sourceplugin"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Top cross-provider matches must precede a block of unmarked NetEase covers.
func TestSearchHongYanRuShuangConsensus(t *testing.T) {
	var rows []netease.Track
	for i := range 9 {
		rows = append(rows, netease.Track{ID: int64(i + 1), Name: "红颜如霜", SearchOrder: i, Artists: []netease.Artist{{Name: "翻唱歌手"}}})
	}
	jay := providerTrack("tx", sourcecatalog.Song{ID: "jay", Name: "红颜如霜", Artists: []string{"周杰伦"}})
	jay.Sources = []netease.SourceTrack{{Platform: "kg", ID: "jay-kg", SearchOrder: 0}}
	rows = append(rows, jay)
	sortSearchBatch("红颜如霜", rows)
	if rows[0].ArtistNames() != "周杰伦" {
		t.Fatalf("top result: %s", rows[0].ArtistNames())
	}
	// Single-provider ties retain the requested platform precedence.
	rows = []netease.Track{providerTrack("tx", sourcecatalog.Song{ID: "x", Name: "歌"}), {ID: 1, Name: "歌"}}
	sortSearchBatch("歌", rows)
	if rows[0].Source != nil {
		t.Fatal("equal results lost NetEase priority")
	}
}

// NetEase identity must win even when its response arrives after an external provider.
func TestSearchMergePrefersNetEaseIdentity(t *testing.T) {
	tx := providerTrack("tx", sourcecatalog.Song{ID: "qq", Name: "歌曲", Artists: []string{"歌手"}, ArtistMIDs: []string{"singer-mid"}, Album: "专辑", Duration: 200 * time.Second})
	wy := netease.Track{ID: 123, Name: "歌曲", Artists: []netease.Artist{{ID: 456, Name: "歌手"}}, Album: netease.Album{Name: "专辑"}, Duration: 200000}
	for _, incoming := range [][]netease.Track{{tx, wy}, {wy, tx}} {
		rows := mergeSearchRecordings(nil, incoming)
		if len(rows) != 1 || rows[0].ID != wy.ID || rows[0].Source != nil || len(searchTrackSources(rows[0])) != 2 || !commentable(&rows[0]) || rows[0].Artists[0].SourceMID != "singer-mid" {
			t.Fatalf("lost NetEase identity or provider fallback: %+v", rows)
		}
	}
}

// Duplicate recordings merge across pages while different lengths and editions remain separate.
func TestSearchRecordingMerge(t *testing.T) {
	song := sourcecatalog.Song{ID: "qq", Name: "最近还好吗", Artists: []string{"S.H.E"}, Album: "电视原声带", Duration: 239 * time.Second}
	tx := providerTrack("tx", song)
	song.ID = "kg"
	kg := providerTrack("kg", song)
	rows := mergeSearchRecordings(nil, []netease.Track{tx, kg})
	if len(rows) != 1 || len(searchTrackSources(rows[0])) != 2 || rows[0].ID != tx.ID {
		t.Fatal("duplicate recording not merged stably")
	}
	rows = mergeSearchRecordings(rows, []netease.Track{kg})
	if len(rows) != 1 || len(searchTrackSources(rows[0])) != 2 {
		t.Fatal("pagination duplicated provider")
	}
	song.Duration = 251 * time.Second
	song.ID = "long"
	rows = mergeSearchRecordings(rows, []netease.Track{providerTrack("kg", song)})
	if len(rows) != 2 {
		t.Fatal("different recording duration merged")
	}
	song.Duration = 239 * time.Second
	song.Name += " (Live)"
	song.ID = "live"
	rows = mergeSearchRecordings(rows, []netease.Track{providerTrack("kg", song)})
	if len(rows) != 3 {
		t.Fatal("live edition merged")
	}
}

// A failed provider preserves successful results and retries its own page without duplicating rows.
// Catalog subtitles previously split the same Mayday recording into separate rows.
func TestSearchRecordingSubtitleMerge(t *testing.T) {
	base := sourcecatalog.Song{ID: "kg", Name: "伤心的人别听慢歌", Artists: []string{"五月天"}, Album: "步步 自选作品辑 the Best of 1999-2013", Duration: 215 * time.Second}
	kg := providerTrack("kg", base)
	qq := base
	qq.ID, qq.Name = "qq", "伤心的人别听慢歌（贯彻快乐）"
	qq.Duration += time.Second
	rows := mergeSearchRecordings(nil, []netease.Track{kg, providerTrack("tx", qq)})
	if len(rows) != 1 || len(searchTrackSources(rows[0])) != 2 {
		t.Fatal("same recording with subtitle was not merged with both sources")
	}
	for _, tc := range []struct {
		name   string
		change func(*sourcecatalog.Song)
	}{
		{"artist", func(s *sourcecatalog.Song) { s.Artists = []string{"其他歌手"} }},
		{"album", func(s *sourcecatalog.Song) { s.Album = "另一张专辑" }},
		{"duration", func(s *sourcecatalog.Song) { s.Duration += 10 * time.Second }},
		{"missing duration", func(s *sourcecatalog.Song) { s.Duration = 0 }},
		{"live", func(s *sourcecatalog.Song) { s.Name = "伤心的人别听慢歌（Live）" }},
		{"remaster", func(s *sourcecatalog.Song) { s.Name = "伤心的人别听慢歌（Remastered）" }},
		{"different song", func(s *sourcecatalog.Song) { s.Name = "伤心的人别听慢歌续集" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := qq
			tc.change(&s)
			if sameSearchRecording(kg, providerTrack("tx", s)) {
				t.Fatal("different or uncertain recording merged")
			}
		})
	}
	if sameSearchTitle("歌（上）", "歌（下）") {
		t.Fatal("different subtitles merged")
	}
}

// A failed provider preserves successful results and retries its own page without duplicating rows.
func TestMultiSearchPartialFailureAndPagination(t *testing.T) {
	d := &searchData{multi: &multiSearch{providers: map[string]*providerSearch{"wy": {next: 1, more: true}, "tx": {next: 1, more: true}}, seen: map[int64]bool{}}}
	requests := []searchRequestPage{{"wy", 1}, {"tx", 1}}
	tx := providerTrack("tx", sourcecatalog.Song{ID: "mid", Name: "song"})
	applySearchAnswers("song", d, requests, map[string]searchAnswer{"wy": {err: errors.New("offline")}, "tx": {songs: []netease.Track{tx}}})
	if len(d.res.Songs) != 1 || d.multi.providers["wy"].next != 1 || d.multi.providers["tx"].next != 2 || d.page.err == "" {
		t.Fatal("partial results or retry cursor lost")
	}
	applySearchAnswers("song", d, requests, map[string]searchAnswer{"wy": {songs: []netease.Track{{ID: 1, Name: "song"}}}, "tx": {songs: []netease.Track{tx}}})
	if len(d.res.Songs) != 2 || d.res.Songs[0].ID != tx.ID {
		t.Fatal("pagination duplicated or moved existing rows")
	}
}

// Provider songs retain distinct identities and resolution fields after queue serialization.
func TestProviderTrackIdentitySurvivesSession(t *testing.T) {
	song := sourcecatalog.Song{ID: "123", MID: "mid", MediaMID: "media", Hash: "hash", Name: "song"}
	tx, kg := providerTrack("tx", song), providerTrack("kg", song)
	if tx.ID >= 0 || tx.ID == kg.ID || tx.ID != providerTrack("tx", song).ID {
		t.Fatal("provider identities collide or change")
	}
	data, err := json.Marshal(tx)
	if err != nil {
		t.Fatal(err)
	}
	var restored netease.Track
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Source == nil || *restored.Source != *tx.Source || restored.ID != tx.ID {
		t.Fatal("queue lost provider resolution fields")
	}
}

// An exact external recording precedes an explicit cover while equal matches keep provider order.
func TestMultiSearchRanking(t *testing.T) {
	song := func(p, name string) netease.Track {
		if p == "wy" {
			return netease.Track{ID: 1, Name: name}
		}
		return providerTrack(p, sourcecatalog.Song{ID: p, Name: name})
	}
	songs := []netease.Track{song("wy", "最近还好吗 (Cover S.H.E)"), song("kg", "最近还好吗"), song("tx", "最近还好吗")}
	sortSearchBatch("最近还好吗", songs)
	if songs[0].Source.Platform != "tx" || songs[1].Source.Platform != "kg" || songs[2].Source != nil {
		t.Fatal("cover or platform priority was wrong")
	}
	live := []netease.Track{song("wy", "trust me"), song("tx", "trust me (Live)")}
	sortSearchBatch("trust me Live", live)
	if live[0].Source == nil {
		t.Fatal("explicit live query did not prefer live version")
	}
}

// A provider row double-click plays its original identity without exposing NetEase-only actions.
// Explicit same-title cover credits promote the credited performer across providers.
func TestSearchCreditsPromoteCreditedArtist(t *testing.T) {
	songs := []netease.Track{
		{ID: 1, Name: "最近还好吗", Artists: []netease.Artist{{Name: "大飛"}}, Album: netease.Album{Name: "翻唱系列"}},
		{ID: 2, Name: "最近还好吗 (Cover S.H.E)"},
		providerTrack("tx", sourcecatalog.Song{ID: "original", Name: "最近还好吗", Artists: []string{"S.H.E"}}),
		{ID: 3, Name: "最近还好吗", Artists: []netease.Artist{{Name: "其他歌手"}}},
	}
	sortSearchBatch("最近还好吗", songs)
	if songs[0].Source == nil || songs[0].Source.ID != "original" {
		t.Fatal("explicit credited performer remained below uncredited versions")
	}
}

// Provider rows reserve the same trailing action space so album columns align.
func TestSearchAlbumColumnsAlignAcrossProviders(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.route = route{page: pageSearch}
	tracks := []netease.Track{{ID: 1, Name: "NetEase", Album: netease.Album{Name: "Album WY"}}, providerTrack("tx", sourcecatalog.Song{ID: "mid", Name: "QQ", Album: "Album TX", Artists: []string{"A long artist name"}})}
	tt := ui.NewTester(func(c *ui.Context) { a.pal = paletteFor(c.Theme()); a.trackRows(c, tracks, true, func(*ui.Context) {}) }, 1200, 800)
	tt.Frame()
	wy, ok := tt.Find("Album WY")
	if !ok {
		t.Fatal("missing NetEase album")
	}
	tx, ok := tt.Find("Album TX")
	if !ok {
		t.Fatal("missing QQ album")
	}
	if wy.X != tx.X {
		t.Fatalf("album column differs: %v vs %v", wy.X, tx.X)
	}
}

// A provider row double-click plays its original identity without exposing NetEase-only actions.
func TestProviderSearchRowPlays(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.source = newSourceManager()
	a.source.configure("plugin", sourceplugin.Info{}, true)
	track := providerTrack("tx", sourcecatalog.Song{ID: "qq-mid", MID: "qq-mid", Name: "External song", Artists: []string{"Singer"}, Duration: time.Minute})
	a.route = route{page: pageSearch, query: "External song", kind: netease.SearchSongs}
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		a.trackRows(c, []netease.Track{track}, true, func(*ui.Context) {})
	}, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	if dir := os.Getenv("NM_FRAMES"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "multi-search.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	a.pl.Cue([]netease.Track{{ID: 71, Name: "Queued first"}, {ID: 72, Name: "Queued next"}})
	if err := tt.Click(track.Name); err != nil {
		t.Fatal(err)
	}
	if a.pl.Current() == nil || a.pl.Current().ID != 71 {
		t.Fatal("single click played song")
	}
	if err := tt.Click(track.Name); err != nil {
		t.Fatal(err)
	}
	if current := a.pl.Current(); current == nil || current.Source == nil || current.Source.ID != "qq-mid" {
		t.Fatal("double click lost provider song")
	}
	// Merging upstream search insertion must keep double-click playback
	// and the existing queue, including provider identities.
	queue := a.pl.Queue()
	order, _ := a.pl.PlayOrder()
	if len(queue) != 3 || len(order) != 3 || queue[order[0]].ID != 71 || queue[order[1]].ID != track.ID || queue[order[2]].ID != 72 {
		t.Fatal("search insertion replaced or reordered the existing queue")
	}
	if len(a.playlistActions(&track, 0)) != 0 {
		t.Fatal("external song exposes NetEase playlist action")
	}
	a.source.setEnabled(false)
	if a.canPlayTrack(track) {
		t.Fatal("external song enabled without plugin")
	}
}

// Direct provider playback uses its own ID and hash without an intervening catalog search.
func TestProviderResolveUsesOriginalIdentity(t *testing.T) {
	m := newSourceManager()
	script := `module.exports={sources:{kg:{name:"酷狗"}},musicUrl:async(source,info)=>"https://audio.example/"+info.hash};`
	info, err := sourceplugin.Validate(script)
	if err != nil {
		t.Fatal(err)
	}
	m.configure(script, info, true)
	defer m.close()
	track := providerTrack("kg", sourcecatalog.Song{ID: "77", Hash: "original-hash", Name: "song"})
	stream, err := m.Resolve(context.Background(), track, "exhigh", nil)
	if err != nil || stream.URL != "https://audio.example/original-hash" || stream.SourceID != "original-hash" {
		t.Fatalf("direct resolve: %+v %v", stream, err)
	}
}
