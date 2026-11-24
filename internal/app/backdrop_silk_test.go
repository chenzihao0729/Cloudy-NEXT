package app

import (
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A cover mostly of one color, with a band of another, gives its ground
// first and the other after it; one of a single color, its ground alone.
func TestCoverPaletteGroundFirst(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			c := color.RGBA{20, 60, 160, 255} // blue ground
			if y >= 48 {
				c = color.RGBA{240, 200, 40, 255} // a yellow band
			}
			img.Set(x, y, c)
		}
	}
	pal := coverPalette(img)
	if len(pal) != 2 {
		t.Fatalf("palette %v, want ground and band", pal)
	}
	if pal[0].B < 140 || pal[1].R < 220 {
		t.Errorf("palette %v: ground not blue first, band not yellow after", pal)
	}
	flat := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := range flat.Pix {
		flat.Pix[i] = 200
	}
	if pal := coverPalette(flat); len(pal) != 1 {
		t.Errorf("one color: palette %v", pal)
	}
}

// The silk's colors are as light as their place asks, whatever the
// cover: white reads over the ground and the broad folds.
func TestSilkColorsKeepTheirTones(t *testing.T) {
	covers := [][]ui.Color{
		nil,
		{ui.RGB(250, 250, 250)},
		{ui.RGB(20, 60, 160), ui.RGB(240, 200, 40)},
		{ui.RGB(10, 10, 10), ui.RGB(200, 30, 90), ui.RGB(30, 200, 90), ui.RGB(90, 30, 200), ui.RGB(250, 240, 230)},
	}
	for i, pal := range covers {
		deep, cs := silkColors(ui.RGB(40, 90, 220), pal)
		got := []ui.Color{deep, cs[0], cs[1], cs[2], cs[3]}
		for j, c := range got {
			if l := luminance(c); math.Abs(l-silkTones[j]) > 0.06 {
				t.Errorf("cover %d color %d: luminance %.3f, want about %.2f", i, j, l, silkTones[j])
			}
		}
	}
}

// Close beats, four a second, the low end leaping to full and back to
// nothing, swell the silk evenly: it breathes, not starts at each one.
func TestSilkSwellBreathesWithCloseBeats(t *testing.T) {
	var sw silkSwell
	now := time.Unix(0, 0)
	lo, hi := float32(1), float32(0)
	for i := range 30 * 8 {
		now = now.Add(time.Second / 30)
		bass := float32(0)
		if i%8 < 2 {
			bass = 1
		}
		sw.step(now, bass, 0)
		if i >= 30*4 {
			lo, hi = min(lo, sw.bass), max(hi, sw.bass)
		}
	}
	if hi-lo > 0.2 {
		t.Errorf("swell goes %.2f to %.2f between beats", lo, hi)
	}
	if lo < 0.15 {
		t.Errorf("swell %.2f: the beats hardly show", lo)
	}
}

// The pointer stirs the silk as it moves, and the silk settles after it
// stops or leaves.
func TestSilkPointerStirsAndSettles(t *testing.T) {
	var sp silkPointer
	sp.aim(0, 0, true)
	sp.step(1.0 / 60)
	if sp.stirring() {
		t.Fatal("stirred by a pointer that only came in")
	}
	for i := range 20 {
		sp.aim(float32(i)*0.02, 0, true)
		sp.step(1.0 / 60)
	}
	if sp.force < 0.3 {
		t.Fatalf("force %.2f while the pointer moves", sp.force)
	}
	for range 60 * 8 {
		sp.step(1.0 / 60)
	}
	if sp.stirring() {
		t.Errorf("still stirring %.4f eight seconds after it stopped", sp.force)
	}
	for i := range 20 {
		sp.aim(float32(i)*0.02, 0, true)
		sp.step(1.0 / 60)
	}
	sp.aim(0, 0, false)
	for range 60 * 8 {
		sp.step(1.0 / 60)
	}
	if sp.stirring() {
		t.Error("still stirring after the pointer left")
	}
}
