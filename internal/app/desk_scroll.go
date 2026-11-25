package app

import (
	"math"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
)

// A line too long for the desktop lyrics' room, even smaller, slides along
// as it is sung: the word being sung kept a little left of the middle, the
// line's start at rest until the singing gets there and its end at rest
// once it has. Its characters fade out toward either edge of the room it
// slides past, where there is more of it.

const (
	deskScrollFade = 40   // DIPs over which characters fade at an edge
	deskScrollLead = 0.42 // where in the room the word sung stays
)

// deskScrollLine is the line m sliding past the room as it is sung at pos.
func (a *app) deskScrollLine(c *ui.Context, m deskMain, pos time.Duration, look deskLook) ui.Element {
	ink := a.deskShape(m, look, 0, false)
	w, h := ink.Size()
	run := max(0, w-deskRoom)
	chars, colors := a.deskChars(m, look, pos)

	// Where each character lies along the line, from its left.
	units := utf16.Encode(chars)
	type glyph struct {
		text     string
		x0, x1   float32
		from, to int // its runes
	}
	var glyphs []glyph
	r := 0
	for at := 0; at < len(units); {
		next := ink.NextBoundary(at)
		if next <= at {
			break
		}
		text := string(utf16.Decode(units[at:next]))
		x0, x1 := ink.Caret(at).X, ink.Caret(next).X
		if x1 < x0 {
			x1 = x0
		}
		n := len([]rune(text))
		glyphs = append(glyphs, glyph{text: text, x0: x0, x1: x1, from: r, to: r + n})
		r += n
		at = next
	}

	// How far along the line the singing is: through the word sung, as it
	// lights, or through the line's time with no words.
	sung := float32(0)
	if len(m.words) > 0 {
		rune0 := 0
		for _, wd := range m.words {
			n := len([]rune(wd.Text))
			x0, x1 := deskRunesX(glyphs, rune0, rune0+n, func(g glyph) (int, int, float32, float32) { return g.from, g.to, g.x0, g.x1 })
			switch {
			case pos >= wd.End:
				sung = x1
			case pos >= wd.Start:
				sung = x0 + (x1-x0)*wordLit(wd, pos)
			}
			rune0 += n
		}
	} else if m.end > m.at {
		sung = w * clamp01(float32(pos-m.at)/float32(m.end-m.at))
	}
	off, moving := a.deskSlide(c, m.text, sung-deskRoom*deskScrollLead, run, deskRoom)

	fade := func(x float32) float32 {
		v := x - off
		k := float32(1)
		if off > 0.5 {
			k = min(k, clamp01(v/deskScrollFade))
		}
		if off < run-0.5 {
			k = min(k, clamp01((deskRoom-v)/deskScrollFade))
		}
		return k
	}
	lay := func(key any, color func(g glyph) ui.Color) ui.Element {
		return ui.RichText(c.Key(key)).FontSize(m.size).FontWeight(800).SingleLine().Children(func() {
			for i, g := range glyphs {
				ui.Text(c.Key(i), g.text).TextColor(color(g))
			}
		})
	}
	box := ui.Box(c.Key("main")).Width(deskRoom).Height(h).Shrink(0).ClipX().PassThrough()
	box.Children(func() {
		ui.Box(c.Key("slide")).Absolute().Left(-off).Top(0).Width(w).Height(h).PassThrough().Children(func() {
			if look.edge.A > 0 {
				width := max(1.1, m.size*0.055)
				for i := range 8 {
					turn := float64(i) * math.Pi / 4
					lay(i, func(g glyph) ui.Color { return look.edge.Alpha(float32(look.edge.A) / 255 * fade((g.x0+g.x1)/2)) }).
						Absolute().Left(width * float32(math.Cos(turn))).Top(width * float32(math.Sin(turn)))
				}
			}
			lay("ink", func(g glyph) ui.Color {
				col := look.sung
				if g.from < len(colors) {
					col = colors[g.from]
				}
				return col.Alpha(float32(col.A) / 255 * fade((g.x0+g.x1)/2))
			}).Absolute().Left(0).Top(0)
		})
	})
	if moving {
		c.AnimationFrame()
	}
	a.deskSlider.room, a.deskSlider.run = box.LayoutBounds(), run
	return box
}

// deskRunesX returns where the runes from to to of glyphs lie, across.
func deskRunesX[G any](glyphs []G, from, to int, of func(G) (int, int, float32, float32)) (x0, x1 float32) {
	first := true
	for _, g := range glyphs {
		gf, gt, gx0, gx1 := of(g)
		if gt <= from || gf >= to {
			continue
		}
		if first {
			x0, first = gx0, false
		}
		x1 = gx1
	}
	return x0, x1
}

// deskStandRoom is how tall a column of standing lyrics may be.
func (a *app) deskStandRoom() float32 {
	room := float32(deskVH - deskToolsRoom - 8)
	if a.deskLookOf(a.settings.DeskStyle).pill.A > 0 {
		room -= 2 * 16
	}
	return room
}

// deskScrollColumn is the line m standing, too tall for its column, sliding
// up it as it is sung at pos, at its own size: as the line lying slides
// across (deskScrollLine), the word sung kept a little above the middle,
// the characters fading toward the column's ends. Standing, a long line
// was made smaller and set in more columns (1.3.10 test.2), where the
// listener wanted it to scroll.
func (a *app) deskScrollColumn(c *ui.Context, m deskMain, chars []rune, colors []ui.Color, pos time.Duration, room float32, look deskLook) ui.Element {
	pieces := deskPieces(chars, m.size, 800, a.deskFamily())
	// Where each piece stands, down the column.
	tops := make([]float32, len(pieces))
	var tall float32
	for i, p := range pieces {
		tops[i] = tall
		tall += p.h
	}
	run := max(0, tall-room)
	// y returns how far down the column rune r is, f of the way through it.
	y := func(r int, f float32) float32 {
		for i, p := range pieces {
			n := len([]rune(p.text))
			if p.text == "" {
				n = 1
			}
			if r < p.at+n {
				return tops[i] + p.h*clamp01((float32(r-p.at)+f)/float32(n))
			}
		}
		return tall
	}
	sung := float32(0)
	if len(m.words) > 0 {
		r := 0
		for _, wd := range m.words {
			n := len([]rune(wd.Text))
			switch {
			case pos >= wd.End:
				sung = y(r+n-1, 1)
			case pos >= wd.Start && n > 0:
				lit := wordLit(wd, pos) * float32(n)
				k := min(int(lit), n-1)
				sung = y(r+k, lit-float32(k))
			}
			r += n
		}
	} else if m.end > m.at {
		sung = tall * clamp01(float32(pos-m.at)/float32(m.end-m.at))
	}
	off, moving := a.deskSlide(c, m.text, sung-room*deskScrollLead, run, room)
	if moving {
		c.AnimationFrame()
	}
	fade := func(at int) float32 {
		for i, p := range pieces {
			if p.at != at {
				continue
			}
			v := tops[i] + p.h/2 - off
			k := float32(1)
			if off > 0.5 {
				k = min(k, clamp01(v/deskScrollFade))
			}
			if off < run-0.5 {
				k = min(k, clamp01((room-v)/deskScrollFade))
			}
			return k
		}
		return 1
	}
	color := func(i int) ui.Color {
		col := look.sung
		if i < len(colors) {
			col = colors[i]
		}
		return col.Alpha(float32(col.A) / 255 * fade(i))
	}
	wide := m.size * deskStep
	box := ui.Box(c.Key("main")).Width(wide).Height(room).Shrink(0).ClipY().PassThrough()
	box.Children(func() {
		ui.Box(c.Key("slide")).Absolute().Left(0).Top(-off).Width(wide).Height(tall).PassThrough().Children(func() {
			// Where each piece stands, slid, for those flying (deskStood).
			a.deskColumn(c, 0, pieces, color, m.size, 800, look, true, fade)
		})
	})
	a.deskSlider.room, a.deskSlider.run = box.LayoutBounds(), run
	return box
}

// deskSlideState is how far the line sliding has slid, as last drawn.
type deskSlideState struct {
	line string
	off  float32
	at   time.Time
	// room is what of the line shows, in the window, as laid out last,
	// and run how far it slides at most: for its characters flying, those
	// showing where they show (deskNowSprites).
	room ui.Rect
	run  float32
}

// shows returns how far a character v along the room, of length long,
// shows: as deskScrollLine and deskScrollColumn fade it toward the ends.
func (s *deskSlideState) shows(v, long float32) float32 {
	if v < 0 || v > long {
		return 0
	}
	k := float32(1)
	if s.off > 0.5 {
		k = min(k, clamp01(v/deskScrollFade))
	}
	if s.off < s.run-0.5 {
		k = min(k, clamp01((long-v)/deskScrollFade))
	}
	return k
}

// deskSlideEase is how long the line takes to catch up with the singing:
// it glides rather than jumping from word to word.
const deskSlideEase = 280 * time.Millisecond

// deskSlide returns how far a line of text should be slid, to want of run
// at most, room being what shows of it, and whether it is still moving.
// It starts and stops gently, at either end: slid straight to where the
// singing is, it set off at speed and hit its end at full speed, stopping
// dead (1.3.10 test.4). Near the ends the way eases in and out over a
// stretch of the room, and the slide follows it over deskSlideEase.
func (a *app) deskSlide(c *ui.Context, text string, want, run, room float32) (float32, bool) {
	target := softClamp(want, run, min(room*0.25, run/2))
	s := &a.deskSlider
	now := c.Now()
	if s.line != text || s.at.IsZero() || now.Sub(s.at) > time.Second {
		*s = deskSlideState{line: text, off: target, at: now}
		return target, false
	}
	dt := float32(now.Sub(s.at)) / float32(deskSlideEase)
	s.at = now
	s.off += (target - s.off) * (1 - float32(math.Exp(-float64(dt))))
	gap := target - s.off
	moving := gap > 0.3 || gap < -0.3 || a.pl.Playing() && target > 0 && target < run
	if !moving {
		s.off = target
	}
	return s.off, moving
}

// softClamp keeps x within 0 and hi, easing into either end over d: it
// follows x in between, and its rate falls to nothing at the ends, where
// min and max stopped it dead.
func softClamp(x, hi, d float32) float32 {
	if hi <= 0 {
		return 0
	}
	if d <= 0 {
		return min(max(x, 0), hi)
	}
	switch {
	case x <= -d:
		return 0
	case x < d:
		return (x + d) * (x + d) / (4 * d)
	case x <= hi-d:
		return x
	case x < hi+d:
		return hi - (hi+d-x)*(hi+d-x)/(4*d)
	}
	return hi
}
