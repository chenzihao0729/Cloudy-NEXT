package app

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// glassBackdrop is a picture the glass preview can lie over: a sky, and
// waves in it from mid to low, glossy with hi along their crests.
type glassBackdrop struct {
	label             string
	sky, mid, low, hi ui.Color
}

// glassBackdrops are the preview's pictures: the first as vibedesign.tips's
// liquid glass demo has it, the last in the cover's colors.
var glassBackdrops = []glassBackdrop{
	{label: "海湾", sky: ui.RGB(222, 232, 246), mid: ui.RGB(70, 140, 240), low: ui.RGB(10, 52, 178), hi: ui.RGB(196, 240, 255)},
	{label: "晚霞", sky: ui.RGB(255, 226, 208), mid: ui.RGB(246, 124, 112), low: ui.RGB(134, 38, 104), hi: ui.RGB(255, 236, 196)},
	{label: "青柠", sky: ui.RGB(228, 246, 222), mid: ui.RGB(84, 200, 140), low: ui.RGB(12, 100, 92), hi: ui.RGB(220, 255, 206)},
	{label: "薰衣草", sky: ui.RGB(236, 230, 250), mid: ui.RGB(150, 128, 240), low: ui.RGB(62, 38, 152), hi: ui.RGB(242, 220, 255)},
	{label: "封面色"},
}

// glassBackdropOf returns picture i, the cover's for the last, darker in
// its sky in dark mode.
func glassBackdropOf(i int, cover ui.Color, dark bool) glassBackdrop {
	i = min(max(i, 0), len(glassBackdrops)-1)
	b := glassBackdrops[i]
	if i == len(glassBackdrops)-1 {
		b.sky, b.mid, b.low, b.hi = cover.Mix(white, 0.82), cover, cover.Mix(ui.RGB(0, 0, 0), 0.55), cover.Mix(white, 0.7)
	}
	if dark {
		b.sky = b.sky.Mix(ui.RGB(18, 18, 24), 0.62)
		b.mid = b.mid.Mix(ui.RGB(0, 0, 0), 0.18)
	}
	return b
}

// glassFlowSource paints the waves: four, one behind another, rolling
// slowly, each shaded down from its glossy crest.
const glassFlowSource = `package main

var Time float
var Sky vec4
var Mid vec4
var Low vec4
var Hi vec4

func Fragment(dst vec4, pos vec2) vec4 {
	size := rectSize()
	uv := pos / size
	t := Time
	col := mix(Sky.rgb, mix(Sky.rgb, Mid.rgb, 0.4), smoothstep(0.0, 0.7, uv.y))
	for i := 0; i < 4; i++ {
		n := float(i)
		x := uv.x*(1.5+0.3*n) + n*1.9
		crest := 0.26 + 0.17*n + 0.08*sin(x*2.1+t*(0.3+0.07*n)) + 0.035*sin(x*4.6-t*(0.42-0.05*n)+n) + 0.06*(0.5-uv.x)*(1.0-0.4*n)
		d := uv.y - crest
		body := mix(Mid.rgb, Low.rgb, (n+1.0)/4.0)
		shade := body * (1.0 - 0.3*smoothstep(0.0, 0.4, d))
		shade = mix(shade, Hi.rgb, 0.6*exp(-max(d, 0.0)*34.0))
		col = mix(col, shade, 0.94*smoothstep(-0.01, 0.02, d))
	}
	col += Hi.rgb * 0.14 * exp(-length(uv-vec2(0.12, 0.0))*2.4)
	return vec4(col, 1)
}
`

var glassFlowShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(glassFlowSource) })

// glassPreview shows the glass as the two sliders under it set it, as they
// move: a small player of the panes' even glass (none of the bar's darker
// ends) over waves rolling by, whose picture can be chosen at its corner.
func (a *app) glassPreview(c *ui.Context) {
	s := &a.settings
	sv := &a.settingsView
	pv := &sv.glassPv
	dark := a.pal.dark
	still := c.Preferences().ReduceMotion || s.PowerSaver
	bg := glassBackdropOf(glassBackdropIndex(s.GlassPreviewBg), a.thumbTint(), dark)
	shown := previewBackdrop{style: bg.label, tint: bg.mid, fog: dark, moving: !still}
	box := ui.Box(c.Key("glass-preview")).Height(196).Radius(20).Margin(12, 20, 8, 20).Background(white.Alpha(0.06)).Label("玻璃预览")
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		w, h := int(r.W), int(r.H)
		if w < 2 || h < 2 {
			return
		}
		if pv.canvas == nil || pv.w != w || pv.h != h {
			pv.canvas, pv.w, pv.h = ui.NewCanvas(w, h), w, h
			pv.at = time.Time{}
		}
		now := p.Now()
		if pv.start.IsZero() {
			pv.start = now
		}
		drift := float32(now.Sub(pv.start).Seconds())
		if still {
			drift = 4
		}
		if pv.at.IsZero() || shown != pv.shown || shown.moving && now.Sub(pv.at) >= backdropFrame-2*time.Millisecond {
			pv.at, pv.shown = now, shown
			pv.canvas.Draw(p, func(cp *ui.Painter) {
				b := ui.Rect{W: float32(w), H: float32(h)}
				if !cp.Accelerated() {
					cp.Fill(b, bg.mid, 0)
					return
				}
				glassFlowShader().Draw(cp, b, shader.Options{Uniforms: map[string]any{
					"Time": drift, "Sky": bg.sky, "Mid": bg.mid, "Low": bg.low, "Hi": bg.hi,
				}})
			})
		}
		if shown.moving {
			p.After(backdropFrame)
		}
	})
	ink := ui.RGB(22, 22, 28)
	if dark {
		ink = white
	}
	box.Children(func() {
		if pv.canvas != nil {
			ui.Image(c, pv.canvas.Bitmap()).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).Radius(20)
		}
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Children(func() {
			card := ui.Column(c.Key("glass-preview-card")).Width(292).Padding(16, 18).Gap(10).Radius(26).
				Shadow(0, 14, 34, -10, ui.RGBA(0, 0, 0, 0.32)).
				Material(barGlass{dark: dark, clear: s.glassClear(dark), bend: float32(s.GlassBend), surface: true,
					veil: [2]float32{0.01, 0.01}, basis: 152})
			card.Children(func() {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(48, 48).Radius(14).Background(bg.low.Mix(bg.mid, 0.45)).Center().
						Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.25)).Children(func() {
						ui.Icon(c, iconSparkles).Size(20, 20).TextColor(white.Alpha(0.9))
					})
					ui.Column(c).Gap(2).Children(func() {
						ui.Text(c, "玻璃时光").FontSize(16).FontWeight(700).TextColor(ink)
					ui.Text(c, "Cloudy-NEXT").FontSize(13).TextColor(ink.Alpha(0.6))
					})
				})
				ui.Box(c).Height(22).Draw(func(p *ui.Painter, r ui.Rect) {
					y := r.Y + r.H - 4
					p.Fill(ui.Rect{X: r.X, Y: y - 2, W: r.W, H: 4}, ink.Alpha(0.16), 2)
					fill := ui.Rect{X: r.X, Y: y - 2, W: r.W * 0.36, H: 4}
					p.Fill(fill, ink.Alpha(0.82), 2)
					p.Fill(ui.Rect{X: fill.X + fill.W - 6, Y: y - 6, W: 12, H: 12}, white, 6)
				}).Children(func() {
					ui.Row(c).Absolute().Top(0).Left(0).Right(0).Justify(ui.SpaceBetween).Children(func() {
						ui.Text(c, "1:12").FontSize(10.5).FontWeight(600).TextColor(ink.Alpha(0.72))
						ui.Text(c, "-2:31").FontSize(10.5).FontWeight(600).TextColor(ink.Alpha(0.72))
					})
				})
				ui.Row(c).Justify(ui.Center).AlignItems(ui.Center).Gap(30).Children(func() {
					ui.Icon(c, iconPrev).Size(18, 18).TextColor(ink.Alpha(0.88))
					ui.Icon(c, iconPause).Size(22, 22).TextColor(ink.Alpha(0.88))
					ui.Icon(c, iconNext).Size(18, 18).TextColor(ink.Alpha(0.88))
				})
			})
		})
		// The pictures to choose from, at the corner.
		ui.Row(c).Absolute().Right(14).Bottom(12).Gap(8).Children(func() {
			for i := range glassBackdrops {
				b := glassBackdropOf(i, a.thumbTint(), false)
				ring := white.Alpha(0.4)
				if i == glassBackdropIndex(s.GlassPreviewBg) {
					ring = white.Alpha(0.95)
				}
				dot := ui.ButtonBase(c.Key("glass-bg-"+b.label)).Size(18, 18).Radius(999).Background(b.mid).
					Border(2, ring).FocusRing(false).
					Label("预览背景：" + b.label).Tooltip(b.label)
				if dot.Clicked() {
					s.GlassPreviewBg = b.label
					if i == 0 {
						s.GlassPreviewBg = ""
					}
					a.changed()
				}
			}
		})
	})
}

// glassBackdropIndex returns the place in glassBackdrops of the picture
// labelled label, the first for one unknown.
func glassBackdropIndex(label string) int {
	for i, b := range glassBackdrops {
		if b.label == label {
			return i
		}
	}
	return 0
}
