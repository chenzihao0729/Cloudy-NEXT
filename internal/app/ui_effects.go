package app

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/player"
)

// The sound effects, as a pane of their own from a song's details: on the
// left the presets, each drawn as the curve it makes; on the right what
// the sound is now, the ten bands as one curve to drag, the finer
// controls, and the sound placed round the head and in a room. They shape
// every song, and are kept.

// effectsView is the pane: open or not, and whether what was changed has
// yet to be heard at once and kept.
type effectsView struct {
	open    bool
	dirty   bool
	held    bool // a slider is being dragged, this frame
	compare bool // the effects are off while 按住听原声 is held
}

// effectPreset is a starting point: the bands, and how much of the bass,
// the room, the voice and the evening out, and what it is called.
type effectPreset struct {
	id, label, desc string
	bands           [10]float64
	bass, width     float64
	vocal, loud     float64
}

// effectPresets are the presets the pane offers, the first leaving the
// sound as it is.
var effectPresets = []effectPreset{
	{"flat", "原声", "不做处理，保留录音原貌", [10]float64{}, 0, 0, 0, 0},
	{"pop", "流行", "人声靠前，节奏明快", [10]float64{-1, 0, 1.5, 3, 3.5, 2.5, 1, 0, 1, 1.5}, 0.15, 0.1, 0.2, 0},
	{"rock", "摇滚", "鼓点有力，吉他明亮", [10]float64{4, 3, 2, 0, -1.5, -1, 1, 2.5, 3.5, 4}, 0.2, 0.15, 0, 0},
	{"jazz", "爵士", "温暖松弛，像在小酒馆", [10]float64{3, 2, 1, 2, -1, -1, 0, 1, 2, 3}, 0, 0.2, 0, 0},
	{"classical", "古典", "层次开阔，弦乐通透", [10]float64{4, 3, 2, 1, -1, -1, 0, 2, 3, 4}, 0, 0.3, 0, 0},
	{"electronic", "电子", "低频饱满，高频闪亮", [10]float64{4, 3.5, 1, 0, -2, 1.5, 1, 1, 3, 4}, 0.35, 0.25, 0, 0},
	{"vocal", "人声", "突出歌声，听清每个字", [10]float64{-2, -2, -1, 1, 3, 4, 3.5, 1.5, 0, -1}, 0, 0, 0.6, 0},
	{"bass", "重低音", "低频下潜，鼓点更震撼", [10]float64{6, 5, 4, 2, 0, 0, 0, 0, 0, 0}, 0.5, 0, 0, 0},
	{"night", "夜间", "高音柔和，音量平稳不吵人", [10]float64{0, 0, 0, 0, 0, 0, -1, -2, -3.5, -5}, 0, 0, 0, 0.6},
}

// customPreset is what the effects are once the listener shaped them.
const customPreset = "custom"

// iconHeadphones is the icon of 按住听原声.
var iconHeadphones = strokeIcon(`<path d="M4 15v-3a8 8 0 0 1 16 0v3"/><path d="M4 15h3v5H5a1 1 0 0 1-1-1z"/><path d="M20 15h-3v5h2a1 1 0 0 0 1-1z"/>`)

func (a *app) openEffects() {
	a.effects.open = true
	a.redraw()
}

// effectsChanged has what plays shaped as the settings say: heard at once
// once no slider is held, rather than after the second decoded ahead, and
// kept then.
func (a *app) effectsChanged() {
	a.pl.SetEffects(a.currentEffects(), false)
	a.effects.dirty = true
}

// effectsSettle hears and keeps what changed, once nothing is held.
func (a *app) effectsSettle() {
	if !a.effects.dirty || a.effects.held {
		return
	}
	a.effects.dirty = false
	a.pl.SetEffects(a.currentEffects(), true)
	a.settings.save()
}

// usePreset starts from p, turning the effects on.
func (a *app) usePreset(p effectPreset) {
	e := &a.settings.Effects
	e.On, e.Bands, e.Bass, e.Width, e.Vocal, e.Loudness = true, p.bands, p.bass, p.width, p.vocal, p.loud
	a.settings.EffectsPreset = p.id
	a.effectsChanged()
}

// shaped marks the effects the listener's own, turned on, as the curve or
// a finer control moved.
func (a *app) shaped() {
	a.settings.Effects.On = true
	a.settings.EffectsPreset = customPreset
	a.effectsChanged()
}

// presetLabel names the preset the effects are at.
func (a *app) presetLabel() string {
	for _, p := range effectPresets {
		if p.id == a.settings.EffectsPreset {
			return p.label
		}
	}
	switch a.settings.EffectsPreset {
	case customPreset:
		return "自定义"
	case smartPreset:
		return "智能音效"
	case moodPreset:
		return "情绪调音"
	}
	return effectPresets[0].label
}

// presetDesc says in a line what the preset the effects are at does: for
// those that choose themselves, what they chose for this song.
func (a *app) presetDesc() string {
	e := a.settings.Effects
	switch {
	case !e.On:
		return "音效已关闭，选一个预设就会打开"
	case a.effects.compare:
		return "正在听原声，松开恢复"
	}
	for _, p := range effectPresets {
		if p.id == a.settings.EffectsPreset {
			return p.desc
		}
	}
	switch a.settings.EffectsPreset {
	case smartPreset:
		if a.auto.note != "" {
			return a.auto.note + " · 每首歌自动挑选，切歌时平滑过渡"
		}
		return "按网易云的曲风，每首歌自动挑选最合适的预设"
	case moodPreset:
		if a.auto.note != "" {
			return a.auto.note
		}
		return "读取歌曲的情绪标签和歌词，按情绪调音"
	}
	return "拖动曲线上的点调整，双击归零"
}

// effectsFollowAuto shows, while the effects choose themselves, what they
// chose: the curve and the finer controls are where the song's are, so
// that moving one goes on from there.
func (a *app) effectsFollowAuto() {
	if a.effects.held || !a.autoOn() || a.auto.fx == nil {
		return
	}
	e, fx := &a.settings.Effects, a.auto.fx
	e.Bands, e.Bass, e.Treble, e.Width, e.Depth, e.Vocal, e.Loudness = fx.Bands, fx.Bass, fx.Treble, fx.Width, fx.Depth, fx.Vocal, fx.Loudness
}

// effectsDialog is the pane of the effects: the presets down the left, and
// on the right the curve, the finer controls, the space and the room.
func (a *app) effectsDialog(c *ui.Context) {
	v := &a.effects
	ink := a.inkOn(false)
	e := &a.settings.Effects
	w, h := c.Size()
	pw := min(760, w-80)
	v.held = false
	if v.open {
		a.effectsFollowAuto()
	}
	a.dialog(c, "effects", 0.35, &v.open, func(panel ui.Element) {
		panel.Width(pw).MaxHeight(h - 56).TextColor(ink.text)
		a.dialogPane(panel, 26, false)
		panel.Children(func() {
			ui.Row(c).Padding(20, 20, 12, 28).Gap(14).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "音效").FontSize(20).FontWeight(700).Grow(1)
				if settingSwitch(c.Key("effects-on"), &e.On).Label("开启音效").Changed() {
					a.effectsChanged()
				}
				if iconButton(c.Key("dialog-close"), iconX, 16, ink.muted, "关闭").Clicked() {
					v.open = false
				}
			})
			ui.Scroll(c.Key("effects-scroll")).Shrink(1).MinHeight(0).Padding(4, 28, 26, 28).Children(func() {
				ui.Row(c).AlignItems(ui.Stretch).Children(func() {
					a.effectsRail(c, ink)
					ui.Box(c).Width(1).Shrink(0).Background(ink.line).Margin(0, 24, 0, 22)
					ui.Column(c.Key("fx-main")).Grow(1).Shrink(1).MinWidth(0).Children(func() {
						a.effectsNow(c, ink)
						a.equalizer(c, ink)
						a.effectsFine(c, ink)
						ui.Box(c).Height(1).Background(ink.line).Margin(16, 0, 10, 0)
						a.effectsSpace(c, ink)
					})
				})
			})
		})
	})
	if v.compare && !v.open {
		a.effectsCompare(false)
	}
	a.effectsSettle()
}

// effectsCompare turns the effects off while 按住听原声 is held, and back
// as it lets go, heard at once either way.
func (a *app) effectsCompare(held bool) {
	if a.effects.compare == held {
		return
	}
	a.effects.compare = held
	fx := a.currentEffects()
	if held {
		fx.On = false
	}
	a.pl.SetEffects(fx, true)
	a.redraw()
}

// railHeading is a group's name in the presets' rail.
func railHeading(c *ui.Context, ink paneInk, title string, top float32) {
	ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(ink.faint).Margin(top, 0, 6, 10)
}

// effectsRail is the presets down the left: those that choose themselves,
// those of a kind of music, each drawn as its curve, and the listener's own.
func (a *app) effectsRail(c *ui.Context, ink paneInk) {
	id := a.settings.EffectsPreset
	ui.Column(c.Key("fx-rail")).Width(176).Shrink(0).Gap(2).Children(func() {
		railHeading(c, ink, "自动", 2)
		for _, m := range []struct {
			id, label string
			icon      *ui.SVG
		}{{smartPreset, "智能音效", iconWand}, {moodPreset, "情绪调音", iconHeart}} {
			icon := m.icon
			if a.railItem(c, "auto-"+m.id, m.label, id == m.id, func(c *ui.Context, col ui.Color) {
				ui.Box(c).Size(30, 16).Shrink(0).Center().Children(func() {
					ui.Icon(c, icon).Size(15, 15).TextColor(col)
				})
			}) {
				a.useAuto(m.id)
			}
		}
		railHeading(c, ink, "风格", 14)
		for i := range effectPresets {
			p := &effectPresets[i]
			if a.railItem(c, "preset-"+p.id, p.label, id == p.id, func(c *ui.Context, col ui.Color) {
				bands := p.bands
				ui.Box(c).Size(30, 16).Shrink(0).Draw(func(pt *ui.Painter, r ui.Rect) { drawShape(pt, r, bands, col) })
			}) {
				a.usePreset(*p)
			}
		}
		ui.Box(c).Height(1).Background(ink.line).Margin(8, 10, 8, 10)
		if a.railItem(c, "preset-custom", "自定义", id == customPreset, func(c *ui.Context, col ui.Color) {
			ui.Box(c).Size(30, 16).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) { drawFaders(p, r, col) })
		}) && id != customPreset {
			// From where the sound is now.
			a.shaped()
		}
	})
}

// railItem is a preset in the rail: its glyph and its name, on a quiet fill
// while it is chosen, the glyph in the accent while the effects are on. It
// reports a click.
func (a *app) railItem(c *ui.Context, key, label string, chosen bool, glyph func(c *ui.Context, col ui.Color)) bool {
	ink := a.inkOn(false)
	b := ui.ButtonBase(c.Key(key)).Row().AlignItems(ui.Center).Justify(ui.Start).Height(32).Radius(8).PaddingX(10).Gap(10).
		FocusRing(false).Label(label)
	bg := ui.Transparent
	text, weight := ink.muted, 400
	col := ink.faint
	switch {
	case chosen:
		bg, text, weight, col = ink.hover, ink.text, 600, ink.muted
		if a.settings.Effects.On {
			col = a.pal.accent
		}
	case b.Hovered():
		bg, text = ink.text.Alpha(0.035), ink.text
	}
	b.Background(b.AnimateColor("bg", bg, motionHover))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.98
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() {
		glyph(c, b.AnimateColor("glyph", col, motionHover))
		ui.Text(c, label).FontSize(13.5).FontWeight(weight).TextColor(text).SingleLine()
	})
	return b.Clicked()
}

// effectsNow is what the effects are at: the preset's name, what it does,
// and 按住听原声.
func (a *app) effectsNow(c *ui.Context, ink paneInk) {
	e := a.settings.Effects
	ui.Row(c).AlignItems(ui.Start).Gap(12).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			title := ink.text
			if !e.On {
				title = ink.muted
			}
			ui.Text(c, a.presetLabel()).FontSize(19).FontWeight(600).TextColor(title).SingleLine()
			ui.Text(c, a.presetDesc()).FontSize(12.5).TextColor(ink.muted).MaxLines(2).LineHeight(1.4)
		})
		b := ui.ButtonBase(c.Key("fx-compare")).Row().AlignItems(ui.Center).Height(28).PaddingX(12).Gap(6).Radius(999).Shrink(0).
			FocusRing(false).Label("按住听原声").Disabled(!e.On)
		bg := ui.Transparent
		switch {
		case b.Pressed() && e.On:
			bg = ink.press
		case b.Hovered() && e.On:
			bg = ink.hover
		}
		b.Background(b.AnimateColor("bg", bg, motionHover)).Border(0.5, ink.line).TextColor(ink.muted)
		if !e.On {
			b.Opacity(0.5)
		}
		b.Children(func() {
			ui.Icon(c, iconHeadphones).Size(14, 14)
			ui.Text(c, "按住听原声").FontSize(12.5).SingleLine()
		})
		a.effectsCompare(b.Pressed() && e.On)
	})
}

// eqHeight is how tall the equalizer's bands run.
const eqHeight = 128

// knobEQ is the size of a band's point on the curve.
const knobEQ = 10

// equalizer is the ten bands as one curve, filled to the zero line, a
// point on it for each band to drag up or down. A preset chosen, the curve
// glides to its shape; held to hear the sound as it was, it lies flat.
func (a *app) equalizer(c *ui.Context, ink paneInk) {
	e := &a.settings.Effects
	live := e.On && !a.effects.compare
	color := ink.faint
	if live {
		color = a.pal.accent
	}
	fill, _ := a.cardColors()
	box := ui.Column(c.Key("eq")).Margin(14, 0, 0, 0).Padding(22, 8, 10, 8).Radius(14).Background(fill)
	box.Children(func() {
		row := ui.Row(c.Key("eq-bands")).AlignItems(ui.Stretch)
		glide := ui.Animation{Spring: ui.SpringOptions{Duration: 520 * time.Millisecond}}
		if a.effects.held || a.motion != motionFull {
			glide = ui.Animation{Duration: time.Millisecond}
		}
		var shown [10]float32
		for i, g := range e.Bands {
			if a.effects.compare {
				g = 0
			}
			shown[i] = row.AnimateTo("g"+strconv.Itoa(i), float32(g), glide)
		}
		color = row.AnimateColor("curve", color, motionHover)
		row.Draw(func(p *ui.Painter, r ui.Rect) {
			top := r.Y + knobEQ/2
			span := float32(eqHeight - knobEQ)
			yOf := func(g float32) float32 { return top + span*(1-(g+player.EffectRange)/(2*player.EffectRange)) }
			for _, g := range []float32{12, 6, -6, -12} {
				p.Fill(ui.Rect{X: r.X + 10, Y: yOf(g) - 0.5, W: r.W - 20, H: 1}, ink.text.Alpha(0.04), 0)
			}
			zero := yOf(0)
			p.Fill(ui.Rect{X: r.X + 10, Y: zero - 0.5, W: r.W - 20, H: 1}, ink.text.Alpha(0.1), 0)
			col := r.W / float32(len(shown))
			pts := make([]ui.Point, len(shown))
			for i, g := range shown {
				pts[i] = ui.Point{X: r.X + col*(float32(i)+0.5), Y: yOf(g)}
			}
			area := (&ui.Path{}).MoveTo(pts[0].X, zero).LineTo(pts[0].X, pts[0].Y)
			curveThrough(area, pts)
			area.LineTo(pts[len(pts)-1].X, zero).Close()
			p.FillPath(area, color.Alpha(0.1))
			p.StrokePath(curveThrough((&ui.Path{}).MoveTo(pts[0].X, pts[0].Y), pts), 2, color)
		})
		row.Children(func() {
			for i := range e.Bands {
				ui.Column(c.Key(i)).Grow(1).Basis(0).AlignItems(ui.Center).Gap(6).Children(func() {
					a.bandSlider(c, i, shown[i], color, ink)
					ui.Text(c, freqLabel(player.EffectBands[i])).FontSize(10.5).FontFeatures("tnum").TextColor(ink.faint)
				})
			}
		})
	})
}

// curveThrough adds to path, which is at pts[0], a curve through the rest
// of pts, as smooth as Catmull-Rom makes it.
func curveThrough(path *ui.Path, pts []ui.Point) *ui.Path {
	for i := 0; i+1 < len(pts); i++ {
		p0, p1, p2, p3 := pts[max(i-1, 0)], pts[i], pts[i+1], pts[min(i+2, len(pts)-1)]
		path.CubeTo(p1.X+(p2.X-p0.X)/6, p1.Y+(p2.Y-p0.Y)/6, p2.X-(p3.X-p1.X)/6, p2.Y-(p3.Y-p1.Y)/6, p2.X, p2.Y)
	}
	return path
}

// drawShape draws bands small in r, a preset's glyph: its curve, ±7 dB
// to the edges.
func drawShape(p *ui.Painter, r ui.Rect, bands [10]float64, col ui.Color) {
	pts := make([]ui.Point, len(bands))
	for i, g := range bands {
		g = max(-7, min(7, g))
		pts[i] = ui.Point{X: r.X + 1 + (r.W-2)*float32(i)/float32(len(bands)-1), Y: r.Y + r.H/2 - float32(g/7)*(r.H/2-1.5)}
	}
	p.StrokePath(curveThrough((&ui.Path{}).MoveTo(pts[0].X, pts[0].Y), pts), 1.5, col)
}

// drawFaders draws three faders small in r, the glyph of 自定义.
func drawFaders(p *ui.Painter, r ui.Rect, col ui.Color) {
	for i, at := range []float32{0.35, 0.7, 0.5} {
		x := r.X + r.W*(0.25+0.25*float32(i))
		p.Fill(ui.Rect{X: x - 0.75, Y: r.Y + 1, W: 1.5, H: r.H - 2}, col.Alpha(0.55), 0.75)
		y := r.Y + 1 + (r.H-2)*(1-at)
		p.Fill(ui.Rect{X: x - 3, Y: y - 1.5, W: 6, H: 3}, col, 1.5)
	}
}

// bandSlider is band i, standing up over the curve: up lifts it, down
// cuts it. It shows its point on the curve at g, where the curve is drawn
// now, and its gain above while it is pointed at or held.
func (a *app) bandSlider(c *ui.Context, i int, g float32, color ui.Color, ink paneInk) {
	e := &a.settings.Effects
	label := freqLabel(player.EffectBands[i]) + "Hz"
	ui.Box(c.Key("band-box")).Children(func() {
		s := ui.SliderBase(c.Key("band"), &e.Bands[i], -player.EffectRange, player.EffectRange).Vertical().
			Size(28, eqHeight).PaddingY(knobEQ / 2).Step(0.5).FocusRing(false).Label(label).Cursor(ui.CursorResizeNS).
			Tooltip(label + " · 双击归零")
		hot := s.Hovered() || s.Pressed()
		shown := float32(0)
		if hot && !a.effects.compare {
			shown = 1
		}
		tag := s.AnimateTo("tag", shown, motionHover)
		if s.Changed() {
			a.shaped()
		}
		if s.Pressed() {
			a.effects.held = true
		}
		if s.DoubleClicked() {
			// Back to nothing, as a fader is.
			e.Bands[i] = 0
			a.shaped()
		}
		size := float32(knobEQ)
		if hot {
			size += 3
		}
		size = s.AnimateTo("size", size, ui.Snappy)
		s.Draw(func(p *ui.Painter, r ui.Rect) {
			top, foot := r.Y+knobEQ/2, r.Y+r.H-knobEQ/2
			f := (g + player.EffectRange) / (2 * player.EffectRange)
			y := foot - (foot-top)*f
			if tag > 0.01 {
				p.Fill(ui.Rect{X: r.X + r.W/2 - 0.5, Y: top, W: 1, H: foot - top}, ink.text.Alpha(0.06*tag), 0.5)
			}
			k := ui.Rect{X: r.X + (r.W-size)/2, Y: y - size/2, W: size, H: size}
			p.Shadow(k, size/2, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.18))
			p.Fill(k, white, size/2)
			p.Stroke(k, color, size/2, 1.6)
		})
		// The gain, over the column while it is pointed at.
		ui.Text(c, gainLabel(e.Bands[i])).FontSize(10.5).FontWeight(600).FontFeatures("tnum").TextColor(ink.muted).SingleLine().
			Absolute().Top(-18).Left(0).Right(0).TextAlign(ui.Center).Opacity(tag)
	})
}

// fineControls are the finer controls: what each moves, its name, what it
// does, and whether it goes down as well as up.
var fineControls = []struct {
	key, label, tip string
	lo              float64
}{
	{"bass", "低音", "低音：向左减弱，向右加强", -1},
	{"treble", "高音", "高音：向左减弱，向右加强", -1},
	{"vocal", "人声", "人声增强：居中的人声更靠前、更清楚，伴奏不受影响", 0},
	{"loud", "响度", "响度均衡：压低过响的段落、抬起轻柔的段落", 0},
	{"depth", "纵深", "环绕深度：声音前后的层次", 0},
	{"width", "声场", "声场宽度：左右声场更开阔", 0},
}

// fineValue is where the finer control of key is.
func fineValue(e *player.Effects, key string) *float64 {
	switch key {
	case "bass":
		return &e.Bass
	case "treble":
		return &e.Treble
	case "vocal":
		return &e.Vocal
	case "loud":
		return &e.Loudness
	case "depth":
		return &e.Depth
	}
	return &e.Width
}

// effectsFine is the finer controls, two by two, and 重置 while any is
// moved.
func (a *app) effectsFine(c *ui.Context, ink paneInk) {
	e := &a.settings.Effects
	moved := false
	for _, f := range fineControls {
		moved = moved || *fineValue(e, f.key) != 0
	}
	ui.Row(c).AlignItems(ui.Center).Height(22).Margin(18, 0, 4, 0).Children(func() {
		ui.Text(c, "细调").FontSize(11.5).FontWeight(600).TextColor(ink.faint).Grow(1)
		if moved {
			b := ui.ButtonBase(c.Key("enhance-reset")).Row().AlignItems(ui.Center).Gap(4).Height(22).PaddingX(6).Radius(8).
				FocusRing(false).Label("重置细调")
			text := ink.faint
			if b.Hovered() {
				text = ink.text
			}
			b.TextColor(text).Children(func() {
				ui.Icon(c, iconReload).Size(12, 12)
				ui.Text(c, "重置").FontSize(12)
			})
			if b.Clicked() {
				e.Bass, e.Treble, e.Vocal, e.Loudness, e.Depth, e.Width = 0, 0, 0, 0, 0, 0
				a.shaped()
			}
		}
	})
	ui.Grid(c).Columns(2).GapX(28).GapY(2).Children(func() {
		for _, f := range fineControls {
			ui.Row(c.Key(f.key)).AlignItems(ui.Center).Gap(12).Height(32).MinWidth(0).Children(func() {
				ui.Text(c, f.label).FontSize(13).Width(30).Shrink(0).SingleLine().Tooltip(f.tip)
				a.effectSlider(c, f.tip, fineValue(e, f.key), f.lo, 0, "", a.shaped)
			})
		}
	})
}

// effectsSpace is the sound placed round the head, how fast it goes round
// in 8D, and the room it plays in: these go with any preset.
func (a *app) effectsSpace(c *ui.Context, ink paneInk) {
	e := &a.settings.Effects
	label := func(s string) {
		ui.Text(c, s).FontSize(13).Width(30).Shrink(0).SingleLine()
	}
	spatial := []string{"", player.Spatial8D, player.Spatial360}
	at := 0
	for i, id := range spatial {
		if e.Spatial == id {
			at = i
		}
	}
	ui.Row(c).AlignItems(ui.Center).Gap(12).Height(38).Children(func() {
		label("空间")
		a.segmentedControl(c.Key("fx-spatial"), &at, []string{"关", "8D 环绕", "360° 环绕"}, true, func(i int) {
			if e.Spatial == spatial[i] {
				return
			}
			e.Spatial = spatial[i]
			if e.Spatial != "" {
				e.On = true
			}
			a.effectsChanged()
		})
		ui.Spacer(c)
		ui.Text(c, "戴耳机效果最好").FontSize(12).TextColor(ink.faint).SingleLine()
	})
	if e.Spatial == player.Spatial8D {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Height(34).Children(func() {
			ui.Text(c, "转速").FontSize(13).Width(30).Shrink(0).TextColor(ink.muted).SingleLine()
			period := 16 - 12*e.Orbit
			a.effectSlider(c, "转速", &e.Orbit, 0, 200, fmt.Sprintf("约 %.0f 秒一圈", period), a.effectsChanged)
		})
	}
	room := 0
	labels := make([]string, len(player.Rooms))
	for i, r := range player.Rooms {
		labels[i] = r.Label
		if r.ID == e.Room {
			room = i
		}
	}
	ui.Row(c).AlignItems(ui.Center).Gap(12).Height(38).Margin(6, 0, 0, 0).Children(func() {
		label("环境")
		a.segmentedControl(c.Key("fx-room"), &room, labels, true, func(i int) {
			if e.Room == player.Rooms[i].ID {
				return
			}
			e.Room = player.Rooms[i].ID
			if e.Room != "" {
				e.On = true
			}
			a.effectsChanged()
		})
	})
}

// useAuto has the effects choose themselves for each song, by id: by
// genre, or by mood.
func (a *app) useAuto(id string) {
	a.settings.Effects.On = true
	a.settings.EffectsPreset = id
	a.auto.song, a.auto.fx, a.auto.note = 0, nil, ""
	a.autoFollow()
	a.effectsChanged()
}

// effectSlider is the slider of an effect, from lo, -1 or 0, to 1, and how
// much beside it, or note if one is given: one that goes down and up fills
// from its middle, and a double click puts it back to 0. It is width wide,
// or as wide as there is room for at 0; changed is called as it moves.
func (a *app) effectSlider(c *ui.Context, label string, v *float64, lo float64, width float32, note string, changed func()) {
	ink := a.inkOn(false)
	on := a.settings.Effects.On
	row := ui.Row(c.Key(label)).Gap(10).AlignItems(ui.Center)
	if width == 0 {
		row.Grow(1).Shrink(1).MinWidth(0)
	}
	row.Children(func() {
		s := ui.SliderBase(c.Key("slider"), v, lo, 1).Height(22).Step(0.01).FocusRing(false).Label(label).Cursor(ui.CursorPointer)
		if width == 0 {
			s.Grow(1).Shrink(1).MinWidth(60)
		} else {
			s.Width(width)
		}
		if s.Changed() {
			changed()
		}
		if s.Pressed() {
			a.effects.held = true
		}
		if s.DoubleClicked() {
			*v = 0
			changed()
		}
		thick := float32(3)
		if s.Hovered() || s.Pressed() {
			thick = 4
		}
		thick = s.AnimateTo("thick", thick, ui.Snappy)
		accent := ink.faint
		if on {
			accent = a.pal.accent
		}
		at, from := float32((*v-lo)/(1-lo)), float32(-lo/(1-lo))
		s.Draw(func(p *ui.Painter, r ui.Rect) {
			const knob = 14
			y := r.Y + (r.H-thick)/2
			span := r.W - knob
			x := func(f float32) float32 { return r.X + knob/2 + span*f }
			p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: thick}, ink.press, thick/2)
			if lo < 0 {
				// The middle, where it does nothing.
				p.Fill(ui.Rect{X: x(from) - 0.5, Y: y - 3, W: 1, H: thick + 6}, ink.faint, 0)
			}
			a0, a1 := min(x(from), x(at)), max(x(from), x(at))
			p.Fill(ui.Rect{X: a0, Y: y, W: max(a1-a0, 0.1), H: thick}, accent, thick/2)
			k := ui.Rect{X: x(at) - knob/2, Y: r.Y + (r.H-knob)/2, W: knob, H: knob}
			p.Shadow(k, knob/2, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.25))
			p.Fill(k, white, knob/2)
		})
		if note != "" {
			ui.Text(c, note).FontSize(12).FontFeatures("tnum").TextColor(ink.faint).SingleLine()
			return
		}
		ui.Text(c, effectAmount(*v, lo)).FontSize(11.5).FontFeatures("tnum").TextColor(ink.faint).Width(38).Shrink(0).TextAlign(ui.End)
	})
}

// effectAmount says how much of an effect from lo there is: 0, 40%, +15%.
func effectAmount(v, lo float64) string {
	n := int(math.Round(math.Abs(v) * 100))
	switch {
	case n == 0:
		return "0"
	case lo >= 0:
		return strconv.Itoa(n) + "%"
	case v > 0:
		return "+" + strconv.Itoa(n) + "%"
	}
	return "−" + strconv.Itoa(n) + "%"
}

// gainLabel says a band's gain: +3, -2.5, 0.
func gainLabel(g float64) string {
	switch {
	case g == 0:
		return "0"
	case g == float64(int(g)):
		return fmt.Sprintf("%+d", int(g))
	}
	return fmt.Sprintf("%+.1f", g)
}

// freqLabel says a band's frequency: 31, 1k, 16k.
func freqLabel(f float64) string {
	if f >= 1000 {
		return strconv.Itoa(int(f/1000)) + "k"
	}
	return strconv.Itoa(int(f))
}

// effectsNote is what the menu says of the effects beside their name.
func (a *app) effectsNote() string {
	if !a.settings.Effects.On {
		return "关闭"
	}
	return a.presetLabel()
}
