package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The account menu grows out of the account row at the foot of the sidebar,
// as a sheet of glass: the row's hover turns into the pane, which keeps its
// foot on the row and rises, a little wider, the rows of the menu showing
// as it uncovers them, clearing from a blur. The row stays where it was, the
// menu's last row. Put away, the rows fade first and the pane sinks back
// into the row.

const (
	accountPad   = 6     // the pane's padding round its rows, as a menu's
	accountWider = 16    // how far past the row the pane reaches, to the right
	accountGone  = 0.002 // a menu shown less than this is gone
	accountGuess = 166   // the menu's height before it has been laid out
)

var (
	// accountOpening rises with a little give at the top, as a sheet.
	accountOpening = growOpen
	accountClosing = growClose
	accountLite    = motionFade
)

// accountMenuK is how far the account menu has grown out of row: 0 put
// away, 1 open, a little over on the way.
func (a *app) accountMenuK(row ui.Element) float32 {
	target, anim := float32(0), accountClosing
	if a.accountOpen {
		target, anim = 1, accountOpening
	}
	if a.motion == motionLite {
		anim = accountLite
	}
	k := row.AnimateTo("account-menu", target, anim)
	if a.motion == motionOff {
		return target
	}
	return k
}

// accountFull is where the open account menu is, h tall, over row: its foot
// on the row's, the row inside it as its last row.
func accountFull(row ui.Rect, h float32) ui.Rect {
	return ui.Rect{X: row.X - accountPad, Y: row.Y + row.H + accountPad - h, W: row.W + 2*accountPad + accountWider, H: h}
}

// accountShape is the pane of the account menu k of the way from row to
// full, and its corner radius: the row's, then a menu's.
func accountShape(row, full ui.Rect, k float32) (ui.Rect, float32) {
	return lerpRect(row, full, k), lerp(8, 20, clamp01(k))
}

// accountShown is how much the menu's rows show at k: once the pane has
// grown some, opening; they go before it shrinks much, put away.
func accountShown(k float32, open bool) float32 {
	if open {
		return smoothstep(0.3, 0.85, k)
	}
	return smoothstep(0.5, 0.9, k)
}

// accountMenu builds the account menu over row while it shows, k of the
// way out of it.
func (a *app) accountMenu(c *ui.Context, row ui.Element, k float32) {
	if !a.accountOpen && k <= accountGone {
		return
	}
	r := row.Bounds()
	if r.W <= 0 || r.H <= 0 {
		return
	}
	p := a.pal
	prof := a.profile
	open := a.accountOpen
	h := a.accountMenuH
	if h <= 0 {
		h = accountGuess
	}
	full := accountFull(r, h)
	shape, radius := accountShape(r, full, k)
	ui.Overlay(c, func() {
		// The pane: the hover of the row, as small, which it fades in from
		// and back into, rather than show the lighter glass of a control.
		pane := ui.Box(c.Key("account-pane")).Absolute().Left(shape.X).Top(shape.Y).Size(shape.W, shape.H).Clip()
		a.pane(pane, radius, false)
		pane.Opacity(smoothstep(0, 0.18, k))
		pane.Children(func() {
			// The rows stay where they go, uncovered by the pane growing.
			shown := accountShown(k, open)
			menu := ui.Column(c.Key("account-menu")).Absolute().Left(full.X - shape.X).Top(full.Y - shape.Y).Width(full.W).
				Padding(accountPad).Gap(1).TextColor(p.text).Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label("账户").Opacity(shown)
			if blur := (1 - shown) * 6; blur > 0.05 && a.motion == motionFull {
				menu.Blur(blur)
			}
			menu.Children(func() {
				a.accountItems(c, open)
				// Room for the row, which shows over it.
				ui.Box(c.Key("account-room")).Height(r.H)
			})
			if b := menu.LayoutBounds(); b.H > 0 {
				a.accountMenuH = b.H
			}
		})
		// The row, over the pane, where it is in the sidebar: the menu's
		// last row. Its hover only shows as the pane goes.
		here := ui.ButtonBase(c.Key("account-here")).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).PaddingX(8).Gap(10).
			Radius(lerp(8, 14, clamp01(k))).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).TextColor(p.text).
			Label(prof.Nickname).Expanded(open)
		if here.Hovered() {
			if f := 1 - smoothstep(0, 0.35, k); f > 0 {
				here.Background(p.hover.Alpha(float32(p.hover.A) / 255 * f))
			}
		}
		here.Children(func() { a.accountRowContent(c, prof) })
		// A press outside the pane puts it away, the row's too: the click
		// that follows on the row must not open it again.
		if open && (pane.PressedOutside() || pane.OverlayShortcut(0, ui.KeyEscape)) {
			a.accountOpen, a.accountShut = false, c.Now()
		}
		if here.Clicked() {
			switch {
			case open:
				a.accountOpen = false
			case c.Now().Sub(a.accountShut) > 300*time.Millisecond:
				a.accountOpen = true // again, as it goes
			}
		}
	})
}
