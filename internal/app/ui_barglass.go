package app

import (
	"sync"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// barGlass is the glass of the player bar: clearer and bending what shows
// through more than the glass plugin's, whose refraction is fixed. It follows
// the plugin's optics (its lens, normal and tone, see glass.hlsl), with these
// to tell it from them:
//
//   - it hardly blurs, so that what is under the bar can be read through its
//     flat middle, its colors brought up rather than washed out, the dull
//     ones most, so that a vivid cover doesn't stain it;
//   - its curve reaches further in, on a rounder profile, and bends further,
//     so that the edges of the capsule visibly swim;
//   - red and blue bend a little apart, as through a thick lens;
//   - in place of a tint over everything, the lightness of what shows through
//     is squeezed: dark stays as it is, and only what is bright, which would
//     drown the bar's own words, comes down;
//   - it reads what is behind it, at 5 × 3 points, and is denser over what
//     is busy, colorful, or as light or dark as its words, clearer over what
//     is calm (scene, density);
//   - light comes from the top left: the rim is bright there and dimmer at
//     the bottom right, a hairline rings it inside, and a soft sheen spreads
//     from the top left corner, as on a slab of glass, a third as bright on
//     dark glass.
//
// clear and bend, from 0 to 1, are the listener's: clearer, it blurs less and
// lets more of what is bright through; bent more, the edge bends further, and
// its curve reaches further in. At 0.5 they are the glass as it was tuned.
type barGlass struct {
	dark        bool
	clear, bend float32
	surface     bool // use the same optics on a rounded panel
	translucent bool // let the native window material show through
	// basis, for a pane on its way to its size, as one growing out of a
	// button, is the smaller side it has there: its rim bends what shows
	// through as it will there, the same all the way, rather than as each
	// size on the way would have it, which made what shows through its
	// edges swim and shake as it grew.
	basis float32
	// veil, on a pane, is how much of its words' backing color lies over
	// all of it, over what is calm and over what is busy, and frost the
	// least it blurs, in DIPs: what its words sit on is read through it,
	// and they have to read over that.
	veil  [2]float32
	frost float32
}

func (a *app) interfaceGlass(dark bool) barGlass {
	return a.paneGlass(dark, 0, sceneMenu)
}

// paneGlass is the liquid glass of a pane of the scene, menus' or
// dialogs', whose shorter side is basis if not 0. A dialog's words are
// more and longer than a menu's: it is denser over what is busy, and blurs
// a little more.
func (a *app) paneGlass(dark bool, basis float32, scene glassScene) barGlass {
	g := barGlass{dark: dark, clear: a.settings.glassClear(a.pal.dark), bend: float32(a.settings.GlassBend),
		surface: true, translucent: true, basis: basis, veil: [2]float32{0.2, 0.5}, frost: 2}
	if scene == sceneDialog {
		g.veil, g.frost = [2]float32{0.24, 0.62}, 3
	}
	return g
}

// barMaterial is the material of the floating player bar and the batch
// bar over it: liquid glass, or frosted as the listener may choose.
func (a *app) barMaterial(dark bool) ui.Material {
	if !a.settings.frosted(sceneBar) {
		return a.floatGlass(dark)
	}
	if dark {
		return glass.Glass{Tint: ui.RGBA(14, 14, 18, 0.62)}
	}
	return glass.Glass{Tint: ui.RGBA(250, 250, 252, 0.58)}
}

// floatGlass is the glass of the bars floating over the page: the player bar
// and the batch bar. Under the cover background, the page is translucent
// over the window's material, so the glass has to take what it shows through
// as translucent too: read as opaque, the clear parts of the page are black,
// and in light mode the middle of the bar went dark.
func (a *app) floatGlass(dark bool) barGlass {
	return barGlass{dark: dark, clear: a.settings.glassClear(a.pal.dark), bend: float32(a.settings.GlassBend),
		translucent: a.settings.GlobalGlass || a.settings.mainGlass()}
}

// How the glass of the bar looks, whatever the listener chose.
const (
	barGlassSpread  = 0.05 // how far apart red and blue bend, of the bend
	barGlassRimWide = 1.1  // how wide the lit rim is, in DIPs
	// barGlassSheen is how bright the sheen from the top left corner is, on
	// a bar and on a pane: the bar's words sit at its ends, under it.
	barGlassSheen     = 0.16
	barGlassSheenPane = 0.24
	// barGlassDarkLight is how lit dark glass is, of light glass.
	barGlassDarkLight = 0.34
)

const barGlassSource = `package main

var Bend float
var Bezel float
var Spread float
var Light float
var Sheen float
var RimWide float
var ToneThin vec4
var ToneDense vec4
var Ink vec4
var InkLum float
var Veil vec2
var Corner float
var Surface float
var Film float

// edge returns the inward distance and outward normal of a rounded rectangle.
func edge(pos, half vec2) vec3 {
	q := pos - half
	r := min(Corner, min(half.x, half.y))
	a := abs(q) - (half - vec2(r))
	m := max(a, vec2(0))
	inside := r - length(m) - min(max(a.x, a.y), 0)
	n := vec2(0)
	if m.x > 0.0 || m.y > 0.0 {
		n = sign(q) * m / length(m)
	} else if a.x > a.y {
		n.x = sign(q.x)
	} else {
		n.y = sign(q.y)
	}
	return vec3(n, max(inside, 0))
}

// lens returns how far the surface bends what shows through at x, 0 at the
// edge of the curve to 1 where it starts, the glass plugin's Snell's law over
// a surface that is a circle's rather than Apple's squircle: rounder, it
// keeps bending further in.
func lens(x float) float {
	u := clamp(1.0-x, 0.0, 1.0)
	v := sqrt(max(1.0-u*u, 0.0001))
	n := sqrt(u*u + v*v)
	s := u / n
	c := v / n
	st := s / 1.5
	ct := sqrt(1.0 - st*st)
	return (s*ct - c*st) / (c*ct + s*st) / 1.118034
}

// tone maps what shows through by its lightness, along t: black to t.x,
// white to t.y, bent by t.z, its colors t.w as far from their lightness,
// less so the more colorful it is already.
func tone(c vec3, t vec4) vec3 {
	l := dot(c, vec3(0.2126, 0.7152, 0.0722))
	v := t.x + (t.y-t.x)*(1.0-pow(max(1.0-l, 0.0), t.z))
	chroma := max(c.r, max(c.g, c.b)) - min(c.r, min(c.g, c.b))
	k := (t.y - t.x) * t.w * max(1.0-0.75*chroma, 0.45)
	return clamp(vec3(v)+(c-vec3(l))*k, vec3(0), vec3(1))
}

// scene reads what is behind the glass at 5 × 3 points over it, each the
// mean of four around it so that a stroke of a word doesn't swing it, and
// drops the two darkest and the two brightest: it returns their lightness,
// how far the brightest and darkest left are apart, how colorful they are,
// and how much of it the app painted (the rest, under a translucent window,
// is the desktop, which can't be read).
func scene(size vec2) vec4 {
	sum := 0.0
	chroma := 0.0
	cover := 0.0
	lo0 := 2.0
	lo1 := 2.0
	lo2 := 2.0
	hi0 := -1.0
	hi1 := -1.0
	hi2 := -1.0
	for j := 0; j < 3; j++ {
		for i := 0; i < 5; i++ {
			at := (vec2(float(i), float(j)) + 0.5) / vec2(5.0, 3.0) * size
			s := backdropSample(at+vec2(-3.0, -2.0)) + backdropSample(at+vec2(3.0, -2.0)) +
				backdropSample(at+vec2(-3.0, 2.0)) + backdropSample(at+vec2(3.0, 2.0))
			s /= 4.0
			rgb := s.rgb / max(s.a, 0.01)
			l := dot(rgb, vec3(0.299, 0.587, 0.114))
			sum += l
			chroma += max(rgb.r, max(rgb.g, rgb.b)) - min(rgb.r, min(rgb.g, rgb.b))
			cover += s.a
			if l < lo0 {
				lo2 = lo1
				lo1 = lo0
				lo0 = l
			} else if l < lo1 {
				lo2 = lo1
				lo1 = l
			} else if l < lo2 {
				lo2 = l
			}
			if l > hi0 {
				hi2 = hi1
				hi1 = hi0
				hi0 = l
			} else if l > hi1 {
				hi2 = hi1
				hi1 = l
			} else if l > hi2 {
				hi2 = l
			}
		}
	}
	mean := (sum - lo0 - lo1 - hi0 - hi1) / 11.0
	return vec4(clamp(mean, 0.0, 1.0), clamp(hi2-lo2, 0.0, 1.0), clamp(chroma/15.0, 0.0, 1.0), clamp(cover/15.0, 0.0, 1.0))
}

func Fragment(dst vec4, pos vec2) vec4 {
	size := rectSize()
	half := size / 2.0
	e := edge(pos, half)
	bezel := min(half.x, half.y) * Bezel
	if Surface > 0.0 {
		bezel = min(bezel, 36.0)
	}
	n := e.xy
	t := e.z
	bend := vec2(0)
	if t < bezel {
		bend = n * Bend * lens(t/bezel)
	}
	// How dense the glass is, from 0 to 1: the more what is behind it is as
	// light as the words on it, busy, or colorful, the denser. What can't be
	// read is dense.
	sc := scene(size)
	density := clamp(0.55*(1.0-abs(sc.x-InkLum))+0.7*sc.y+0.3*sc.z, 0.0, 1.0)
	density = mix(1.0, density, sc.w)
	tn := mix(ToneThin, ToneDense, density)
	r := backdropSample(pos - bend*(1.0+Spread))
	g := backdropSample(pos - bend)
	b := backdropSample(pos - bend*(1.0-Spread))
	c := tone(vec3(r.r, g.g, b.b), tn)
	alpha := Film
	if Film < 1.0 {
		// Keep a clear pane translucent where the compositor will supply
		// the desktop. Where the app fills the backdrop, show the refracted
		// image at full strength, as the original capsule does.
		rgb := vec3(r.r/max(r.a, 0.01), g.g/max(g.a, 0.01), b.b/max(b.a, 0.01))
		c = mix(Ink.rgb, tone(rgb, tn), g.a)
		alpha = mix(Film, 1.0, g.a)
	}
	// Under what plays and the buttons at the end, what shows through keeps
	// to itself: words over words can't be read, however clear the glass.
	width := size.x
	side := max(width*0.5-220.0, 130.0)
	under := max(1.0-smoothstep(side*0.72, side*1.02, pos.x), 0.7*smoothstep(width-side*0.6, width-side*0.35, pos.x))
	// A pane's words are all over it: the veil lies over all of it, thin
	// over what is calm and thick over what is busy.
	veil := Ink.a * under * mix(0.6, 1.1, density)
	if Surface > 0.0 {
		veil = mix(Veil.x, Veil.y, density)
	}
	c = mix(c, Ink.rgb, min(veil, 0.95))
	// The light, from the top left: the rim lit there and dimmer at the
	// bottom right, a hairline inside it all round, a soft glow just inside
	// the rim, brighter at the top and left and a little shade at the
	// bottom, and a sheen spreading from the top left corner.
	tl := clamp(max(-n.x, 0.0)+max(-n.y, 0.0), 0.0, 1.0)
	br := clamp(max(n.x, 0.0)+max(n.y, 0.0), 0.0, 1.0)
	x := t / RimWide
	band := exp(-x * x)
	ring := exp(-4.0 * t * t)
	glow := exp(-t / min(7.0, min(half.x, half.y)*0.3))
	sheen := 1.0 - smoothstep(0.0, 0.55, length(pos/size)*0.70710678)
	c = mix(c, vec3(1), clamp(Light*(0.95*tl+0.55*br)*band, 0.0, 1.0))
	c = mix(c, vec3(1), Light*0.3*ring)
	c = mix(c, vec3(1), clamp(Light*(0.14*tl*glow+Sheen*sheen), 0.0, 1.0))
	c = mix(c, vec3(0), Light*0.07*max(n.y, 0.0)*glow)
	return vec4(c * alpha, alpha)
}
`

var barGlassShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(barGlassSource) })

// tones returns the tone of the glass over what is calm (thin) and
// over what is busy (dense): the lightness black and white show at through
// it, how it bends between them, and how much of its colors it keeps.
//
// Light glass lifts black less and less as it gets clearer, and over what is
// calm only half as much, so that what shows through stays vivid rather than
// milky; it keeps nearly twice the colors, the dull ones most (tone). Dark
// glass dims white more over what is bright or busy.
func (g barGlass) tones() (thin, dense [4]float32) {
	if g.dark {
		high := 0.32 + 0.48*g.clear
		return [4]float32{0.03, min(high*1.1, 1), 1.5, 2.4}, [4]float32{0.03, high * 0.8, 1.5, 2.4}
	}
	low := 0.62 - 0.4*g.clear
	return [4]float32{low * 0.5, 1, 1.2, 1.9}, [4]float32{min(low*1.15, 0.8), 1, 1.2, 1.9}
}

// PaintMaterial paints the glass over box, a capsule by radii. On the CPU the
// shader, which runs interpreted, would take a good part of a second a frame:
// there it is the plugin's glass, which is drawn natively.
func (g barGlass) PaintMaterial(p *ui.Painter, box ui.Rect, radii [4]float32) {
	if !p.Accelerated() {
		alpha := float32(0.24)
		if g.dark {
			alpha = 0.32
		}
		if g.surface {
			alpha = 0.78 // Keep words behind a software-rendered dialog from competing with its text.
		}
		tint := white.Alpha(alpha)
		if g.dark {
			tint = ui.RGBA(0, 0, 0, alpha)
		}
		if g.translucent && !g.surface {
			// The native backdrop can still show through on software rendering.
			p.Fill(box, tint, radii[0])
			p.Stroke(box, tint.Mix(white.Alpha(0.24), 0.65), radii[0], 1)
		} else {
			glass.Paint(p, box, radii[0], glass.Glass{Tint: tint})
		}
		return
	}
	thin, dense := g.tones()
	blur := max(2.8*(1-g.clear)*(1-g.clear), g.frost)
	veil := g.veil
	if veil == [2]float32{} {
		veil = [2]float32{0.15, 0.15}
	}
	ink, inkLum, light := [4]float32{0.97, 0.97, 0.98, 0.85}, float32(0.12), float32(1)
	if g.dark {
		ink, inkLum, light = [4]float32{0.05, 0.05, 0.06, 0.85}, 0.95, barGlassDarkLight
	}
	film, surface, bend, sheen := float32(1), float32(0), 144*g.bend, float32(barGlassSheen)
	if g.translucent {
		film = lerp(0.8, 0.36, clamp01(g.clear))
	}
	side := min(box.W, box.H)
	if g.basis > 0 {
		side = g.basis
	}
	if g.surface {
		surface, sheen = 1, barGlassSheenPane
		bend = min(bend, 1.2*side*g.bend, 76*g.bend)
	}
	// The shader's bezel is a share of the smaller half of the pane: for a
	// basis, the share that makes it as wide as there, within the pane.
	bezel := float32(0.33 + 0.66*g.bend)
	if now := min(box.W, box.H); g.basis > 0 && now > 0 {
		bezel = min(bezel*g.basis/now, 1)
	}
	options := shader.Options{Radius: radii[0], BackdropBlur: blur, Uniforms: map[string]any{
		"Bend": bend, "Bezel": bezel, "Spread": barGlassSpread,
		"Light": light, "Sheen": sheen, "RimWide": barGlassRimWide,
		"ToneThin": thin[:], "ToneDense": dense[:], "Ink": ink[:], "InkLum": inkLum, "Veil": veil[:],
		"Corner": radii[0], "Surface": surface, "Film": film,
	}}

	barGlassShader().Draw(p, box, options)
}
