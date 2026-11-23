package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The account menu grows out of the account row: its pane starts as the
// row, keeps its foot below the row's and ends a menu over it, wider, the
// row its last row, the corners going from the row's to a menu's.
func TestAccountShapeGrowsFromRow(t *testing.T) {
	row := ui.Rect{X: 14, Y: 640, W: 200, H: 40}
	full := accountFull(row, 170)
	if foot := full.Y + full.H; foot != row.Y+row.H+accountPad {
		t.Fatalf("foot = %v, want %v under the row", foot, row.Y+row.H+accountPad)
	}
	if full.X != row.X-accountPad || full.W != row.W+2*accountPad+accountWider {
		t.Fatalf("full = %+v: not round the row, wider to the right", full)
	}
	if s, r := accountShape(row, full, 0); s != row || r != 8 {
		t.Fatalf("at 0 the pane is %+v (radius %v), want the row", s, r)
	}
	if s, r := accountShape(row, full, 1); s != full || r != 20 {
		t.Fatalf("at 1 the pane is %+v (radius %v), want the menu", s, r)
	}
	prev := row.Y + row.H
	for k := float32(0); k <= 1; k += 0.1 {
		s, _ := accountShape(row, full, k)
		if foot := s.Y + s.H; foot < prev-0.01 || foot > row.Y+row.H+accountPad+0.01 {
			t.Fatalf("at %v the foot is at %v: it should only sink to the menu's", k, foot)
		}
		prev = s.Y + s.H
	}
	// The rows show once the pane has grown some, and go first.
	if accountShown(0.2, true) != 0 || accountShown(1, true) != 1 {
		t.Fatal("the rows show too soon or never")
	}
	if accountShown(0.5, false) != 0 || accountShown(0.95, false) != 1 {
		t.Fatal("put away, the rows should go before the pane shrinks half way")
	}
}

// Open, the menu sits over the row with the row as its last row; put away,
// it shrinks back over some frames, taking no clicks, then is gone.
func TestAccountMenuMorphs(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "小明"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	row, ok := tt.Find("小明")
	if !ok {
		t.Fatal("no account row")
	}
	if err := tt.Click("小明"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	menu, ok := tt.Find("账户")
	if !ok || a.accountMenuH <= 0 {
		t.Fatal("the menu did not show")
	}
	if foot := menu.Y + menu.H; abs32(foot-(row.Y+row.H+accountPad)) > 0.5 || abs32(menu.X-(row.X-accountPad)) > 0.5 {
		t.Fatalf("menu %+v is not round the row %+v", menu, row)
	}
	if menu.H < 4*30+row.H {
		t.Fatalf("menu is %v tall: its rows and the account row do not fit", menu.H)
	}

	// Escape puts it away; while it goes, its rows still show but do
	// nothing.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.accountOpen {
		t.Fatal("Escape left it open")
	}
	tt.Advance(40 * time.Millisecond)
	tt.Frame()
	if !tt.HasText("退出登录…") {
		t.Fatal("the menu went at once, without shrinking back")
	}
	if r, ok := tt.Find("我的主页"); ok {
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		tt.Frame()
		if a.route.page == pageUser {
			t.Fatal("a row of the menu going away took a click")
		}
	}
	tt.Advance(time.Second)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if tt.HasText("退出登录…") {
		t.Fatal("the menu stayed after shrinking back")
	}
}

// Without motion the menu shows and goes at once.
func TestAccountMenuWithoutMotion(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Motion = "off"
	a.profile = &netease.Profile{UserID: 1, Nickname: "小明"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Click("小明")
	tt.Frame()
	tt.Frame()
	if !tt.HasText("退出登录…") {
		t.Fatal("not open")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if tt.HasText("退出登录…") {
		t.Fatal("without motion the menu should go at once")
	}
}
