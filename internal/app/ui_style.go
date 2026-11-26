package app

import (
	"errors"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Roaming a style plays on through songs of one kind, as the app's radios do,
// as "style:<label>". NetEase's own scene modes aren't open beyond a few, and
// need the listener signed in: this goes by the public playlists of the style,
// best played first, which works for anyone, though not to their taste.

// A roamStyle is a style to roam: what it is called, what shows on its tile,
// and what finds its playlists.
type roamStyle struct {
	label string
	icon  *ui.SVG
	glyph string // in place of an icon, for a language
	words string
}

var roamGroups = []struct {
	title  string
	styles []roamStyle
}{
	// Languages lead: they are the one group that pairs with every other,
	// so the style chosen below can be read against them at once.
	{"语种", []roamStyle{
		{label: "华语", glyph: "中", words: "华语"},
		{label: "欧美", glyph: "En", words: "欧美"},
		{label: "粤语", glyph: "粤", words: "粤语"},
		{label: "日语", glyph: "あ", words: "日语"},
		{label: "K-Pop", glyph: "한", words: "K-Pop"},
		{label: "法语", glyph: "Fr", words: "法语"},
		{label: "全球", icon: iconGlobe, words: "世界音乐"},
	}},
	{"心情", []roamStyle{
		{label: "伤感", icon: iconHeartCrack, words: "伤感"},
		{label: "放松", icon: iconFlower, words: "放松"},
		{label: "欢快", icon: iconParty, words: "欢快"},
		{label: "抒情", icon: iconFeather2, words: "抒情"},
		{label: "治愈", icon: iconSprout, words: "治愈"},
		{label: "情歌", icon: iconHeart2, words: "情歌"},
		{label: "励志", icon: iconFlame, words: "励志"},
		{label: "经典", icon: iconTape, words: "经典 老歌"},
	}},
	{"场景", []roamStyle{
		{label: "运动", icon: iconDumbbell, words: "运动 跑步"},
		{label: "助眠", icon: iconMoonStar, words: "助眠"},
		{label: "专注", icon: iconBook, words: "专注 学习"},
		{label: "下雨天", icon: iconUmbrella, words: "下雨天"},
		{label: "打游戏", icon: iconGamepad, words: "打游戏"},
		{label: "出行", icon: iconCar, words: "出行 自驾"},
		{label: "洗澡", icon: iconBath, words: "洗澡"},
		{label: "咖啡馆", icon: iconCoffee, words: "咖啡馆"},
		{label: "舞蹈", icon: iconSteps, words: "舞蹈"},
		{label: "影视", icon: iconClapper, words: "影视金曲"},
	}},
	{"曲风", []roamStyle{
		{label: "R&B", icon: iconMicVocal, words: "R&B"},
		{label: "说唱", icon: iconMic, words: "说唱"},
		{label: "电音", icon: iconWave, words: "电音"},
		{label: "摇滚", icon: iconZap, words: "摇滚"},
		{label: "民谣", icon: iconGuitar, words: "民谣"},
		{label: "国风", icon: iconFan, words: "国风"},
		{label: "轻音乐", icon: iconLeaf, words: "轻音乐"},
		{label: "古典", icon: iconPiano, words: "古典"},
		{label: "爵士", icon: iconWine, words: "爵士"},
		{label: "蓝调", icon: iconCloudMoon, words: "蓝调"},
		{label: "慢摇 DJ", icon: iconDisc, words: "慢摇 DJ"},
		{label: "二次元", icon: iconWand, words: "二次元"},
		{label: "拉丁", icon: iconSun, words: "拉丁"},
		{label: "放克", icon: iconRadioOld, words: "放克 Funk"},
		{label: "乡村乐", icon: iconWheat, words: "乡村"},
		{label: "音乐剧", icon: iconDrama, words: "音乐剧"},
		{label: "宝藏原创", icon: iconGem, words: "宝藏 原创"},
	}},
}

const stylePrefix = "style:"

func styleRadio(label string) string { return stylePrefix + label }

// isRoam reports whether a radio plays a style, not the FM.
func isRoam(name string) bool { return strings.HasPrefix(name, stylePrefix) }

// styleWords returns what finds the playlists of the style of a radio.
func styleWords(label string) string {
	if parts := strings.Split(label, "|"); len(parts) > 1 {
		words := make([]string, len(parts))
		for i, part := range parts {
			words[i] = styleWords(part)
		}
		return strings.Join(words, " ")
	}
	for _, g := range roamGroups {
		for _, s := range g.styles {
			if s.label == label {
				return s.words
			}
		}
	}
	return label
}

// How a style goes on: from one playlist at a time, a few songs at a time, of
// the best played it finds.
const (
	roamPlaylists = 12
	roamBatch     = 8
)

// roamer deals the songs of styles out of their playlists, on the goroutines
// radios load on.
type roamer struct {
	mu      sync.Mutex
	pools   map[string][]netease.Playlist // the playlists found for a style
	dealing map[string][]netease.Track    // what is left of the one it is on
	played  map[int64]bool                // songs dealt, none twice
}

// next returns a few songs of a style that haven't played.
func (r *roamer) next(label string) ([]netease.Track, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pools == nil {
		r.pools, r.dealing, r.played = map[string][]netease.Playlist{}, map[string][]netease.Track{}, map[int64]bool{}
	}
	for tries := 0; tries < 4; tries++ {
		var out []netease.Track
		left := r.dealing[label]
		for len(left) > 0 && len(out) < roamBatch {
			tr := left[0]
			left = left[1:]
			if !r.played[tr.ID] && tr.Playable() {
				r.played[tr.ID] = true
				out = append(out, tr)
			}
		}
		r.dealing[label] = left
		if len(out) > 0 {
			return out, nil
		}
		if err := r.refill(label); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("style: no songs left")
}

// refill opens another of the style's playlists, shuffled: more often one of
// the best played.
func (r *roamer) refill(label string) error {
	pool := r.pools[label]
	if len(pool) == 0 {
		res, err := netease.SearchPage(styleWords(label), netease.SearchPlaylists, roamPlaylists, 0)
		if err != nil {
			return err
		}
		if pool = res.Playlists; len(pool) == 0 {
			return errors.New("style: no playlists")
		}
	}
	at := min(rand.IntN(len(pool)), rand.IntN(len(pool)))
	pl := pool[at]
	r.pools[label] = append(pool[:at:at], pool[at+1:]...)
	detail, err := netease.PlaylistDetail(pl.ID)
	if err != nil {
		return err
	}
	songs := append([]netease.Track(nil), detail.Tracks...)
	rand.Shuffle(len(songs), func(i, j int) { songs[i], songs[j] = songs[j], songs[i] })
	r.dealing[label] = songs
	return nil
}

// roamChoice returns the selected style and language, including one loading.
func (a *app) roamChoice() (style, language string) {
	name := a.radioStarting
	if name == "" {
		name = a.pl.Radio()
	}
	if !isRoam(name) {
		return
	}
	for _, label := range strings.Split(strings.TrimPrefix(name, stylePrefix), "|") {
		if isRoamLanguage(label) {
			language = label
		} else {
			style = label
		}
	}
	return
}

func isRoamLanguage(label string) bool {
	for _, g := range roamGroups {
		if g.title != "语种" {
			continue
		}
		for _, s := range g.styles {
			if s.label == label {
				return true
			}
		}
	}
	return false
}

// playStyle toggles one style or one language; the two combine in a search.
func (a *app) playStyle(label string) {
	style, language := a.roamChoice()
	chosen := &style
	if isRoamLanguage(label) {
		chosen = &language
	}
	if *chosen == label {
		*chosen = ""
	} else {
		*chosen = label
	}
	label = strings.Trim(strings.Join([]string{style, language}, "|"), "|")
	name := styleRadio(label)
	if label == "" {
		a.exitRadio()
		return
	}
	a.startRadio(name, "这个风格暂时无法播放，请稍后再试")
}

// How the styles lay out: every card shares one grid of columns, so a column
// runs straight down from one card to the next.
const (
	roamPad     = 14 // inside a card
	roamGap     = 10 // between tiles
	roamTileMin = 76 // the narrowest a tile gets
	roamWide    = 9  // columns when mood and scene sit side by side
)

// roamSection lists the styles, each a tile that starts it: the languages as
// a row of capsules on top, as they go with any style, and the styles below
// in cards. Wide enough, mood and scene share a card of nine columns, four
// and five, and the genres fill two rows of the same nine under it.
func (a *app) roamSection(c *ui.Context) {
	p := a.pal
	style, language := a.roamChoice()
	ui.Row(c).AlignItems(ui.End).Gap(16).Children(func() {
		ui.Box(c).Grow(1).Children(func() { a.sectionLarge(c, "风格漫游") })
		hint := "选一个风格开始，无需登录"
		if picked := strings.Trim(strings.Join([]string{language, style}, " · "), " · "); picked != "" {
			hint = "当前：" + picked
		}
		ui.Text(c, hint).FontSize(13).TextColor(p.muted).SingleLine().Margin(0, 0, 20, 0)
	})
	var languages, moods, scenes, genres *[]roamStyle
	for i := range roamGroups {
		switch roamGroups[i].title {
		case "语种":
			languages = &roamGroups[i].styles
		case "心情":
			moods = &roamGroups[i].styles
		case "场景":
			scenes = &roamGroups[i].styles
		case "曲风":
			genres = &roamGroups[i].styles
		}
	}
	ui.Row(c).Wrap().GapX(8).GapY(8).AlignItems(ui.Center).Margin(0, 0, 16, 0).Children(func() {
		ui.Text(c, "语种").FontSize(13).FontWeight(600).TextColor(p.muted).Margin(0, 6, 0, 2)
		for i := range *languages {
			a.languagePill(c.Key((*languages)[i].label), &(*languages)[i])
		}
	})
	w, _ := c.Size()
	inner := w - a.sideWidth(w) - 2*pagePad
	cols := int((inner - 2*roamPad + roamGap) / (roamTileMin + roamGap))
	ui.Column(c).Gap(12).Children(func() {
		if cols >= roamWide {
			a.roamPairCard(c.Key("mood-scene"), inner, "心情", *moods, "场景", *scenes)
			a.roamCard(c.Key("genre"), "曲风", *genres, roamWide)
			return
		}
		cols = min(max(cols, 3), 8)
		a.roamCard(c.Key("mood"), "心情", *moods, cols)
		a.roamCard(c.Key("scene"), "场景", *scenes, cols)
		a.roamCard(c.Key("genre"), "曲风", *genres, cols)
	})
}

// roamCardBox is the card a group of styles sits in. Light, over the cover's
// colors, the settings' milky white would cover them: a thin pane of glass
// lets them through. The settings' cards keep theirs, over their own window.
func (a *app) roamCardBox(c *ui.Context) ui.Element {
	fill, edge := a.cardColors()
	if !a.pal.dark && a.settings.mainGlass() {
		fill, edge = white.Alpha(0.25), white.Alpha(0.5)
	}
	return ui.Column(c).Padding(roamPad-2, roamPad, roamPad, roamPad).Radius(20).Background(fill).Border(0.5, edge)
}

// roamTitle heads a group: its name and how many styles it holds.
func (a *app) roamTitle(c *ui.Context, title string, n int) ui.Element {
	p := a.pal
	return ui.Row(c).Gap(6).AlignItems(ui.Baseline).Padding(0, 2).Children(func() {
		ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(p.text)
		ui.Text(c, strconv.Itoa(n)).FontSize(12.5).TextColor(p.faint)
	})
}

// roamCard is one group in a card, in cols columns. The genres end on a tile
// that roams a style picked at random, which fills the last cell of nine.
func (a *app) roamCard(c *ui.Context, title string, styles []roamStyle, cols int) {
	a.roamCardBox(c).Gap(10).Children(func() {
		a.roamTitle(c, title, len(styles))
		ui.Grid(c.Key("grid")).Columns(cols).GapX(roamGap).GapY(roamGap).Children(func() {
			for i := range styles {
				a.styleTile(c.Key(styles[i].label), &styles[i])
			}
			if title == "曲风" {
				a.shuffleTile(c.Key("shuffle"))
			}
		})
	})
}

// roamPairCard puts two groups side by side in one card of nine columns, four
// for the first and five for the second, with a hairline between. inner is
// the width the card spans, which places the hairline in the gap.
func (a *app) roamPairCard(c *ui.Context, inner float32, leftTitle string, left []roamStyle, rightTitle string, right []roamStyle) {
	const split = 4
	tile := (inner - 2*roamPad - (roamWide-1)*roamGap) / roamWide
	a.roamCardBox(c).Children(func() {
		ui.Grid(c.Key("grid")).Columns(roamWide).GapX(roamGap).GapY(roamGap).Children(func() {
			a.roamTitle(c.Key("l"), leftTitle, len(left)).ColumnStart(1).RowStart(1).ColumnSpan(split)
			a.roamTitle(c.Key("r"), rightTitle, len(right)).ColumnStart(split + 1).RowStart(1).ColumnSpan(roamWide - split)
			for i := range left {
				a.styleTile(c.Key(left[i].label), &left[i]).ColumnStart(1 + i%split).RowStart(2 + i/split)
			}
			for i := range right {
				a.styleTile(c.Key(right[i].label), &right[i]).ColumnStart(split + 1 + i%(roamWide-split)).RowStart(2 + i/(roamWide-split))
			}
		})
		x := roamPad + split*(tile+roamGap) - roamGap/2 - 0.5
		ui.Box(c.Key("rule")).Absolute().Left(x).Top(roamPad + 28).Bottom(roamPad).Width(1).Background(a.pal.hairline)
	})
}

// languagePill is a language: a capsule that lit stays lit under any style
// chosen below, and goes out on a second click.
func (a *app) languagePill(c *ui.Context, s *roamStyle) {
	p := a.pal
	_, language := a.roamChoice()
	on := language == s.label
	pill := ui.ButtonBase(c).Row().Height(34).Padding(0, 14).Gap(7).Radius(999).AlignItems(ui.Center).FocusRing(false).Label(s.label)
	bg, edge, ink := p.hover, p.hairline, p.text
	switch {
	case on:
		bg, edge, ink = p.accent.Alpha(0.16), p.accent, p.accent
	case pill.Hovered():
		bg = p.press
	}
	pill.Background(pill.AnimateColor("bg", bg, motionHover))
	pill.Border(1, pill.AnimateColor("edge", edge, motionHover))
	if pill.Pressed() {
		pill.AnimatePose(ui.Pose{Scale: 0.96}, ui.Snappy)
	} else {
		pill.AnimatePose(ui.Pose{}, ui.Snappy)
	}
	pill.Children(func() {
		if s.icon != nil {
			ui.Icon(c, s.icon).Size(16, 16).TextColor(ink)
		} else {
			ui.Text(c, s.glyph).FontSize(15).FontWeight(800).TextColor(ink)
		}
		label := s.label
		if on && isRoam(a.radioStarting) {
			label = "正在调频…"
		}
		ui.Text(c, label).FontSize(13.5).FontWeight(500).TextColor(p.text).SingleLine()
	})
	if pill.Clicked() {
		a.playStyle(s.label)
	}
}

// shuffleTile roams a style picked at random, other than the one playing,
// keeping the language chosen.
func (a *app) shuffleTile(c *ui.Context) {
	p := a.pal
	tile := ui.ButtonBase(c).Column().Height(72).Gap(6).Radius(14).AlignItems(ui.Center).Justify(ui.Center).FocusRing(false).Label("随机漫游")
	bg, edge := ui.Transparent, p.faint
	if tile.Hovered() {
		bg, edge = p.hover, p.muted
	}
	tile.Background(tile.AnimateColor("bg", bg, motionHover))
	tile.Border(1, tile.AnimateColor("edge", edge, motionHover))
	if tile.Pressed() {
		tile.AnimatePose(ui.Pose{Scale: 0.97}, ui.Snappy)
	} else {
		tile.AnimatePose(ui.Pose{}, ui.Snappy)
	}
	tile.Children(func() {
		ui.Icon(c, iconShuffle).Size(22, 22).TextColor(p.muted)
		ui.Text(c, "随机漫游").FontSize(13.5).FontWeight(500).TextColor(p.muted).SingleLine()
	})
	if tile.Clicked() {
		style, _ := a.roamChoice()
		var pick []string
		for _, g := range roamGroups {
			if g.title == "语种" {
				continue
			}
			for _, s := range g.styles {
				if s.label != style {
					pick = append(pick, s.label)
				}
			}
		}
		if len(pick) > 0 {
			a.playStyle(pick[rand.IntN(len(pick))])
		}
	}
}

// styleTile is a style: lit while it plays, and framed in the accent under
// the pointer. The colors follow the cover playing when the settings say so,
// through the palette's accent.
func (a *app) styleTile(c *ui.Context, s *roamStyle) ui.Element {
	p := a.pal
	style, language := a.roamChoice()
	on := style == s.label || language == s.label
	starting := on && isRoam(a.radioStarting)
	tile := ui.ButtonBase(c).Column().Height(72).Gap(6).Radius(14).AlignItems(ui.Center).Justify(ui.Center).FocusRing(false).Label(s.label).
		Border(1.5, ui.Transparent)
	bg, edge, ink := p.hover, ui.Transparent, p.text
	switch {
	case on:
		bg, edge, ink = p.accent.Alpha(0.16), p.accent, p.accent
	case tile.Hovered():
		bg = p.press
	}
	tile.Background(tile.AnimateColor("bg", bg, motionHover))
	tile.BorderColor(tile.AnimateColor("edge", edge, motionHover))
	lift := ui.Pose{}
	if tile.Hovered() {
		lift.Y = -2
	}
	if tile.Pressed() {
		lift = ui.Pose{Scale: 0.97}
	}
	tile.AnimatePose(lift, ui.Snappy)
	tile.Children(func() {
		// Under the pointer the icon lights up: a blurred copy of it in the
		// accent shows through behind the sharp one. The copy is a normal
		// child and the sharp one absolute, as absolute children paint last.
		glow := tile.Hovered()
		ui.Box(c).Size(22, 22).Children(func() {
			if glow {
				ui.Box(c).Size(22, 22).Blur(6).Opacity(0.8).Center().Children(func() {
					if s.icon != nil {
						ui.Icon(c, s.icon).Size(22, 22).TextColor(p.accent)
					} else {
						ui.Text(c, s.glyph).FontSize(21).FontWeight(800).TextColor(p.accent).LineHeight(1.15)
					}
				})
			}
			ui.Box(c).Absolute().Top(0).Left(0).Size(22, 22).Center().Children(func() {
				if s.icon != nil {
					ui.Icon(c, s.icon).Size(22, 22).TextColor(ink)
				} else {
					ui.Text(c, s.glyph).FontSize(21).FontWeight(800).TextColor(ink).LineHeight(1.15)
				}
			})
		})
		label := s.label
		if starting {
			label = "正在调频…"
		}
		// The label stays in the text color: the icon alone takes the
		// accent, so a row of chosen tiles still reads as words.
		ui.Text(c, label).FontSize(13.5).FontWeight(500).TextColor(p.text).SingleLine()
		if on {
			ui.Icon(c, iconCheck).Absolute().Top(8).Right(9).Size(11, 11).TextColor(p.accent)
		}
	})
	if tile.Clicked() {
		a.playStyle(s.label)
	}
	return tile
}
