package app

import (
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// The silk is a scene of the living backdrop: lengths of silk in the
// cover's own colors, lying in long folds across the picture and slowly
// slipping over one another, with the sheen silk has where its folds turn
// to the light. The bass makes the folds deeper and their colors fuller,
// the treble the sheen brighter; the pointer, moving over it, stirs it,
// as a hand drawn across silk would, and it settles again behind it.
//
// Its colors are the cover's, taken from it (coverPalette) rather than
// turned from its one hue as the other scenes', and made as deep as white
// needs to read over them: the ground dark, the folds darker than the
// lyrics, a fine strand here and there nearly white.

// silkSource folds the plane by noise warped by noise of its own: a
// field whose bands are the folds. Each of four colors lies along bands
// of its own width, the finest the brightest; the field's slope, as a
// surface's, gives where the folds turn to the light.
const silkSource = `package main

var Time float
var Bass float
var High float
var Deep vec4
var C1 vec4
var C2 vec4
var C3 vec4
var C4 vec4
// Pointer is where the pointer is, in heights of the picture from its
// middle, up positive, and how hard it stirs; Pull, how it goes.
var Pointer vec4
var Pull vec2

func hash(p vec2) float {
	return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453)
}

func slope(p vec2) vec2 {
	q := vec2(dot(p, vec2(127.1, 311.7)), dot(p, vec2(269.5, 183.3)))
	return -1.0 + 2.0*fract(sin(q)*43758.5453)
}

// wave is gradient noise, of -0.5 to 0.5 or so, smooth in its slope.
func wave(p vec2) float {
	i := floor(p)
	f := fract(p)
	u := f * f * f * (f*(f*6.0-15.0) + 10.0)
	a := dot(slope(i), f)
	b := dot(slope(i+vec2(1, 0)), f-vec2(1, 0))
	c := dot(slope(i+vec2(0, 1)), f-vec2(0, 1))
	d := dot(slope(i+vec2(1, 1)), f-vec2(1, 1))
	return mix(mix(a, b, u.x), mix(c, d, u.x), u.y)
}

// folds is wave of four octaves, each turned from the last.
func folds(p vec2) float {
	v := 0.0
	amp := 0.5
	for i := 0; i < 4; i++ {
		v += amp * wave(p)
		p = vec2(0.8*p.x+0.6*p.y, -0.6*p.x+0.8*p.y)*2.03 + 11.7
		amp *= 0.5
	}
	return v
}

// strands takes p, a place in heights of the picture from its middle, to
// where it lies along the silk: stirred by the pointer, which turns what
// is near it about itself and draws it along after it, and then into long
// strands, slanting up to the right.
func strands(p vec2) vec2 {
	d := p - Pointer.xy
	g := exp(-dot(d, d)*7.0) * Pointer.z
	p += vec2(-d.y, d.x)*g*0.9 + Pull*g*0.35
	return vec2(0.878*p.x+0.479*p.y, -0.479*p.x+0.878*p.y) * vec2(0.26, 0.9)
}

// band is a fold of a color: light along the crests of a sine, the
// sharper the narrower.
func band(x float, sharp float) float {
	return pow(0.5+0.5*sin(x), sharp)
}

func Fragment(dst vec4, pos vec2) vec4 {
	size := rectSize()
	px := 1.0 / size.y
	p := vec2(pos.x-0.5*size.x, 0.5*size.y-pos.y) * px
	t := Time
	s := strands(p)
	q := vec2(folds(s*1.1+vec2(0, t*0.11)), folds(s*1.1+vec2(4.7, 1.9-t*0.09)))
	sr := s*1.2 + q*1.5
	r := vec2(folds(sr+vec2(1.7, 9.2)+t*0.06), folds(sr+vec2(8.3, 2.8)-t*0.05))
	f := folds(s + r*1.7 + vec2(t*0.03, 0))
	ph := f*1.5 + r.x*0.8 + q.y*0.4
	// Where each color lies, and its folds across it. The music makes
	// them fuller, not other: the folds keep their shape.
	m1 := smoothstep(-0.22, 0.14, q.x)
	m2 := smoothstep(-0.16, 0.2, q.y)
	m3 := smoothstep(-0.1, 0.22, r.y)
	m4 := smoothstep(0.0, 0.26, r.x-q.x)
	swell := 0.92 + 0.14*Bass
	c := Deep.rgb
	c = mix(c, C1.rgb, band(ph*6.0, 2.6)*m1*0.92*swell)
	c = mix(c, C2.rgb, band(ph*5.0+2.1, 3.4)*m2*0.85*swell)
	c = mix(c, C3.rgb, band(ph*9.0+4.0, 8.0)*m3*0.8*swell)
	c = mix(c, C4.rgb, band(ph*15.0+1.0, 18.0)*m4*0.9)
	// The sheen: the field's slope as a surface's, lit from above. The
	// finest folds' slope is taken a pixel across and a pixel up, the GPU's
	// own (dfdx) being one for each square of four pixels, which shows as
	// steps along a highlight; the broad warps' is smooth enough for it.
	rx := r + vec2(dfdx(r.x), dfdx(r.y))
	ry := r - vec2(dfdy(r.x), dfdy(r.y))
	fx := folds(strands(p+vec2(px, 0)) + rx*1.7 + vec2(t*0.03, 0))
	fy := folds(strands(p+vec2(0, px)) + ry*1.7 + vec2(t*0.03, 0))
	broad := r.x*0.8 + q.y*0.4
	gr := vec2((fx-f)*1.5+dfdx(broad), (fy-f)*1.5-dfdy(broad)) * size.y * 0.045
	n := normalize(vec3(-gr, 1.0))
	lit := dot(n, normalize(vec3(0.35, 0.55, 0.76)))
	c *= 0.7 + 0.42*lit
	spec := pow(max(dot(n, normalize(vec3(0.2, 0.4, 0.9))), 0.0), 40.0)
	c += spec * 0.1 * (1.0 + 0.8*High) * (0.5 + C4.rgb)
	// Darker toward the corners.
	vc := pos/size - 0.5
	c *= 1.0 - 0.3*dot(vc, vc)
	c += (hash(pos*1.31+vec2(0.7, 3.3)) - 0.5) * 0.008
	return vec4(clamp(c, 0.0, 1.0), 1)
}
`

var silkShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(silkSource) })

// silkTones are how light the silk's colors are, as luminance: its
// ground, its three broad colors, and its fine strand.
var silkTones = [5]float64{0.12, 0.26, 0.35, 0.45, 0.74}

// silkColors returns the silk's ground and its four colors for a cover
// of tint whose colors are pal (coverPalette's, or none): each the hue of
// one of the cover's, as colorful, made as light as its place in the
// silk asks. A cover of fewer colors has the others turned from its own,
// one of none from tint.
func silkColors(tint ui.Color, pal []ui.Color) (deep ui.Color, cs [4]ui.Color) {
	ground := tint
	var more []ui.Color
	if len(pal) > 0 {
		ground, more = pal[0], pal[1:]
	}
	// The ground: the cover's, if it has a color, else its most vivid.
	gh, gs, _ := hsv(float64(ground.R)/255, float64(ground.G)/255, float64(ground.B)/255)
	if gs < 0.15 {
		gh, gs, _ = hsv(float64(tint.R)/255, float64(tint.G)/255, float64(tint.B)/255)
	}
	if len(more) == 0 {
		more = []ui.Color{tint}
	}
	// Too few: the next turned from the ones there are, by turns warmer
	// and cooler.
	turns := [...]float64{28, -32, 16}
	base := len(more)
	for i := 0; len(more) < 4; i++ {
		src := more[i%base]
		h, s, v := hsv(float64(src.R)/255, float64(src.G)/255, float64(src.B)/255)
		more = append(more, hsvColor(turnHue(h, turns[i%len(turns)]), s, v))
	}
	deep = silkTone(hsvColor(gh, min(gs, 0.75), 1), silkTones[0])
	// The finest strand is the lightest of them, whitened.
	lightest := 0
	for i, c := range more[:4] {
		if luminance(c) > luminance(more[lightest]) {
			lightest = i
		}
	}
	for i := range 3 {
		cs[i] = silkTone(more[i], silkTones[i+1])
	}
	cs[3] = silkTone(more[lightest].Mix(ui.RGB(255, 255, 255), 0.35), silkTones[4])
	return deep, cs
}

// silkTone returns c made as light as lum, keeping its hue; its color,
// if it has one, a little fuller, as silk's is.
func silkTone(c ui.Color, lum float64) ui.Color {
	h, s, _ := hsv(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	if s > 0.12 {
		s = min(max(s*1.15, 0.3), 0.8)
	} else {
		s = 0
	}
	col := hsvColor(h, s, 1)
	r, g, b := float64(col.R)/255, float64(col.G)/255, float64(col.B)/255
	k := lum / max(0.2126*r+0.7152*g+0.0722*b, 1e-3)
	ch := func(v float64) uint8 { return uint8(min(1, v*k)*255 + 0.5) }
	return ui.RGB(ch(r), ch(g), ch(b))
}

func luminance(c ui.Color) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// silkPointer is the pointer as the silk feels it: following it a little
// behind, and stirring it as hard as it goes, settling when it stops.
type silkPointer struct {
	x, y   float32 // where it is, in heights from the middle, up positive
	vx, vy float32 // how it goes
	force  float32
	tx, ty float32 // where the pointer is
	in     bool    // over the picture
	seen   bool    // tx, ty have been where it is
	last   time.Time
}

// aim says where the pointer is now, or that it is not over the picture.
func (sp *silkPointer) aim(x, y float32, in bool) {
	sp.in = in
	if !in {
		return
	}
	sp.tx, sp.ty = x, y
	if !sp.seen {
		// It came in: from where it is, not from the middle.
		sp.x, sp.y, sp.seen = x, y, true
	}
}

// step follows the pointer dt seconds on.
func (sp *silkPointer) step(dt float32) {
	k := 1 - float32(math.Exp(float64(-dt*5)))
	dx, dy := sp.tx-sp.x, sp.ty-sp.y
	sp.x += dx * k
	sp.y += dy * k
	sp.vx += (dx - sp.vx) * k
	sp.vy += (dy - sp.vy) * k
	target := float32(0)
	if sp.in {
		target = min(1, float32(math.Hypot(float64(dx), float64(dy)))*6)
	}
	// Caught up with a pointer standing still: it stirs no more, though
	// following it only ever comes nearer.
	if target < 0.004 {
		target = 0
	}
	// Stirred at once, settling slowly.
	rate := float32(1.2)
	if target > sp.force {
		rate = 8
	}
	sp.force += (target - sp.force) * (1 - float32(math.Exp(float64(-dt*rate))))
	if sp.force < 0.002 && target == 0 {
		sp.force = 0
	}
}

// stirring reports whether the silk still moves with the pointer.
func (sp *silkPointer) stirring() bool { return sp.force > 0 }

// silkInput is what the silk is given besides the scenes' own: the
// cover's colors, the treble, and the pointer. None, for a picture of it
// (its card, a test), paints it with no pointer, in tint's colors.
type silkInput struct {
	palette    []ui.Color
	bass, high float32 // as silkSwell has them
	pointer    *silkPointer
}

// silkSwell is the music as the silk takes it: the low end and the
// treble, rising over a half second and falling over more, so that the
// silk breathes with the music rather than starting at each beat, which,
// the beats close together, made it twitch.
type silkSwell struct {
	bass, high float32
	last       time.Time
}

// silkRise and silkFall are how long the swell takes to rise and to fall
// most of the way.
const (
	silkRise = 0.45
	silkFall = 1.4
)

// step follows the low end and the treble as they are at now.
func (sw *silkSwell) step(now time.Time, bass, high float32) {
	dt := float32(now.Sub(sw.last).Seconds())
	if sw.last.IsZero() || dt > 0.2 {
		dt = 1.0 / 30
	}
	sw.last = now
	follow := func(v *float32, to float32) {
		tau := float32(silkFall)
		if to > *v {
			tau = silkRise
		}
		*v += (to - *v) * (1 - float32(math.Exp(float64(-dt/tau))))
	}
	follow(&sw.bass, bass)
	follow(&sw.high, high)
}

// drawSilk paints the silk for a cover of tint in r at drift seconds.
func drawSilk(p *ui.Painter, r ui.Rect, tint ui.Color, drift, bass float32, in *silkInput) {
	var pal []ui.Color
	high := float32(0.3)
	var pointer [4]float32
	var pull [2]float32
	if in != nil {
		pal, bass, high = in.palette, in.bass, in.high
		if sp := in.pointer; sp != nil {
			pointer = [4]float32{sp.x, sp.y, sp.force, 0}
			pull = [2]float32{sp.vx, sp.vy}
		}
	}
	deep, cs := silkColors(tint, pal)
	silkShader().Draw(p, r, shader.Options{Uniforms: map[string]any{
		"Time": drift, "Bass": min(bass, 1), "High": min(high, 1),
		"Deep": deep, "C1": cs[0], "C2": cs[1], "C3": cs[2], "C4": cs[3],
		"Pointer": pointer, "Pull": pull,
	}})
}
