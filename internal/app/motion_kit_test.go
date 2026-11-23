package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// A hover's colors take motionHover, never a time of their own.
func TestHoversTakeTheSet(t *testing.T) {
	own := regexp.MustCompile(`AnimateColor\("[a-z]+", [^)]*\), ui\.Animation\{Duration: 1[24]0 \* time\.Millisecond\}\)|AnimateColor\("[a-z]+", [\w.]+, ui\.Animation\{Duration: 1[24]0 \* time\.Millisecond\}\)`)
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, m := range own.FindAllString(string(b), -1) {
			t.Errorf("%s: %s takes a time of its own", f, m)
		}
	}
}

// A pane grows from what opened it to where it opens; with nothing noted,
// from a little of itself round its middle; lite, it is where it opens.
func TestGrowRect(t *testing.T) {
	from := ui.Rect{X: 900, Y: 700, W: 32, H: 32}
	full := ui.Rect{X: 500, Y: 100, W: 400, H: 560}
	s := growSpec{from: from, fromRadius: 14, radius: 20}
	if r, rad := growRect(s, full, false); r != from || rad != 14 {
		t.Errorf("at 0 it is %v rounded %v, not the button %v", r, rad, from)
	}
	s.k = 1
	if r, rad := growRect(s, full, false); r != full || rad != 20 {
		t.Errorf("at 1 it is %v rounded %v, not open at %v", r, rad, full)
	}
	s.k, s.from = 0, ui.Rect{}
	if r, _ := growRect(s, full, false); r.W != full.W*0.86 || r.X+r.W/2 != full.X+full.W/2 {
		t.Errorf("from nothing it starts at %v, not 0.86 of itself round its middle", r)
	}
	s.from = from
	if r, _ := growRect(s, full, true); r != full {
		t.Errorf("lite, it grows from %v rather than show where it opens", r)
	}
}

// A dialog flies out of what opened it only when that was noted as it
// opened: not out of a control used long before.
func TestGrowSourceIsFresh(t *testing.T) {
	a := newTestApp()
	a.growFromRect("settings", ui.Rect{X: 1, Y: 2, W: 30, H: 30}, 14)
	if _, _, ok := a.growSource("settings", true); !ok {
		t.Fatal("opened just after its gear, it does not fly from it")
	}
	if _, _, ok := a.growSource("settings", true); !ok {
		t.Fatal("open, it forgot where it flew from")
	}
	a.growSource("settings", false)
	a.growKeep("settings").at = time.Now().Add(-2 * growFresh)
	if _, _, ok := a.growSource("settings", true); ok {
		t.Error("opened by a shortcut, it flew from the gear clicked long before")
	}
}

// The menu grows out of the button that opened it, takes no press while it
// still sits over that button, and goes back into it as it closes.
func TestMenuGrowsOutOfItsButton(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "宝船", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	more, _ := tt.Find("更多")
	if err := tt.Click("更多"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.menu.open {
		t.Fatal("the dots did not open the menu")
	}
	if f := a.menu.from; abs(f.X-more.X) > 1 || abs(f.Y-more.Y) > 1 || abs(f.W-more.W) > 1 {
		t.Errorf("the menu grows from %v, not its button %v", f, more)
	}
	settle(tt)
	tt.ClickAt(5, 5)
	tt.Frame()
	if a.menu.open {
		t.Fatal("a press outside left it open")
	}
	if !tt.HasText("复制歌名") {
		t.Error("closing, the menu went at once rather than back into its button")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if tt.HasText("复制歌名") {
		t.Error("the menu stayed after it closed")
	}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// The settings fly out of the gear that opened them.
func TestSettingsFlyFromTheGear(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.settingsView.open {
		t.Fatal("the gear did not open the settings")
	}
	g := a.growKeep("settings")
	gear, _ := tt.Find("设置")
	if !g.use || abs(g.from.X-gear.X) > 1 || abs(g.from.Y-gear.Y) > 1 {
		t.Errorf("the settings fly from %v (%v), not the gear at %v", g.from, g.use, gear)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if !tt.HasText("外观") && !tt.HasText("通用") {
		t.Errorf("the settings did not come in: %q", tt.Texts())
	}
}

// What comes in after its first frame starts out of sight, and shows once
// the frames after have moved it in; one there before shows at once.
func TestEnterAfterPaint(t *testing.T) {
	a := newTestApp()
	a.motion = motionFull
	tt := ui.NewTester(func(c *ui.Context) {
		box := ui.Box(c.Key("in")).Size(100, 100).Background(ui.RGB(255, 0, 0))
		a.enterAfterPaint(box, ui.Pose{Fade: 1, Y: 6}, motionPush)
	}, 200, 200)
	tt.Frame()
	if px := tt.Image().RGBAAt(50, 50); px.G < 200 {
		t.Errorf("its first frame shows it at %v, not out of sight", px)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if px := tt.Image().RGBAAt(50, 50); px.R < 240 || px.G > 20 {
		t.Errorf("after a second it shows at %v, not in", px)
	}
}

// Content fades in over a skeleton only where the page showed one.
func TestLoadedInFollowsTheSkeleton(t *testing.T) {
	a := newTestApp()
	a.motion = motionFull
	a.route = route{page: pagePlaylist, id: 1}
	shown := false
	tt := ui.NewTester(func(c *ui.Context) {
		if !shown {
			a.sawSkeleton()
			return
		}
		box := ui.Box(c.Key("head")).Size(100, 100).Background(ui.RGB(255, 0, 0))
		a.loadedIn(box, 0)
	}, 200, 200)
	tt.Frame()
	shown = true
	tt.Frame()
	if px := tt.Image().RGBAAt(50, 50); px.G < 200 {
		t.Errorf("after a skeleton, the content shows at once: %v", px)
	}
	a.route = route{page: pagePlaylist, id: 2}
	tt2 := ui.NewTester(func(c *ui.Context) {
		box := ui.Box(c.Key("head")).Size(100, 100).Background(ui.RGB(255, 0, 0))
		a.loadedIn(box, 0)
	}, 200, 200)
	tt2.Frame()
	if px := tt2.Image().RGBAAt(50, 50); px.G > 20 {
		t.Errorf("with no skeleton before, the content faded in: %v", px)
	}
}

// Its menu open, a row keeps its hover: only the row it was opened for.
func TestMenuHeld(t *testing.T) {
	a := newTestApp()
	var rowID uint64
	var heldRow, heldOther bool
	tt := ui.NewTester(func(c *ui.Context) {
		row := ui.Box(c.Key("row")).Size(100, 40)
		other := ui.Box(c.Key("other")).Size(100, 40)
		rowID = row.ID()
		heldRow, heldOther = a.menuHeld(row), a.menuHeld(other)
	}, 200, 200)
	tt.Frame()
	a.menu = paneMenu{open: true, source: rowID}
	tt.Frame()
	if !heldRow || heldOther {
		t.Errorf("held: the row %v, the other %v", heldRow, heldOther)
	}
	a.menu.open = false
	tt.Frame()
	if heldRow {
		t.Error("the row kept its hover as the menu closed")
	}
}

// Held, a switch's knob is wider, toward where it would go, and its far
// side stays where it was.
func TestSwitchKnobStretches(t *testing.T) {
	r := ui.Rect{X: 0, Y: 0, W: 44, H: 26}
	off, offHeld := switchKnob(r, 2, 0, 0), switchKnob(r, 2, 0, switchStretch)
	if offHeld.W != off.W+switchStretch || offHeld.X != off.X {
		t.Errorf("off and held, the knob is %v, not %v stretched to the right", offHeld, off)
	}
	on, onHeld := switchKnob(r, 2, 1, 0), switchKnob(r, 2, 1, switchStretch)
	if onHeld.W != on.W+switchStretch || onHeld.X+onHeld.W != on.X+on.W {
		t.Errorf("on and held, the knob is %v, not %v stretched to the left", onHeld, on)
	}
}

// A dialog opened from a menu's row, a chip or a settings button, which
// note no source of their own, flies out of the control clicked just
// before; not out of one clicked long before.
func TestDialogFliesFromWhatWasPressed(t *testing.T) {
	a := newTestApp()
	a.mk.pressed = growKeep{from: ui.Rect{X: 10, Y: 20, W: 200, H: 38}, radius: 14, at: time.Now()}
	if r, _, ok := a.growSource("playlist-name", true); !ok || r.X != 10 || r.W != 200 {
		t.Errorf("opened from a menu's row, it flies from %v (%v)", r, ok)
	}
	a.growSource("playlist-name", false)
	a.mk.pressed.at = time.Now().Add(-2 * pressFresh)
	a.growKeep("playlist-name").at = time.Now().Add(-2 * growFresh)
	if _, _, ok := a.growSource("playlist-name", true); ok {
		t.Error("opened later, it flew from the row clicked before")
	}
}

// A dialog that builds nothing while closed, as the shortcut editor did,
// comes in again each time it opens, not only the first.
func TestDialogOpensAgainWithMotion(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	for round := range 3 {
		a.growFromRect("effects", ui.Rect{X: 900, Y: 400, W: 60, H: 30}, 14)
		a.openEffects()
		tt.Frame()
		g := a.growKeep("effects")
		if g.opens != round+1 || !g.use {
			t.Fatalf("round %d: it opens at once rather than fly (seen opening %d times, from its button %v)", round, g.opens, g.use)
		}
		tt.Advance(time.Second)
		tt.Frame()
		a.effects.open = false
		tt.Frame()
		if !tt.HasText("音效") {
			t.Errorf("round %d: closing, it went at once rather than fly back", round)
		}
		tt.Advance(time.Second)
		tt.Frame()
	}
}

// A select near the foot of the window, with room under it for a part of
// its list, shows its list over its button as it grows, not under it first, and its list goes back into the button as
// it closes rather than at once.
func TestSelectGrowsWhereItShowsAndGoesBack(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	chosen := "标准"
	options := []string{"标准", "较高", "极高", "无损"}
	tt := ui.NewTester(func(c *ui.Context) {
		a.mk.frame++
		a.motion = a.motionLevel(c)
		ui.Column(c).Width(400).Height(400).Justify(ui.End).Padding(10, 10, 90, 10).Children(func() {
			a.paneSelect(c.Key("q"), "音质", &chosen, options)
		})
	}, 400, 400)
	tt.Frame()
	button, _ := tt.Find("音质")
	if err := tt.Click("音质"); err != nil {
		t.Fatal(err)
	}
	g := a.growKeep("select:音质")
	for i := range 12 {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
		if g.full.W > 0 && g.full.Y+g.full.H > button.Y+1 {
			t.Fatalf("frame %d: the list is at %v, under or over its button %v, rather than above it", i, g.full, button)
		}
	}
	settle(tt)
	if err := tt.Click("极高"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if chosen != "极高" {
		t.Fatalf("chose %q", chosen)
	}
	if g.ghost != a.mk.frame {
		t.Error("closing, the list went at once rather than back into its button")
	}
	if tt.HasText("无损") {
		t.Error("going, the list still takes the pointer")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if g.ghost == a.mk.frame {
		t.Error("the list stayed after it closed")
	}
}

// The calendar's row in the settings opens its drawer wherever it is
// clicked, not only on its chevron.
func TestSettingsRowOpensAsAWhole(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1200, 800)
	a.openSettings()
	a.settingsView.query = "台历"
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	name, ok := tt.Find("台历内容")
	if !ok {
		t.Fatal("no row of the calendar")
	}
	tt.ClickAt(name.X+4, name.Y+name.H/2)
	tt.Frame()
	if !a.deckDrawer {
		t.Fatal("a click on its name did not open the drawer")
	}
	if _, ok := tt.Find("收起台历内容"); !ok {
		t.Error("open, the row is not called 收起台历内容")
	}
}

// A select's highlight rises into a lens as it goes to the option under the
// pointer, and is down again once there: no lens over a highlight at rest.
func TestSelectLensLandsWithTheHighlight(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	chosen := "标准"
	options := []string{"标准", "较高", "极高", "无损"}
	tt := ui.NewTester(func(c *ui.Context) {
		a.mk.frame++
		a.motion = a.motionLevel(c)
		ui.Column(c).Width(400).Height(400).Padding(10).Children(func() {
			a.paneSelect(c.Key("q"), "音质", &chosen, options)
		})
	}, 400, 400)
	tt.Frame()
	if err := tt.Click("音质"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.mk.lens == a.mk.frame {
		t.Fatal("open, at rest, the highlight is a lens")
	}
	r, _ := tt.Find("极高")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	tt.Advance(60 * time.Millisecond)
	tt.Frame()
	if a.mk.lens != a.mk.frame {
		t.Error("on its way to the option under the pointer, the highlight is no lens")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.mk.lens == a.mk.frame {
		t.Error("landed, the highlight is still a lens")
	}
}

// At rest, the select's highlight shows on its option: drawn under the
// lens's way, its color is the hover's, not a fraction of it.
func TestSelectHighlightShowsAtRest(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	chosen := "标准"
	options := []string{"标准", "较高", "极高", "无损"}
	tt := ui.NewTester(func(c *ui.Context) {
		a.mk.frame++
		a.motion = a.motionLevel(c)
		ui.Column(c).Width(400).Height(400).Padding(10).Children(func() {
			a.paneSelect(c.Key("q"), "音质", &chosen, options)
		})
	}, 400, 400)
	tt.Frame()
	if err := tt.Click("音质"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	r, _ := tt.Find("较高")
	img := tt.Image()
	x, y := int(r.X+r.W-6), int(r.Y+r.H/2) // past its words, within its row
	lit := img.RGBAAt(x, y-35)             // 标准, chosen, under the highlight
	plain := img.RGBAAt(x, y+35)
	if int(plain.R)-int(lit.R) < 6 {
		t.Errorf("the chosen option %v is not lit against another %v", lit, plain)
	}
}
