package app

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// What the sidebar lists is the listener's to choose: which of its entries
// and its groups of playlists show, in what order, and which groups start
// folded (viewers asked to hide 播客 or 云盘, and to fold 我的歌单, 1.3.x).
// 为你推荐 always shows, where the app starts. They are changed from the
// settings (首页与侧栏 → 侧边栏), or from the sidebar's own menu: a right
// press on an entry, a group's title, or nothing.

// A sideItem is an entry of the sidebar, or a group of playlists in it.
type sideItem struct {
	id, label string
	icon      *ui.SVG
	group     bool // a group of playlists, under a title that folds it
	fixed     bool // always shown
}

// sideItems are what the sidebar can list, in the order it lists them
// unless the listener chose another.
var sideItems = []sideItem{
	{id: "home", label: "为你推荐", icon: iconHome, fixed: true},
	{id: "fm", label: "私人 FM", icon: iconRadio},
	{id: "charts", label: "排行榜", icon: iconCharts},
	{id: "podcasts", label: "播客", icon: iconPodcast},
	{id: "downloads", label: "下载歌曲", icon: iconDownload},
	{id: "local", label: "本地音乐", icon: iconLibrary},
	{id: "recent", label: "最近播放", icon: iconHistory},
	{id: "cloud", label: "我的音乐云盘", icon: iconCloud},
	{id: "mine", label: "我的歌单", icon: iconNote, group: true},
	{id: "saved", label: "收藏的歌单", icon: iconNote, group: true},
	{id: "albums", label: "我的专辑", icon: iconAlbum, group: true},
}

// sideItemOf returns the item of id.
func sideItemOf(id string) (sideItem, bool) {
	for _, it := range sideItems {
		if it.id == id {
			return it, true
		}
	}
	return sideItem{}, false
}

// sideOrder returns the ids of all the sidebar's items in the order the
// listener chose, those it never knew of (added since) where they come
// by default, after the one before them.
func (s settings) sideOrder() []string {
	var out []string
	for _, id := range s.SideOrder {
		if _, ok := sideItemOf(id); ok && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	for i, it := range sideItems {
		if slices.Contains(out, it.id) {
			continue
		}
		at := 0
		if i > 0 {
			at = slices.Index(out, sideItems[i-1].id) + 1
		}
		out = slices.Insert(out, at, it.id)
	}
	return out
}

// sideOn reports whether the sidebar shows id.
func (s settings) sideOn(id string) bool {
	if it, ok := sideItemOf(id); ok && it.fixed {
		return true
	}
	return !slices.Contains(s.SideHidden, id)
}

// sideFolded reports whether the group id starts folded.
func (s settings) sideFolded(id string) bool { return slices.Contains(s.SideFolded, id) }

// sideFold folds the groups as the settings start them.
func (a *app) sideFold() {
	s := &a.settings
	a.mineCollapsed, a.savedCollapsed, a.albumsCollapsed = s.sideFolded("mine"), s.sideFolded("saved"), s.sideFolded("albums")
}

// setSideShown shows id in the sidebar, or hides it.
func (a *app) setSideShown(id string, on bool) {
	s := &a.settings
	s.SideHidden = slices.DeleteFunc(slices.Clone(s.SideHidden), func(x string) bool { return x == id })
	if !on {
		if it, ok := sideItemOf(id); ok && it.fixed {
			return
		}
		s.SideHidden = append(s.SideHidden, id)
	}
	a.changed()
}

// setSideFolded has the group id start folded, or open; it folds or opens
// now too.
func (a *app) setSideFolded(id string, folded bool) {
	s := &a.settings
	s.SideFolded = slices.DeleteFunc(slices.Clone(s.SideFolded), func(x string) bool { return x == id })
	if folded {
		s.SideFolded = append(s.SideFolded, id)
	}
	switch id {
	case "mine":
		a.mineCollapsed = folded
	case "saved":
		a.savedCollapsed = folded
	case "albums":
		a.albumsCollapsed = folded
	}
	a.changed()
}

// sideReorder keeps shown, the ids of the items shown, as their order,
// those hidden where they were.
func (a *app) sideReorder(shown []string) {
	order := a.settings.sideOrder()
	k := 0
	for i, id := range order {
		if a.settings.sideOn(id) && k < len(shown) {
			order[i] = shown[k]
			k++
		}
	}
	a.settings.SideOrder = order
	a.changed()
}

// sideList lists the sidebar's entries and groups as the settings have
// them; the search results, while there are some, after the entries before
// the first group.
func (a *app) sideList(c *ui.Context) {
	searched := false
	search := func() {
		if !searched && a.searched.query != "" {
			a.navItem(c, iconSearch, "搜索结果", a.searched)
		}
		searched = true
	}
	for _, id := range a.settings.sideOrder() {
		if !a.settings.sideOn(id) {
			continue
		}
		it, _ := sideItemOf(id)
		if it.group {
			search()
		}
		switch id {
		case "home":
			a.navItem(c, it.icon, it.label, route{page: pageHome})
		case "fm":
			a.navItem(c, it.icon, it.label, route{page: pageFM})
		case "charts":
			a.navItem(c, it.icon, it.label, route{page: pageCharts})
		case "podcasts":
			a.navItem(c, it.icon, it.label, route{page: pagePodcasts})
		case "downloads":
			a.navItem(c, it.icon, it.label, route{page: pageDownloads})
		case "local":
			a.navItem(c, it.icon, it.label, route{page: pageLocal})
		case "recent":
			a.navItem(c, it.icon, it.label, route{page: pageRecent})
		case "cloud":
			if a.profile != nil {
				a.navItem(c, it.icon, it.label, route{page: pageCloud})
			}
		case "mine":
			a.myPlaylists(c, it.label, a.mine, true, &a.mineCollapsed)
		case "saved":
			a.myPlaylists(c, it.label, a.saved, false, &a.savedCollapsed)
		case "albums":
			a.myAlbumsSection(c)
		}
	}
	search()
}

// sideMenu is the menu a right press on the sidebar opens: over the entry
// or group label, if any, to hide it, or fold it by default; and to edit
// the sidebar.
func (a *app) sideMenu(label string) [][]menuAction {
	edit := menuAction{label: "编辑侧栏…", icon: iconSettings, do: a.editSidebar}
	var it sideItem
	for _, x := range sideItems {
		if x.label == label {
			it = x
		}
	}
	if it.id == "" {
		return [][]menuAction{{edit}}
	}
	var mine []menuAction
	if !it.fixed {
		id := it.id
		mine = append(mine, menuAction{label: "从侧栏隐藏「" + it.label + "」", icon: iconEyeOff, do: func() {
			a.setSideShown(id, false)
			a.toast = "已隐藏「" + it.label + "」，可在侧栏右键「编辑侧栏」里加回来"
		}})
	}
	if it.group {
		id, folded := it.id, a.settings.sideFolded(it.id)
		label := "默认收起"
		if folded {
			label = "默认展开"
		}
		mine = append(mine, menuAction{label: label, icon: iconDown, do: func() { a.setSideFolded(id, !folded) }})
	}
	if len(mine) == 0 {
		return [][]menuAction{{edit}}
	}
	return [][]menuAction{mine, {edit}}
}

// editSidebar opens the settings where the sidebar's contents are, their
// list open.
func (a *app) editSidebar() {
	a.sideDrawer = true
	a.openSettingsAt(settingsHome)
}

// sideSummary names what the sidebar shows, and how many are hidden.
func (a *app) sideSummary() string {
	var hidden []string
	for _, id := range a.settings.sideOrder() {
		if !a.settings.sideOn(id) {
			it, _ := sideItemOf(id)
			hidden = append(hidden, it.label)
		}
	}
	if len(hidden) == 0 {
		return "全部显示 · 可以隐藏、调整顺序，或让分组默认收起"
	}
	return "已隐藏：" + strings.Join(hidden, "、")
}

// sideSettings is the sidebar's row in the settings, whose button opens
// the list of what it shows under it, as the calendar's does.
func (a *app) sideSettings(c *ui.Context, k *settingsKit) {
	open := a.sideDrawer
	label := "管理"
	if open {
		label = "收起"
	}
	k.opens(c, "side-manage", label+"侧栏内容", "侧栏内容", a.sideSummary(), open, func() { a.sideDrawer = !open })
	if k.probe {
		return
	}
	drawer(c, "side-drawer", open, func() {
		ui.Box(c).Padding(0, 20, 16, 20).Children(func() { a.sideManager(c, k.ink) })
	})
}

// sideSort is a row of the sidebar's list being dragged to a new place.
type sideSort struct {
	id       string
	dragging bool
	dy       float32
}

// sideManager is what the sidebar shows: the items shown, in their order,
// each dragged by its row to a new place, the others making room, each
// with a button that hides it (为你推荐 none), the groups a switch that
// folds them at the start; and those hidden, each with a button that shows
// it again, after the rest. As the calendar's (deckManager).
func (a *app) sideManager(c *ui.Context, ink paneInk) {
	s := &a.settings
	var shown, hidden []string
	for _, id := range s.sideOrder() {
		if s.sideOn(id) {
			shown = append(shown, id)
		} else {
			hidden = append(hidden, id)
		}
	}
	caption := func(words string) {
		ui.Text(c, words).FontSize(12).FontWeight(600).TextColor(ink.muted).Margin(10, 0, 4, 4)
	}
	words := func(it sideItem) {
		ui.Icon(c, it.icon).Size(16, 16).TextColor(ink.muted).Shrink(0)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, it.label).FontSize(13.5).FontWeight(600).TextColor(ink.text).SingleLine()
			desc := "入口"
			switch {
			case it.fixed:
				desc = "入口 · 始终显示"
			case it.group:
				desc = "歌单分组"
			}
			ui.Text(c, desc).FontSize(12).TextColor(ink.muted).SingleLine()
		})
	}
	const rowH, rowGap = float32(52), float32(6)
	const step = rowH + rowGap
	d := &a.sideSort
	if d.dragging && !slices.Contains(shown, d.id) {
		d.dragging = false
	}
	from, to := -1, -1
	if d.dragging {
		from = slices.Index(shown, d.id)
		to = max(0, min(len(shown)-1, from+int(math.Round(float64(d.dy/step)))))
	}
	slot := func(i int) int {
		switch {
		case from < 0 || i == from:
			return i
		case from < i && i <= to:
			return i - 1
		case to <= i && i < from:
			return i + 1
		}
		return i
	}
	glide := ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond, Bounce: 0.15}}
	row := func(i int, id string) {
		it, _ := sideItemOf(id)
		r := ui.Row(c.Key(id)).Absolute().Top(0).Left(0).Right(0).Height(rowH).Gap(10).Padding(6, 8, 6, 8).Radius(14).
			Background(ink.hover).AlignItems(ui.Center).Label("拖动排序 " + it.label).Cursor(ui.CursorGrab)
		dragged := d.dragging && d.id == id
		if _, dy, ok := r.Dragged(); ok {
			if !d.dragging || d.id != id {
				*d = sideSort{id: id, dragging: true}
			}
			d.dy += dy
			dragged = true
		}
		if _, _, ok := r.DragEnded(); ok && d.dragging && d.id == id {
			if from >= 0 && to != from {
				moved := slices.Delete(slices.Clone(shown), from, from+1)
				moved = slices.Insert(moved, to, id)
				a.sideReorder(moved)
			}
			d.dragging, dragged = false, false
		}
		pose := ui.Pose{Y: float32(slot(i)) * step}
		if dragged {
			lo, hi := -float32(i)*step-6, float32(len(shown)-1-i)*step+6
			pose = ui.Pose{Y: float32(i)*step + max(lo, min(hi, d.dy)), Scale: 1.02}
			lifted := ui.Hex("#ffffff")
			if a.pal.dark {
				lifted = ui.Hex("#323238")
			}
			r.Cursor(ui.CursorGrabbing).Shadow(0, 10, 24, -8, ui.RGBA(0, 0, 0, 0.35)).Background(lifted)
			r.AnimatePose(pose, ui.Animation{Spring: ui.SpringOptions{Duration: 70 * time.Millisecond}})
		} else {
			r.AnimatePose(pose, glide)
		}
		r.Children(func() {
			ui.Box(c).Size(14, 20).Shrink(0).Draw(func(p *ui.Painter, b ui.Rect) {
				for x := range 2 {
					for y := range 3 {
						p.Fill(ui.Rect{X: b.X + 3 + float32(x)*6, Y: b.Y + 3 + float32(y)*6, W: 2.5, H: 2.5}, ink.faint, 1.25)
					}
				}
			})
			words(it)
			if it.group {
				folded := s.sideFolded(id)
				ui.Text(c, "默认收起").FontSize(12).TextColor(ink.muted).Shrink(0)
				if settingSwitch(c.Key("fold"), &folded).Label("默认收起 " + it.label).Changed() {
					a.setSideFolded(id, folded)
				}
			}
			if !it.fixed && iconButton(c.Key("hide"), iconEyeOff, 16, ink.muted, "隐藏 "+it.label).Clicked() {
				a.setSideShown(id, false)
			}
		})
	}
	ui.Column(c).Gap(6).Children(func() {
		caption("显示中 · 拖动调整顺序，从上到下就是侧栏的顺序")
		ui.Box(c.Key("shown")).Height(float32(len(shown))*step - rowGap).Children(func() {
			for i, id := range shown {
				if !(d.dragging && d.id == id) {
					row(i, id)
				}
			}
			if from >= 0 {
				row(from, d.id)
			}
		})
		if len(hidden) > 0 {
			caption("已隐藏")
			for _, id := range hidden {
				it, _ := sideItemOf(id)
				ui.Row(c.Key(id)).Height(rowH).Gap(10).Padding(6, 8, 6, 8).Radius(14).Background(ink.hover).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(14, 20).Shrink(0)
					words(it)
					if settingButton(c.Key("show"), ink, "显示", iconPlus, false, false).Label("显示 " + it.label).Clicked() {
						a.setSideShown(id, true)
					}
				})
			}
		}
	})
}

var iconEyeOff = strokeIcon(`<path d="M10.7 5.1A10 10 0 0 1 12 5c5 0 9 4.5 10 7-.4 1-1.2 2.3-2.3 3.5"/><path d="M6.6 6.6C4.4 8 2.8 10.1 2 12c1 2.5 5 7 10 7 1.9 0 3.6-.6 5.1-1.5"/><path d="M9.9 9.9a3 3 0 0 0 4.2 4.2"/><path d="M3 3l18 18"/>`)
