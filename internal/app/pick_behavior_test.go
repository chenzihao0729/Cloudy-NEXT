package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/sourceplugin"
)

// Queue handling must happen once on double click, never on the first click,
// including after merging upstream's single-click queue preference.
func TestDoubleClickQueuePreference(t *testing.T) {
	for _, search := range []bool{false, true} {
		for _, adds := range []bool{false, true} {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.ClickAdds = adds
			a.route = route{page: pagePlaylist, id: 9}
			if search {
				a.route = route{page: pageSearch, query: "Picked"}
			}
			tracks := []netease.Track{{ID: 81, Name: "Picked"}, {ID: 82, Name: "Other result"}}
			a.pl.Cue([]netease.Track{{ID: 71, Name: "Playing"}, {ID: 72, Name: "Pending"}})
			tt := ui.NewTester(func(c *ui.Context) {
				a.pal = paletteFor(c.Theme())
				a.trackRows(c, tracks, true, func(*ui.Context) {})
			}, 1200, 800)
			tt.Frame()
			tt.Advance(time.Second)
			if err := tt.Click("Picked"); err != nil {
				t.Fatal(err)
			}
			if a.pl.Current().ID != 71 || len(a.pl.Queue()) != 2 {
				t.Fatal("single click changed playback or queue")
			}
			if err := tt.Click("Picked"); err != nil {
				t.Fatal(err)
			}
			queue := a.pl.Queue()
			order, _ := a.pl.PlayOrder()
			if a.pl.Current().ID != 81 {
				t.Fatal("double click did not play picked track")
			}
			if search || adds {
				if len(order) != 3 || queue[order[0]].ID != 71 || queue[order[1]].ID != 81 || queue[order[2]].ID != 72 {
					t.Fatal("insertion lost original queue or inserted more than once")
				}
			} else if len(order) != 2 || queue[order[0]].ID != 81 || queue[order[1]].ID != 82 {
				t.Fatal("replace preference did not use the displayed list")
			}
		}
	}
}

// NetEase membership restrictions must not disable a configured plugin's
// lossless quality; disabling the plugin restores the official restriction.
func TestPluginLosslessQualityIndependentOfMembership(t *testing.T) {
	a := newTestApp()
	a.source = newSourceManager()
	a.source.configure("plugin", sourceplugin.Info{}, true)
	if a.levelLocked("lossless") != "" || a.levelLocked("hires") != "" || a.levelLocked("jymaster") == "" {
		t.Fatal("plugin quality used NetEase membership incorrectly")
	}
	a.source.setEnabled(false)
	if a.levelLocked("lossless") == "" {
		t.Fatal("official lossless lost its membership restriction")
	}
}
