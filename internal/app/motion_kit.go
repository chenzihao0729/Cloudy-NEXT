package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// How things move, the rest of the set begun at the top of ui_expand.go:
// what a hover turns, what a press does, what leaves, and what grows out of
// what opened it. A hover's colors and a press take these, never a number of
// their own (TestHoversTakeTheSet).
var (
	motionHover   = ui.Animation{Duration: 150 * time.Millisecond, Ease: motionEase} // a hover's colors
	motionPress   = ui.Snappy                                                        // a press, and its release
	motionLeave   = ui.Animation{Duration: 120 * time.Millisecond, Ease: ui.EaseIn}  // what goes, quickly, as what replaces it comes
	motionPush    = ui.Animation{Spring: ui.SpringOptions{Duration: 450 * time.Millisecond}}
	motionLand    = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}} // content arriving where its skeleton was
	motionFade    = ui.Animation{Spring: ui.SpringOptions{Duration: 240 * time.Millisecond}} // what only fades, as motion is lite
	motionSwap    = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond}} // what takes another's place, as a settings category
	settingsLeave = ui.Animation{Duration: 60 * time.Millisecond, Ease: ui.EaseIn}
	// A pane grows out of what opened it with a little give, as a sheet,
	// and goes back into it without.
	growOpen  = ui.Animation{Spring: ui.SpringOptions{Duration: 360 * time.Millisecond, Bounce: 0.12}}
	growQuick = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond, Bounce: 0.1}} // a pane a hover opens
	growClose = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond}}
)

const (
	growGone  = 0.002 // a pane grown less than this is gone
	growFresh = time.Second
)

// motionKit is what the motion of the app keeps from frame to frame.
type motionKit struct {
	skeleton route     // the page whose skeleton showed last
	gear     ui.Rect   // where the settings' gear is, for the motion scene
	cards    int       // the cards built so far in the frame, which come in one after another
	frame    uint64    // the views built so far, to tell a pane open again from one open all along
	comments time.Time // when the comments last showed a skeleton, which what comes fades in over
	fullAt   time.Time // when a segmented control last saw full motion (segmentedControl)
	lens     uint64    // the view that last built a lens (liquidLens)
	pressed  growKeep  // the control clicked last that may open a dialog: a menu's row, a chip, a settings button
	grows    map[string]*growKeep
}

// growKeep is what a pane growing out of what opened it keeps: what that
// was, as it opened it, and how tall the pane's content was.
type growKeep struct {
	from   ui.Rect
	radius float32
	at     time.Time // when from was noted
	use    bool      // whether the pane open, or going, flies from from
	was    bool      // whether the pane was open in the frame before
	h      float32
	shut   time.Time // when a press outside put it away
	laid   bool      // whether a dialog flying from from has been laid out, which it waits for
	shown  bool      // whether the pane was open in the frame before (growK)
	seen   uint64    // the view growSource last looked in, and growK
	kseen  uint64
	edge   bool    // whether the pane opens in this view, as growSource found
	opens  int     // how many times it opened, as growSource saw
	ready  bool    // whether the open pane's body has been laid out, which it waits for
	full   ui.Rect // where a panel the framework places was, open (growInside)
	ghost  uint64  // the view that last built it going (growGhost)
}

func (a *app) growKeep(key string) *growKeep {
	if a.mk.grows == nil {
		a.mk.grows = map[string]*growKeep{}
	}
	g := a.mk.grows[key]
	if g == nil {
		g = &growKeep{}
		a.mk.grows[key] = g
	}
	return g
}

// growFrom notes that el, rounded by radius, opens the pane key: the pane
// opening next grows out of it, and goes back into it.
func (a *app) growFrom(key string, el ui.Element, radius float32) {
	a.growFromRect(key, el.Bounds(), radius)
}

// pressFresh is how soon after a control is clicked a dialog opening takes
// it for what opened it (pressedFrom).
const pressFresh = 250 * time.Millisecond

// pressedFrom notes el, rounded by radius, as clicked, for a dialog it
// opens to fly out of it: the rows of menus, the chips of the lyrics, the
// buttons of the settings, which open dialogs through their actions.
func (a *app) pressedFrom(el ui.Element, radius float32) {
	a.mk.pressed = growKeep{from: el.Bounds(), radius: radius, at: time.Now()}
}

func (a *app) growFromRect(key string, r ui.Rect, radius float32) {
	g := a.growKeep(key)
	g.from, g.radius, g.at = r, radius, time.Now()
}

// growSource is what the pane key opening, or open, grows from: what opened
// it, if it was noted as it did; nothing, as for a shortcut, otherwise.
func (a *app) growSource(key string, open bool) (ui.Rect, float32, bool) {
	g := a.growKeep(key)
	// A dialog that builds nothing while closed, as some return before
	// they get here, opens again whether or not it saw itself close.
	g.edge = open && (!g.was || g.seen+1 < a.mk.frame)
	g.seen = a.mk.frame
	if g.edge {
		g.opens++
		g.use = g.from.W > 0 && g.from.H > 0 && time.Since(g.at) < growFresh
		if p := a.mk.pressed; !g.use && p.from.W > 0 && time.Since(p.at) < pressFresh {
			// Not noted as it opened: out of the control clicked just
			// before, which opened it, as a menu's row does.
			g.from, g.radius, g.at, g.use = p.from, p.radius, p.at, true
		}
		g.laid = false
	}
	g.was = open
	return g.from, g.radius, g.use
}

// growK is how far the pane key has grown out of what opened it: 0 put away,
// 1 open, a little over on the way. It is built whether the pane shows or
// not, for its first frame to start from where it was.
func (a *app) growK(c *ui.Context, key string, open bool, opening ui.Animation) float32 {
	g := a.growKeep(key)
	if open && (!g.shown || g.kseen+1 < a.mk.frame) {
		g.ready = false
	}
	g.shown, g.kseen = open, a.mk.frame
	if open && !g.ready && !g.laid && a.motion == motionFull {
		// Its first frame, which lays out what it holds, may be slow to
		// build: it grows from the next.
		return a.growHold(c, key)
	}
	stage := ui.Box(c.Key(key+"-grow")).Absolute().Size(0, 0).PassThrough()
	target := b2f(open)
	anim := growClose
	if open {
		anim = opening
	}
	if a.motion == motionLite {
		anim = motionFade
	}
	k := stage.AnimateTo("k", target, anim)
	if a.motion == motionOff {
		return target
	}
	return k
}

// growHold keeps the pane key where it starts, put away, and asks for the
// next frame: a dialog flies once its first frame, slow to build as the
// settings are, is on screen, rather than be half way out by then.
func (a *app) growHold(c *ui.Context, key string) float32 {
	stage := ui.Box(c.Key(key+"-grow")).Absolute().Size(0, 0).PassThrough()
	stage.Jump("k", 0)
	stage.Jump(enterKick{}, 0)
	stage.AnimateTo(enterKick{}, 1, ui.Animation{Duration: time.Millisecond})
	return 0
}

// growSpec is a pane grown out of what opened it.
type growSpec struct {
	key        string
	k          float32 // how far it has grown
	open       bool
	from       ui.Rect // what opened it, in window DIPs; empty, it grows from its own middle
	fromRadius float32
	full       ui.Rect // where it is, open; with no height, as tall as its content was
	guess      float32 // how tall it is before its content has been laid out
	radius     float32
	onDark     bool
}

// growRect is the pane of s, k of the way from what opened it, and its
// corner radius.
func growRect(s growSpec, full ui.Rect, lite bool) (ui.Rect, float32) {
	from := s.from
	if lite {
		return full, s.radius
	}
	if from.W <= 0 || from.H <= 0 {
		// From its own middle, a little smaller, as the panes did.
		from = ui.Rect{X: full.X + full.W*0.07, Y: full.Y + full.H*0.07, W: full.W * 0.86, H: full.H * 0.86}
	}
	return lerpRect(from, full, s.k), lerp(s.fromRadius, s.radius, clamp01(s.k))
}

// growPane builds the pane of s where it is, in an overlay: its glass going
// from what opened it to where it is open, k of the way, and holding its
// body, laid out where it is open, which fill fills; the glass uncovers it
// as it grows, and it clears from a blur. Put away, the body fades first and
// the glass goes back into what opened it, taking no presses on the way.
// It returns the glass, for presses outside it, and the body.
func (a *app) growPane(c *ui.Context, s growSpec, fill func(body ui.Element)) (shape, body ui.Element) {
	g := a.growKeep(s.key)
	full := s.full
	if full.H <= 0 {
		full.H = g.h
		if full.H <= 0 {
			full.H = max(s.guess, 40)
		}
	}
	rect, radius := growRect(s, full, a.motion == motionLite)
	shape = ui.Box(c.Key(s.key+"-glass")).Absolute().Left(rect.X).Top(rect.Y).Size(rect.W, rect.H).Clip()
	if s.k < 0.5 {
		// Still over what opened it, it leaves it the pointer: a click
		// on the button closes it, or mutes, rather than land on glass.
		shape.PassThrough()
	}
	a.paneGrowing(shape, radius, s.onDark, min(full.W, full.H))
	if a.motion == motionLite {
		shape.Opacity(clamp01(s.k))
	} else if f := smoothstep(0, 0.18, s.k); f < 1 {
		// Out of the hover of what opened it, rather than at once.
		shape.Opacity(f)
	}
	shape.Children(func() {
		shown := accountShown(s.k, s.open)
		body = ui.Column(c.Key(s.key + "-body")).Absolute().Left(full.X - rect.X).Top(full.Y - rect.Y).Width(full.W)
		if s.full.H > 0 {
			body.Height(full.H)
		}
		if shown < 1 {
			body.Opacity(shown)
		}
		if blur := (1 - shown) * 6; blur > 0.05 && a.motion == motionFull {
			body.Blur(blur)
		}
		fill(body)
		if b := body.LayoutBounds(); b.H > 0 {
			g.h, g.ready = b.H, true
		}
		if !s.open {
			// Going, it takes the presses that would reach its rows,
			// and only those: what opened it, under it, takes its own.
			ui.Box(c.Key(s.key+"-going")).Absolute().Left(full.X-rect.X).Top(full.Y-rect.Y).Size(full.W, full.H)
		}
	})
	return shape, body
}

// growShut notes that a press outside put the pane key away: the click
// that follows, on what opened it, must not open it again.
func (a *app) growShut(key string) { a.growKeep(key).shut = time.Now() }

// growJustShut reports whether a press outside just put the pane key away.
func (a *app) growJustShut(key string) bool {
	return time.Since(a.growKeep(key).shut) < 300*time.Millisecond
}

// enterAfterPaint brings el in from from: posed there in the first frame it
// is built, and to its place, as anim says, from the next, once that frame
// is on screen. A view slow to build, starting where it moves from in the
// frame that builds it, would show most of the motion gone by its first
// frame. It moves only by its pose, which paints without laying out.
func (a *app) enterAfterPaint(el ui.Element, from ui.Pose, anim ui.Animation) {
	b := el.Bounds()
	a.enterPose(el, from, anim, b.W == 0 && b.H == 0)
}

// enterPose poses el at from in its first frame, and at rest after.
func (a *app) enterPose(el ui.Element, from ui.Pose, anim ui.Animation, first bool) {
	switch a.motion {
	case motionOff:
		return
	case motionLite:
		from = ui.Pose{Fade: from.Fade}
		d := anim.Delay
		anim = motionFade
		anim.Delay = d
	}
	if first {
		el.AnimatePose(from, anim)
		// A frame to build next, which sends it on its way: a pose at
		// rest asks for none.
		el.Jump(enterKick{}, 0)
		el.AnimateTo(enterKick{}, 1, ui.Animation{Duration: time.Millisecond})
		return
	}
	el.AnimatePose(ui.Pose{}, anim)
}

// enterKick is the value enterPose moves for a frame to follow the first.
type enterKick struct{}

// sawSkeleton notes that the page showed its skeleton: its content fades
// in over where it was (loadedIn).
func (a *app) sawSkeleton() { a.mk.skeleton = a.route }

// loadedIn fades el, a part of a page's content, in over the skeleton it
// replaces, order of the parts after the first, a beat each: the title,
// then what is under it, then its buttons. A page whose content was there
// as it opened, as kept from before, shows it at once.
func (a *app) loadedIn(el ui.Element, order int) {
	if a.mk.skeleton != a.route {
		return
	}
	anim := motionLand
	anim.Delay = time.Duration(order) * 40 * time.Millisecond
	a.enterAfterPaint(el, ui.Pose{Fade: 1, Y: 4}, anim)
}

// pressPose sinks el, a row or a card, to scale as it is pressed, and lets
// it spring back as it is let go: what the pointer holds answers it.
func (a *app) pressPose(el ui.Element, scale float32) {
	if a.motion != motionFull {
		return
	}
	p := ui.Pose{}
	if el.Pressed() {
		p.Scale = scale
	}
	el.AnimatePose(p, motionPress)
}

// indexOrPlay is a song's number in its list, or, under the pointer, the
// play button in its place: the one shrinking and fading as the other
// grows in, as the play button turns to pause.
func (a *app) indexOrPlay(c *ui.Context, row ui.Element, n int, hovered bool, ink, faint ui.Color) {
	h := b2f(hovered)
	if a.motion == motionFull {
		h = clamp01(row.AnimateTo("lead", h, motionPress))
	} else if a.motion == motionLite {
		h = clamp01(row.AnimateTo("lead", h, motionHover))
	}
	ui.Box(c.Key("lead")).Size(26, 18).Children(func() {
		if h < 1 {
			s := 1 - 0.3*h
			ui.Row(c.Key("n")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Justify(ui.Center).AlignItems(ui.Center).
				Opacity(1-h).Scale(s, s).Children(func() {
				ui.Textf(c, "%d", n).FontSize(13).FontFeatures("tnum").TextColor(faint)
			})
		}
		if h > 0 {
			s := 0.6 + 0.4*h
			ui.Row(c.Key("play")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Justify(ui.Center).AlignItems(ui.Center).
				Opacity(h).Scale(s, s).Children(func() {
				ui.Icon(c, iconPlay).Size(13, 13).TextColor(ink)
			})
		}
	})
}

// pressedLift is where a card's cover stands, lifted as lift under the
// pointer: pressed, it sinks back under it, a little smaller, and springs up
// again as it is let go.
func (a *app) pressedLift(card ui.Element, lift ui.Pose) ui.Pose {
	if card.Pressed() && a.motion == motionFull {
		return ui.Pose{Y: -1, Scale: 0.97}
	}
	return lift
}

// growInside grows a panel the framework places, a popover's or a
// combobox's, out of from, k of the way: the panel shows nothing of its
// own; a pane of glass inside it goes from from to the panel's box, holding
// what fill builds where it is open, width wide and height tall (as tall
// as it is, with none), which it uncovers as it grows. Grown, the glass is
// the panel's pane. Before the panel has a box, the frame shows nothing of
// it. The panel's padding and gaps go to the body fill fills.
func (a *app) growInside(c *ui.Context, panel ui.Element, key string, k float32, open bool, from ui.Rect, fromR, radius, width, height float32, fill func(body ui.Element)) {
	g := a.growKeep(key)
	full := panel.LayoutBounds()
	switch {
	case full.W > 0 && open:
		g.full = full
	case full.W <= 0 && !open:
		// Going, built anew where it was (growGhost).
		full = g.full
	}
	growing := a.motion == motionFull && (k < 0.999 || k > 1.001) && from.W > 0 && from.H > 0
	// Going, its elements are others than the panel's, which the
	// framework would take for these moved.
	part := key
	if !open {
		part += "-going"
	}
	shape := ui.Column(c.Key(part + "-glass")).AlignItems(ui.Stretch)
	var dx, dy float32
	if growing && full.W > 0 {
		fullH := height
		if fullH <= 0 {
			fullH = max(g.h, from.H)
		}
		// The panel keeps its height as the glass grows in it: smaller,
		// it would fit under what opened it and show there, then go over
		// it as it grew too tall to.
		panel.Height(fullH)
		t := 1 - k
		dx, dy = (from.X-full.X)*t, (from.Y-full.Y)*t
		// Placed, not moved by a transform: a clip under a transform is
		// drawn on the CPU, which draws no glass, the pane going white.
		shape.Absolute().Left(dx).Top(dy).Width(lerp(from.W, width, k)).Height(lerp(from.H, fullH, k)).Clip()
		a.paneGrowing(shape, lerp(fromR, radius, clamp01(k)), false, min(width, fullH))
		if f := smoothstep(0, 0.18, k); f < 1 {
			shape.Opacity(f)
		}
	} else {
		shape.Width(width)
		if height > 0 {
			shape.Height(height)
		}
		a.pane(shape, radius, false)
		if growing || a.motion == motionLite && k < 1 {
			// No box yet to grow into, or lite: it only fades.
			shape.Opacity(clamp01(k) * b2f(full.W > 0 || !growing))
		}
	}
	shape.Children(func() {
		body := ui.Column(c.Key(part + "-body")).Width(width)
		if height > 0 {
			body.Height(height)
		}
		if growing {
			if full.W > 0 {
				body.Absolute().Left(-dx).Top(-dy)
			}
			shown := accountShown(k, open)
			if shown < 1 {
				body.Opacity(shown)
			}
			if blur := (1 - shown) * 6; blur > 0.05 {
				body.Blur(blur)
			}
		}
		fill(body)
		if b := body.LayoutBounds(); b.H > 0 {
			g.h, g.ready = b.H, true
		}
	})
}

// cardIn fades el, a card of a page's grid, in over the skeleton it
// replaces, a little after the card before it, as loadedIn does the parts
// of a page's head: the first dozen one after another, the rest with the
// twelfth. With press, the card sinks to it as it is pressed (pressPose):
// one pose for both.
func (a *app) cardIn(el ui.Element, press float32) {
	n := a.mk.cards
	a.mk.cards++
	if press > 0 && a.motion == motionFull && el.Pressed() {
		el.AnimatePose(ui.Pose{Scale: press}, motionPress)
		return
	}
	if a.mk.skeleton == a.route {
		anim := motionLand
		anim.Delay = time.Duration(min(n, 12)) * 18 * time.Millisecond
		a.enterAfterPaint(el, ui.Pose{Fade: 1, Y: 6}, anim)
		return
	}
	if press > 0 && a.motion == motionFull {
		el.AnimatePose(ui.Pose{}, motionPress)
	}
}

// commentIn fades a comment in, a little after the one above it, as it
// comes in place of the comments' skeleton, or under the skeleton of more:
// only then, not as the comments show again with nothing loading.
func (a *app) commentIn(el ui.Element) {
	n := a.mk.cards
	a.mk.cards++
	if time.Since(a.mk.comments) > 2*time.Second {
		return
	}
	anim := motionLand
	anim.Delay = time.Duration(min(n, 8)) * 30 * time.Millisecond
	a.enterAfterPaint(el, ui.Pose{Fade: 1, Y: 6}, anim)
}

// growGhost builds, as the pane key goes back into from, k of the way, the
// panel the framework stopped building as it closed, which growInside grew:
// where it was, as it was, and taking neither the pointer nor the focus.
// fill builds what it held, without what it did: rows, not items.
func (a *app) growGhost(c *ui.Context, key string, k float32, from ui.Rect, fromR, radius, width, height float32, fill func(body ui.Element)) {
	g := a.growKeep(key)
	if k <= growGone || g.full.W <= 0 || a.motion == motionOff {
		return
	}
	g.ghost = a.mk.frame
	ui.Overlay(c, func() {
		panel := ui.Box(c.Key(key + "-ghost")).Absolute().Left(g.full.X).Top(g.full.Y).Width(g.full.W).Inert()
		panel.Children(func() {
			a.growInside(c, panel, key, k, false, from, fromR, radius, width, height, fill)
		})
	})
}

// How a highlight that slides from one choice to the next, a segmented
// control's capsule or a select's row, rises into a lens of liquid glass on
// its way and lands as one with where it goes: Apple's lens, floating over
// what it crosses.
var (
	lensLiftUp   = ui.Animation{Spring: ui.SpringOptions{Duration: 160 * time.Millisecond}}
	lensLiftDown = ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond}}
	lensSlide    = ui.Animation{Spring: ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.06}}
)

// lensLift is how far the highlight el slides, now at pos on its way to
// to, has risen into a lens: up as it is held or sets off, and down as it
// nears to, by how near it is, so that it is down as it gets there rather
// than float a moment after.
func lensLift(el ui.Element, pos, to float32, held bool) float32 {
	dist := pos - to
	if dist < 0 {
		dist = -dist
	}
	up := held || dist > 0.02
	anim := lensLiftDown
	if up {
		anim = lensLiftUp
	}
	lift := clamp01(el.AnimateTo("lift", b2f(up), anim))
	if !held {
		// Down before the spring's long last stretch, which would leave
		// it hanging there, half a lens, as it crept the last of the way.
		lift = min(lift, smoothstep(0.06, 0.5, dist))
	}
	return lift
}

// liquidLens builds the lens over r, in its parent, lift of the way risen:
// grown by grow of its width and height, centered on r, the app's liquid
// glass, the player bar's, clear in its middle, what is under it read
// through it, and bending it only at its rim, its shadow under it. It takes
// no presses.
func (a *app) liquidLens(c *ui.Context, key string, r ui.Rect, lift, growW, growH float32) {
	a.mk.lens = a.mk.frame
	w, h := r.W*(1+growW*lift), r.H*(1+growH*lift)
	ui.Box(c.Key(key)).Absolute().Left(r.X-(w-r.W)/2).Top(r.Y-(h-r.H)/2).Size(w, h).Radius(h/2).PassThrough().
		Shadow(0, 5*lift, 16*lift, -2, ui.RGBA(0, 0, 0, 0.16*lift)).
		Material(barGlass{dark: a.pal.dark, clear: 1, bend: 0.18, surface: true, basis: r.H}).Opacity(smoothstep(0.05, 0.45, lift))
}

// liquidOK reports whether highlights may rise into lenses: with full
// motion, and glass, as not saving power.
func (a *app) liquidOK() bool { return a.motion == motionFull && !a.settings.PowerSaver }
