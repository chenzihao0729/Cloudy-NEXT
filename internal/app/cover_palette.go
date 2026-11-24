package app

import (
	"image"
	"math"
	"slices"

	"github.com/egoist/mygo/ui"
)

// coverPalette returns the colors a cover is made of, as a painter would
// take them from it for a backdrop in its colors: first the one it is
// mostly of, its ground, then up to four others standing out from it,
// the most telling first. A cover of one color has its ground alone.
//
// The cover's pixels, a few hundred of them, are grouped by k-means,
// begun from the colors farthest apart, so that a cover always gives the
// same; groups nearly alike are taken as one.
func coverPalette(img image.Image) []ui.Color {
	small := shrink(img, 32)
	px := make([][3]float64, 0, len(small.Pix)/4)
	for i := 0; i < len(small.Pix); i += 4 {
		if small.Pix[i+3] < 128 {
			continue
		}
		px = append(px, [3]float64{float64(small.Pix[i]), float64(small.Pix[i+1]), float64(small.Pix[i+2])})
	}
	if len(px) < 16 {
		return nil
	}
	groups := paletteGroups(px, 6)
	ground := groups[0]
	type pick struct {
		c     [3]float64
		score float64
	}
	var picks []pick
	for _, g := range groups[1:] {
		d := paletteDistance(ground.c, g.c)
		if d < 0.1 {
			continue
		}
		share := float64(g.n) / float64(len(px))
		// Large, colorful and unlike the ground tell the most.
		score := math.Sqrt(share) * (0.35 + 1.6*paletteChroma(g.c)) * min(1, d/0.25)
		picks = append(picks, pick{g.c, score})
	}
	slices.SortStableFunc(picks, func(a, b pick) int {
		switch {
		case a.score > b.score:
			return -1
		case a.score < b.score:
			return 1
		}
		return 0
	})
	out := []ui.Color{paletteColor(ground.c)}
	for i := 0; i < len(picks) && i < 4; i++ {
		out = append(out, paletteColor(picks[i].c))
	}
	return out
}

// paletteGroup is a group of a cover's pixels: their mean, and how many.
type paletteGroup struct {
	c [3]float64
	n int
}

// paletteGroups groups px into k or fewer, the largest first.
func paletteGroups(px [][3]float64, k int) []paletteGroup {
	var mean [3]float64
	for _, p := range px {
		mean[0], mean[1], mean[2] = mean[0]+p[0], mean[1]+p[1], mean[2]+p[2]
	}
	n := float64(len(px))
	mean = [3]float64{mean[0] / n, mean[1] / n, mean[2] / n}
	// Begun from the mean, and then each time from the pixel farthest
	// from all begun from so far.
	cents := [][3]float64{mean}
	far := make([]float64, len(px))
	for i, p := range px {
		far[i] = paletteDistance(p, mean)
	}
	for len(cents) < k {
		best, bestD := 0, -1.0
		for i, d := range far {
			if d > bestD {
				best, bestD = i, d
			}
		}
		if bestD < 0.02 {
			break
		}
		c := px[best]
		cents = append(cents, c)
		for i, p := range px {
			far[i] = min(far[i], paletteDistance(p, c))
		}
	}
	counts := make([]int, len(cents))
	sums := make([][3]float64, len(cents))
	for range 10 {
		clear(counts)
		clear(sums)
		for _, p := range px {
			bi, bd := 0, math.Inf(1)
			for j, c := range cents {
				if d := paletteDistance(p, c); d < bd {
					bi, bd = j, d
				}
			}
			sums[bi][0], sums[bi][1], sums[bi][2] = sums[bi][0]+p[0], sums[bi][1]+p[1], sums[bi][2]+p[2]
			counts[bi]++
		}
		for j := range cents {
			if counts[j] > 0 {
				m := float64(counts[j])
				cents[j] = [3]float64{sums[j][0] / m, sums[j][1] / m, sums[j][2] / m}
			}
		}
	}
	var out []paletteGroup
	for j, c := range cents {
		if counts[j] > 0 {
			out = append(out, paletteGroup{c, counts[j]})
		}
	}
	// Groups nearly alike are one.
	for i := 0; i < len(out); i++ {
		for j := len(out) - 1; j > i; j-- {
			if paletteDistance(out[i].c, out[j].c) < 0.07 {
				t := float64(out[j].n) / float64(out[i].n+out[j].n)
				for ch := range 3 {
					out[i].c[ch] += (out[j].c[ch] - out[i].c[ch]) * t
				}
				out[i].n += out[j].n
				out = slices.Delete(out, j, j+1)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b paletteGroup) int { return b.n - a.n })
	return out
}

// paletteDistance is how unlike colors a and b, of 0 to 255, look, from
// 0 to 1: green counts most, as the eye sees.
func paletteDistance(a, b [3]float64) float64 {
	dr, dg, db := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return math.Sqrt(2*dr*dr+4*dg*dg+3*db*db) / 765
}

// paletteChroma is how colorful c is, from 0 to 1.
func paletteChroma(c [3]float64) float64 {
	return (max(c[0], c[1], c[2]) - min(c[0], c[1], c[2])) / 255
}

func paletteColor(c [3]float64) ui.Color {
	ch := func(v float64) uint8 { return uint8(min(255, max(0, v)) + 0.5) }
	return ui.RGB(ch(c[0]), ch(c[1]), ch(c[2]))
}
