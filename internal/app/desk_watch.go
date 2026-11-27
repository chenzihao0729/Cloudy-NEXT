package app

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// deskState is what the desktop lyrics follow outside their window: the
// pointer over them while they are locked, which lets nothing else through
// to them, and whether a game or a video is full screen in front.
type deskState struct {
	stop      chan struct{} // stops watchDesk
	placed    mygo.Point    // where placeDesk last put the window
	placedSet bool          // and the window hasn't been moved since
	near      bool          // the pointer is over the locked lyrics
	// unlockWin is the button that unlocks locked lyrics, in a window of
	// its own over theirs while the pointer is over them, and unlockSince
	// when it last showed, for it to come in.
	unlockWin   *mygo.Window
	unlockSince time.Time
	covered     bool      // something full screen is in front
	checked     time.Time // when that was last asked
	idle        bool      // nothing plays, and they hide then
	idleSince   time.Time // when nothing has played since, or zero
	vertical    bool      // the window is made standing up
	// moreAt is the tools' more button, in the window; moreWin the panel
	// it opens, and moreShut when that last closed.
	moreAt ui.Rect
	bodyAt ui.Rect // the lyrics, lying or standing, in the window
	// toolsAt is the tools' capsule over unlocked lyrics, in the window,
	// over whether the pointer is over the lyrics, which shows it; through
	// that the window lets the pointer through, being away from both.
	toolsAt  ui.Rect
	moves    int // the listener's moves of the window, for deskSettleSoon
	over     bool
	through  bool
	moreWin  *mygo.Window
	moreShut time.Time
	more     deskMore
}

// watchDesk follows, while the desktop lyrics are open, the pointer over
// them and what is full screen, from a clock off the main thread: the
// window, locked, hears nothing of the pointer, and drawing it on a clock
// would take frames from a game.
func (a *app) watchDesk() {
	a.stopDeskWatch()
	stop := make(chan struct{})
	a.desk.stop = stop
	go func() {
		t := time.NewTicker(120 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				mygo.RunOnMain(a.deskTick)
			}
		}
	}()
}

// stopDeskWatch stops watchDesk, as the lyrics close.
func (a *app) stopDeskWatch() {
	if a.desk.stop != nil {
		close(a.desk.stop)
		a.desk.stop = nil
	}
	a.desk.near, a.desk.covered, a.desk.idle = false, false, false
	a.desk.idleSince = time.Time{}
	a.dropFloat(&a.desk.unlockWin)
}

// deskTick is a tick of watchDesk, on the main thread.
func (a *app) deskTick() {
	w := a.deskWin
	if w == nil || a.desk.stop == nil {
		a.showDeskUnlock(false)
		return
	}
	s := &a.settings
	now := time.Now()
	// Nothing playing a moment, not between two songs, they go.
	switch {
	case !s.DeskHideStopped || a.pl.Playing():
		a.desk.idleSince = time.Time{}
	case a.desk.idleSince.IsZero():
		a.desk.idleSince = now
	}
	if idle := !a.desk.idleSince.IsZero() && now.Sub(a.desk.idleSince) >= 1500*time.Millisecond; idle != a.desk.idle {
		a.desk.idle = idle
		a.applyDeskLyrics()
	}
	if now.Sub(a.desk.checked) >= time.Second {
		a.desk.checked = now
		if covered := !s.DeskOverFullscreen && fullscreenInFront(w.NativeHandle()); covered != a.desk.covered {
			a.desk.covered = covered
			a.applyDeskLyrics()
		}
	}
	if !w.IsVisible() || !s.DeskLock || a.deskFly.win != nil {
		a.desk.near = false
		a.showDeskUnlock(false)
		if w.IsVisible() && !s.DeskLock {
			a.deskPassAround(w)
		}
		return
	}
	p := mygo.Screen.CursorScreenPoint()
	b := w.Bounds()
	in := func(r mygo.Rectangle) bool {
		return p.X >= r.X && p.Y >= r.Y && p.X < r.X+r.Width && p.Y < r.Y+r.Height
	}
	near := in(b)
	if u := a.desk.unlockWin; u != nil && u.IsVisible() && in(u.Bounds()) {
		near = true // over the button, which may reach past the window
	}
	a.desk.near = near
	a.showDeskUnlock(near)
}

// deskUnlockSize is how large the unlock button's window is: the button,
// and room for its shadow.
const deskUnlockSize = 52

// showDeskUnlock shows the button that unlocks locked lyrics, over them
// where their tools are, or hides it. The lyrics' window lets the pointer
// through all the while; the button has a window of its own, which takes
// it. The lyrics' window took the pointer itself over where the button
// showed in it, as a clock found it there: a click just as the pointer
// got there, or before the button had been laid out, went through to what
// was under it, and the button could not be pressed (1.3.9).
func (a *app) showDeskUnlock(on bool) {
	u := a.desk.unlockWin
	if !on {
		if u != nil && u.IsVisible() {
			u.Hide()
		}
		return
	}
	b := a.deskWin.Bounds()
	x := b.X + b.Width/2 - deskUnlockSize/2
	y := b.Y + deskToolsRoom - 20 - deskUnlockSize/2
	if u == nil {
		u = a.floatWindow(" 解锁桌面歌词", x, y, deskUnlockSize, deskUnlockSize, a.deskUnlockView, func() {
			a.showDeskUnlock(false)
		})
		showAtOnce(u.NativeHandle())
		a.desk.unlockWin = u
	}
	if u.IsVisible() {
		if ub := u.Bounds(); ub.X != x || ub.Y != y {
			u.SetPosition(x, y)
		}
		return
	}
	a.desk.unlockSince = time.Now()
	u.Show()
	// A flyout shows by its anchor: over the lyrics, and over them in
	// front, both always on top.
	u.SetPosition(x, y)
	u.SetAlwaysOnTop(true)
	u.Invalidate()
}

// deskHitPad is how far around the lyrics, and their tools, unlocked lyrics
// take the pointer.
const deskHitPad = 12

// deskPassAround lets the pointer through unlocked lyrics' window where
// nothing of them shows: the window is far larger than the line it shows,
// and clear all around it, which took the clicks meant for what is under
// it, the player bar's own buttons or the taskbar (1.3.9). Over the lyrics,
// or their tools while they show, the window takes the pointer, to be
// dragged, or its tools pressed. Off a clock, as the locked lyrics' unlock
// button is: the window hears nothing of the pointer while it lets it by.
func (a *app) deskPassAround(w *mygo.Window) {
	if a.deskFly.win != nil {
		return // the flight has the window let it through, or not
	}
	live := a.desk.bodyAt
	if live.W <= 0 {
		return // not laid out yet
	}
	if t := a.desk.toolsAt; a.desk.over && t.W > 0 {
		live = unionRect(live, t)
	}
	p := mygo.Screen.CursorScreenPoint()
	b := w.Bounds()
	x, y := float32(p.X-b.X), float32(p.Y-b.Y)
	in := x >= live.X-deskHitPad && y >= live.Y-deskHitPad && x < live.X+live.W+deskHitPad && y < live.Y+live.H+deskHitPad
	if through := !in; through != a.desk.through {
		a.desk.through = through
		w.SetIgnoreMouseEvents(through)
		w.Invalidate()
	}
}

// unionRect returns the least rectangle holding a and b.
func unionRect(a, b ui.Rect) ui.Rect {
	x0, y0 := min(a.X, b.X), min(a.Y, b.Y)
	x1, y1 := max(a.X+a.W, b.X+b.W), max(a.Y+a.H, b.Y+b.H)
	return ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}
