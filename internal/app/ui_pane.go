package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// Panes are what floats over the app: menus, popovers, dialogs, toasts and
// the settings. All take their look from pane, so that they are of one
// material, dark glass or, with settings.GlassPanels off, plain.

// A paneInk holds the colors of what is on a pane.
type paneInk struct {
	text, muted, faint ui.Color
	hover, press, line ui.Color
}

// inkOn returns the colors for a pane, white where it is dark. onDark is for
// a pane over Now Playing, which is dark whatever the theme.
func (a *app) inkOn(onDark bool) paneInk {
	if onDark || a.pal.dark {
		return paneInk{white, white.Alpha(0.6), white.Alpha(0.42), white.Alpha(0.1), white.Alpha(0.16), white.Alpha(0.1)}
	}
	ink := ui.Hex("#1d1d1f")
	return paneInk{ink, ink.Alpha(0.62), ink.Alpha(0.42), ink.Alpha(0.07), ink.Alpha(0.12), ink.Alpha(0.1)}
}

// pane gives e the look of a pane, rounded by radius: a menu's, a
// popover's or a toast's (sceneMenu). Glass lightens what shows through in
// a light theme: a dark pane there takes more of its tint.
func (a *app) pane(e ui.Element, radius float32, onDark bool) ui.Element {
	e.Shadow(0, 18, 44, -12, ui.RGBA(0, 0, 0, 0.45))
	return a.paneSurfaceAt(e, radius, onDark, 0, sceneMenu)
}

// dialogPane is pane for a dialog or the settings (sceneDialog), which may
// be glazed apart from the menus.
func (a *app) dialogPane(e ui.Element, radius float32, onDark bool) ui.Element {
	e.Shadow(0, 18, 44, -12, ui.RGBA(0, 0, 0, 0.45))
	return a.paneSurfaceAt(e, radius, onDark, 0, sceneDialog)
}

// paneGrowing is pane for a pane on its way to its size, whose shorter side
// there is basis: its glass is the one of that size all the way, rather
// than changing, blurring more and more, with each size on the way.
func (a *app) paneGrowing(e ui.Element, radius float32, onDark bool, basis float32) ui.Element {
	e.Shadow(0, 18, 44, -12, ui.RGBA(0, 0, 0, 0.45))
	return a.paneSurfaceAt(e, radius, onDark, basis, sceneMenu)
}

// paneSurface is pane without its shadow, for a pane that casts its own.
func (a *app) paneSurface(e ui.Element, radius float32, onDark bool) ui.Element {
	return a.paneSurfaceAt(e, radius, onDark, 0, sceneMenu)
}

// paneSurfaceAt is paneSurface in the material of a pane whose shorter
// side is basis, if not 0, whatever its size now, glazed as the listener
// chose for its scene: liquid glass, by default, or frosted.
func (a *app) paneSurfaceAt(e ui.Element, radius float32, onDark bool, basis float32, scene glassScene) ui.Element {
	e.Radius(radius)
	dark := onDark || a.pal.dark
	switch {
	case !a.settings.glassIn(scene):
		return a.plainPane(e, onDark)
	case !a.settings.frosted(scene):
		return e.Material(a.paneGlass(dark, basis, scene))
	}
	return a.frostPane(e, onDark, basis)
}

// frostPane glazes e in frosted glass, the glass plugin's, tinted for the
// words on it to read over anything.
func (a *app) frostPane(e ui.Element, onDark bool, basis float32) ui.Element {
	switch {
	case a.pal.dark:
		return e.Material(glass.Glass{Tint: ui.RGBA(14, 14, 18, 0.62), Basis: basis}).Border(0.5, white.Alpha(0.14))
	case onDark:
		return e.Material(glass.Glass{Tint: ui.RGBA(14, 14, 18, 0.86), Basis: basis}).Border(0.5, white.Alpha(0.14))
	}
	return e.Material(glass.Glass{Tint: ui.RGBA(250, 250, 252, 0.58), Basis: basis}).Border(0.5, ui.RGBA(0, 0, 0, 0.1))
}

// A menuAction is a row of a menu: what it does, or more rows it opens.
type menuAction struct {
	label string // what it says, in full
	// title and note say it in two parts where there is room for them: the
	// name of what it opens, and what kind of thing that is.
	title, note string
	icon        *ui.SVG
	checked     bool
	tag         string // the membership it needs, marked as NetEase marks it: "VIP" or "SVIP"
	// locked, if set, says why it cannot be chosen: it shows dimmed, and a
	// press says so, keeping the menu open.
	locked string
	do     func()
	sub    [][]menuAction
}

// paneMenu is the menu open, one at a time, in a pane of its own over all:
// behind a button, or where the pointer pressed.
type paneMenu struct {
	open     bool
	x, y     float32
	up       float32 // what it rises from, where there is no room below y
	onDark   bool
	groups   [][]menuAction
	head     func(c *ui.Context, ink paneInk)
	sub      *menuAction // what shows in place of groups, if anything
	anchor   ui.Handle   // the control it opens under, for a window that changes size
	anchored bool
	closedAt time.Time // when a press outside closed it
	query    string    // what a long list in it is searched for
	at       ui.Rect   // where it showed, which it goes from as it closes
	from     ui.Rect   // what it grows out of: its button, or a little of itself round the pointer
	fromR    float32   // the corner radius of from
	source   uint64    // the element pressed for it with the secondary button, which keeps its hover
	src      ui.Handle // that element, to close the menu once it moves, as its list scrolls
	srcAt    ui.Rect   // where it was laid out as the menu opened
	natural  float32   // how tall the page showing is, laid out, for the pane to ease to as pages change
	rise     bool      // it opened upward from up, chosen as it opened
	page     string    // the page showing: "" the menu, else the label of its sub
	pageAt   time.Time // when page came, for the page coming in to wait for the one going
}

// menuSearchFrom is how many rows a menu's list has for a search over it:
// the playlists to add a song to, say.
const menuSearchFrom = 9

// menuSearchHeight is how tall the search of a long list is.
const menuSearchHeight = 40

// listed is the list of m that shows: its groups, or those of its sub.
func (m *paneMenu) listed() [][]menuAction {
	if m.sub != nil {
		return m.sub.sub
	}
	return m.groups
}

// searchable reports whether m's list is long enough for a search.
func (m *paneMenu) searchable() bool {
	n := 0
	for _, g := range m.listed() {
		n += len(g)
	}
	return n >= menuSearchFrom
}

// matching keeps of groups the actions whose names have q, and the groups
// with any left.
func matching(groups [][]menuAction, q string) [][]menuAction {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return groups
	}
	var out [][]menuAction
	for _, g := range groups {
		var kept []menuAction
		for _, act := range g {
			name := act.title
			if name == "" {
				name = act.label
			}
			if strings.Contains(strings.ToLower(name+" "+act.note), q) {
				kept = append(kept, act)
			}
		}
		if len(kept) > 0 {
			out = append(out, kept)
		}
	}
	return out
}

// menuWidth is how wide a menu is; the heights are what its rows take, to
// place it before it is built.
const (
	menuWidth      = 268
	menuRowHeight  = 39
	menuLineHeight = 9
	menuHeadHeight = 66
	menuListHeight = 300
	menuTallest    = 520 // the most a menu's own groups take before they scroll
)

// menuRows is how tall groups are in a menu.
func menuRows(groups [][]menuAction) float32 {
	h := float32(0)
	for i, g := range groups {
		if i > 0 {
			h += menuLineHeight
		}
		h += float32(len(g)) * menuRowHeight
	}
	return h
}

// height returns how tall m shows in a window winH tall.
func (m *paneMenu) height(winH float32) float32 {
	search := float32(0)
	if m.searchable() {
		search = menuSearchHeight
	}
	if m.sub != nil {
		return 12 + menuRowHeight + menuLineHeight + search + min(menuRows(m.sub.sub), menuListHeight)
	}
	return m.chrome() + search + min(menuRows(m.groups), m.room(winH))
}

// chrome is how tall a menu is around its groups: its padding, and its head
// if it has one.
func (m *paneMenu) chrome() float32 {
	h := float32(12)
	if m.head != nil {
		h += menuHeadHeight
	}
	return h
}

// room is how tall the groups of m may be before they scroll: a long menu,
// such as the playlists to add a song to, keeps in the window.
func (m *paneMenu) room(winH float32) float32 {
	room := winH - 16 - m.chrome()
	if m.searchable() {
		room -= menuSearchHeight
	}
	return max(min(room, menuTallest), 3*menuRowHeight)
}

// openMenu opens a menu at x, y, rising from up where there is no room below.
func (a *app) openMenu(x, y, up float32, onDark bool, groups [][]menuAction, head func(c *ui.Context, ink paneInk)) {
	a.menu = paneMenu{open: true, x: x, y: y, up: up, onDark: onDark, groups: groups, head: head}
	a.redraw()
}

// menuBelow opens a menu under b, a button, unless the press on b just closed
// it: a click on a button of an open menu closes it, as a press outside it.
func (a *app) menuBelow(b ui.Element, onDark bool, groups [][]menuAction, head func(c *ui.Context, ink paneInk)) {
	if time.Since(a.menu.closedAt) < 300*time.Millisecond {
		return
	}
	r := b.Bounds()
	a.openMenu(r.X, r.Y+r.H+8, r.Y-8, onDark, groups, head)
	b.Bind(&a.menu.anchor)
	a.menu.anchored = true
}

// contextMenu opens a menu where the secondary button presses el.
func (a *app) contextMenu(el ui.Element, onDark bool, groups func() [][]menuAction, head func(c *ui.Context, ink paneInk)) {
	if a.menu.open && a.menu.source != 0 && a.menu.source == el.ID() {
		el.Bind(&a.menu.src) // each frame, to find it where it is now
	}
	if !el.RightClicked() {
		return
	}
	// A second right click closes the menu open, wherever it lands, rather
	// than open another (1.3.12, the user's ask): the press closed it as a
	// press outside, just now, or it is still open, its overlay built later.
	if a.menu.open || time.Since(a.menu.closedAt) < 300*time.Millisecond {
		a.menu.open, a.menu.closedAt = false, time.Now()
		return
	}
	// Asked only now: an element asking for the pointer is built each time
	// it moves over it, which over the rows of a list adds up.
	x, y, _ := el.PointerPosition()
	r := el.Bounds()
	a.openMenu(r.X+x, r.Y+y, r.Y+y, onDark, groups(), head)
	a.menu.source = el.ID()
	el.Bind(&a.menu.src)
}

// menuOverlay builds the menu open, if one is: grown out of the button that
// opened it, or out of the pointer, and going back into it as it closes.
func (a *app) menuOverlay(c *ui.Context) {
	m := &a.menu
	k := a.growK(c, "menu", m.open, growOpen)
	if !m.open && k <= growGone {
		m.source = 0
		return
	}
	w, h := c.Size()
	ink := a.inkOn(m.onDark)
	x, y := m.at.X, m.at.Y
	if m.open && m.source != 0 {
		// What it was opened on moved, or left: its list scrolled, under
		// the wheel. The menu goes, as a context menu does (1.3.12).
		// Laid out, not as posed: a row easing in as its page does moves
		// by its pose, which is no scroll.
		e := c.Resolve(m.src)
		// Gone is not moved: the bar's cover is another as the song
		// changes, its menu still for the song it opened on. A row
		// scrolled out of its list moved on its way out.
		if b := e.LayoutBounds(); e.Valid() && b.H > 0 {
			if m.srcAt.H == 0 {
				m.srcAt = b
			} else if abs32(b.Y-m.srcAt.Y) > 2 || abs32(b.X-m.srcAt.X) > 2 {
				m.open, m.closedAt = false, time.Now()
			}
		}
	}
	if m.open {
		var button ui.Rect
		if m.anchored {
			r := m.anchor.Bounds(c)
			if e := c.Resolve(m.anchor); e.Valid() {
				r = e.LayoutBounds()
			}
			if r.W <= 0 || r.H <= 0 {
				m.open, m.sub = false, nil
				return
			}
			m.x, m.y, m.up = r.X, r.Y+r.H+8, r.Y-8
			button = r
		}
		x, y = min(max(m.x, 8), w-menuWidth-8), m.y
		tall := m.height(h)
		if m.at.W == 0 {
			// Up or down is chosen once, as it opens: a page of it
			// taller or shorter keeps the side it opened to.
			m.rise = y+tall > h-8
		}
		if m.rise {
			y = max(m.up-tall, 8)
		}
		if g := a.growKeep("menu"); g.h > 0 && m.at.W > 0 {
			tall = g.h // as laid out, once it has been
		}
		m.at = ui.Rect{X: x, Y: y, W: menuWidth, H: tall}
		if button.W > 0 {
			m.from, m.fromR = button, min(min(button.W, button.H)/2, 14)
		} else {
			// Out of the pointer: a little of itself round it, as the
			// menus grew. Risen, it grows up out of where it rose from,
			// pushed down from the window's top or not.
			ox, oy := m.x, m.y
			if m.rise {
				oy = m.up
			}
			const s = 0.86
			m.from = ui.Rect{X: ox + (x-ox)*s, Y: oy + (y-oy)*s, W: menuWidth * s, H: tall * s}
			m.fromR = 20
		}
	}
	ui.Overlay(c, func() {
		// Its height eases to the page showing, once it is open: a list in
		// it is taller or shorter than the menu.
		fullH := float32(0)
		if m.natural > 0 {
			hv := ui.Box(c.Key("menu-height")).Absolute().Size(0, 0)
			if k < 1 || !m.open {
				hv.Jump("h", m.natural)
			}
			fullH = hv.AnimateTo("h", m.natural, motionPane)
			if m.open {
				// Placed by the height on its way, not the page's: risen
				// from its button, its top would jump to where a page
				// shorter or taller ends while its height still eased.
				if m.rise {
					y = max(m.up-fullH, 8)
				} else {
					y = max(min(m.y, h-8-fullH), 8)
				}
				m.at.Y, m.at.H = y, fullH
			}
		}
		spec := growSpec{key: "menu", k: k, open: m.open, from: m.from, fromRadius: m.fromR, full: ui.Rect{X: x, Y: y, W: menuWidth, H: fullH}, guess: m.at.H, radius: 20, onDark: m.onDark}
		if m.rise && fullH > 0 {
			spec.rest = h // its pages rest on its bottom, by its button
		}
		shape, _ := a.growPane(c, spec, func(panel ui.Element) {
			panel.Padding(6).Gap(1).TextColor(ink.text).Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label("菜单")
			panel.Children(func() {
				// The menu and a list of it are pages: the list comes in from the
				// right as the menu goes out to the left, and back the other way.
				if sub := m.sub; sub != nil {
					page := ui.Column(c.Key("menu-page-" + sub.label)).Gap(1).Shrink(0).Transition(menuPage(1))
					m.measure(page)
					m.pageIn(page, "sub:"+sub.label)
					page.Children(func() {
						if a.menuRow(c.Key("back"), menuAction{label: "返回", title: sub.label, icon: iconBack}, false, ink).Clicked() {
							m.sub, m.query = nil, ""
						}
						menuLine(c.Key("back-line"), ink)
						a.menuSearch(c, ink)
						ui.Scroll(c.Key("menu-list")).MaxHeight(menuListHeight).Gap(1).Children(func() { a.menuGroups(c, matching(sub.sub, m.query), ink) })
					})
					return
				}
				page := ui.Column(c.Key("menu-page")).Gap(1).Shrink(0).Transition(menuPage(-1))
				m.measure(page)
				m.pageIn(page, "")
				page.Children(func() {
					if m.head != nil {
						m.head(c, ink)
						menuLine(c.Key("head-line"), ink)
					}
					a.menuSearch(c, ink)
					groups := matching(m.groups, m.query)
					if room := m.room(h); menuRows(m.groups) > room {
						ui.Scroll(c.Key("menu-groups")).MaxHeight(room).Gap(1).Children(func() { a.menuGroups(c, groups, ink) })
					} else {
						a.menuGroups(c, groups, ink)
					}
				})
			})
		})
		if m.open && (shape.PressedOutside() || shape.OverlayShortcut(0, ui.KeyEscape)) {
			// The page showing stays as it goes: put back to the menu's first
			// page now, it would change pages and height while it shrank.
			// It opens next at its first page (openMenu).
			m.open, m.closedAt = false, time.Now()
		}
	})
}

// menuHeld reports whether el is what the menu open was opened for with the
// secondary button: it keeps its hover while the menu shows, as on macOS.
func (a *app) menuHeld(el ui.Element) bool {
	return a.menu.source != 0 && a.menu.source == el.ID() && a.menu.open
}

// menuSearch searches a long list of the menu open, as of the playlists to
// add a song to; a list that finds nothing says so.
func (a *app) menuSearch(c *ui.Context, ink paneInk) {
	m := &a.menu
	if !m.searchable() {
		return
	}
	field := ui.Row(c.Key("menu-search-row")).Height(32).Margin(0, 2, 6, 2).PaddingX(10).Gap(7).Radius(14).AlignItems(ui.Center).Background(ink.hover)
	field.Children(func() {
		ui.Icon(c, iconSearch).Size(13, 13).TextColor(ink.muted)
		ui.TextInputBase(c.Key("menu-search"), &m.query).AutoFocus().Grow(1).MinWidth(0).
			Placeholder("搜索").FontSize(12.5).Label("搜索")
		if m.query != "" && iconButton(c.Key("menu-search-clear"), iconX, 11, ink.muted, "清除搜索").Size(20, 20).Clicked() {
			m.query = ""
		}
	})
	if len(matching(m.listed(), m.query)) == 0 {
		ui.Text(c.Key("menu-search-none"), "没有找到").FontSize(12.5).TextColor(ink.muted).Padding(10, 12)
	}
}

// menuGroups lists groups in the menu, a line between them.
func (a *app) menuGroups(c *ui.Context, groups [][]menuAction, ink paneInk) {
	for i, group := range groups {
		if i > 0 {
			menuLine(c.Key(i), ink)
		}
		for _, act := range group {
			if row := a.menuRow(c, act, act.sub != nil, ink); row.Clicked() {
				if act.locked != "" {
					a.toast = act.locked
					continue
				}
				if act.sub != nil {
					a.menu.sub, a.menu.query = &act, ""
				} else {
					a.menu.open = false // on the page it was chosen on, as it goes
					a.pressedFrom(row, 14)
					act.do()
				}
			}
		}
	}
}

// menuRow is a row of a menu, its corners concentric with the pane's,
// lighter under the pointer; opens says that it opens more.
func (a *app) menuRow(c *ui.Context, act menuAction, opens bool, ink paneInk) ui.Element {
	b := ui.ButtonBase(c.Key(act.label)).Role(ui.RoleMenuItem).Label(act.label).Height(38).PaddingX(10).Gap(11).Radius(14).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	bg := ui.Transparent
	switch {
	case act.locked != "":
		// Dimmed, and still: its tooltip says why.
		b.Opacity(0.42).Tooltip(act.locked)
	case b.Pressed():
		bg = ink.press
	case b.Hovered() || b.FocusVisible():
		bg = ink.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
	title := act.title
	if title == "" {
		title = act.label
	}
	b.Children(func() {
		if act.icon != nil {
			ui.Icon(c, act.icon).Size(16, 16).Shrink(0).TextColor(ink.muted)
		}
		ui.Text(c, title).FontSize(13.5).FontWeight(500).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if act.tag != "" {
			vipTag(c, act.tag, act.tag == "SVIP")
		}
		if act.note != "" {
			ui.Text(c, act.note).FontSize(12).TextColor(ink.faint).Shrink(0)
		}
		if act.checked {
			ui.Icon(c, iconCheck).Size(15, 15).Shrink(0).TextColor(a.pal.accent)
		}
		if opens {
			ui.Icon(c, iconForward).Size(14, 14).Shrink(0).TextColor(ink.faint)
		}
	})
	return b
}

// menuLine parts groups of a menu.
func menuLine(c *ui.Context, ink paneInk) {
	ui.Box(c).Height(0.5).Margin(4, 10).Background(ink.line)
}

// paneSelect is a select of options, its list a pane, reporting a change.
// family, if given, is the font each option is shown in.
func (a *app) paneSelect(c *ui.Context, label string, selected *string, options []string, family ...func(option string) string) bool {
	before := *selected
	font := func(e ui.Element, option string) {
		if len(family) > 0 {
			e.Font(family[0](option))
		}
	}
	ink := a.inkOn(false)
	sel := ui.SelectBase(c, selected)
	bg := ink.hover
	if sel.Trigger.Hovered() {
		bg = ink.press
	}
	sel.Trigger.MinWidth(112).MaxWidth(220).Height(32).PaddingX(14).Gap(8).Radius(999).Border(0.5, ink.line).AlignItems(ui.Center).FocusRing(false).Label(label).
		Background(sel.Trigger.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond})).Children(func() {
		font(ui.Text(c, *selected).FontSize(13).SingleLine().Grow(1).Shrink(1).MinWidth(0).TextColor(ink.text), *selected)
		needsTag(c, *selected)
		ui.Icon(c, iconUpDown).Size(13, 13).Shrink(0).TextColor(ink.faint)
	})
	// The list grows out of the button, and goes back into it: the select
	// builds it no more once it closes, so it goes as rows built where it
	// was (growGhost).
	key := "select:" + label
	open := sel.Open()
	grown := a.growK(c, key, open, growOpen)
	from := sel.Trigger.Bounds()
	const rowH, rowGap, pad = 34, 1, 6
	list := func(item func(o string) ui.Element, after func()) func(body ui.Element) {
		return func(body ui.Element) {
			body.Padding(pad).Gap(rowGap)
			body.Children(func() {
				for _, o := range options {
					row := item(o).Height(rowH).PaddingX(10).Gap(8).Radius(14).AlignItems(ui.Center)
					row.Children(func() {
						font(ui.Text(c, o).FontSize(13).SingleLine().Grow(1).Shrink(1).MinWidth(0), o)
						needsTag(c, o)
						if o == *selected {
							ui.Icon(c, iconCheck).Size(14, 14).Shrink(0).TextColor(a.pal.accent)
						}
					})
				}
				if after != nil {
					after()
				}
			})
		}
	}
	sel.Popup(func(panel ui.Element) {
		panel.Width(190).Margin(6, 0, 0, 0).TextColor(ink.text)
		a.growInside(c, panel, key, grown, true, from, 16, 20, 190, 0, func(body ui.Element) {
			// The highlight slides from row to row under the pointer,
			// rising into a lens of liquid glass on its way, or as its row
			// is held, and landing as it gets there.
			liquid := a.liquidOK()
			hl, held, i := -1, false, 0
			var pos, lift float32
			if liquid {
				body.Draw(func(p *ui.Painter, b ui.Rect) {
					if hl < 0 || lift >= 0.999 {
						return
					}
					f := ink.hover
					p.Fill(ui.Rect{X: b.X + pad, Y: b.Y + pad + pos*(rowH+rowGap), W: b.W - 2*pad, H: rowH}, f.Alpha(1-lift), 14)
				})
			}
			list(func(o string) ui.Element {
				item := sel.Item(o)
				if item.Highlighted() {
					hl, held = i, item.Pressed()
					if !liquid {
						item.Background(ink.hover)
					}
				}
				i++
				return item
			}, func() {
				if !liquid || hl < 0 {
					return
				}
				pos = body.AnimateTo("hl", float32(hl), lensSlide)
				lift = lensLift(body, pos, float32(hl), held)
				if lift > 0.01 {
					a.liquidLens(c, "hl-lens", ui.Rect{X: pad, Y: pad + pos*(rowH+rowGap), W: 190 - 2*pad, H: rowH}, lift, 0.04, 0.2)
				}
			})(body)
		})
	})
	if !open {
		a.growGhost(c, key, grown, from, 16, 20, 190, 0, func(body ui.Element) {
			body.TextColor(ink.text)
			list(func(o string) ui.Element { return ui.Row(c.Key(o)) }, nil)(body)
		})
	}
	return *selected != before
}

// paneToasts builds the window's toasts as panes, above the player.
func (a *app) paneToasts(c *ui.Context) {
	ink := a.inkOn(a.nowPlaying)
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(0, 0, playerBarHeight+10, 0).AlignItems(ui.Center).Justify(ui.End).Gap(8)
		for _, t := range toasts {
			toast := ui.ToastBase(c, t)
			a.pane(toast.Root, 20, a.nowPlaying)
			toast.Root.Row().Padding(10, 16).TextColor(ink.text).
				Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Y: 8}, Exit: &ui.Motion{}})
			toast.Root.Children(func() { ui.Text(c, t.Title).FontSize(13).FontWeight(500) })
		}
	})
}

// plainPane paints e as a pane with no glass: solid, with a hairline.
func (a *app) plainPane(e ui.Element, onDark bool) ui.Element {
	if onDark || a.pal.dark {
		return e.Background(ui.Hex("#232327")).Border(0.5, white.Alpha(0.12))
	}
	return e.Background(ui.Hex("#f6f6f8")).Border(0.5, ui.RGBA(0, 0, 0, 0.12))
}

// menuPage is how a page of a menu comes and goes: the menu itself from
// and to the left (side -1), a list of it from and to the right (side 1),
// sliding a little as it fades, over the pane's own time.
func menuPage(side float32) ui.ElementTransition {
	return ui.ElementTransition{Animation: &motionPane, Position: true,
		Enter: &ui.Motion{X: 36 * side, Opacity: 1}, Exit: &ui.Motion{X: 36 * side}}
}

// menuPageIn is how the page coming in fades: once the one going has mostly
// gone, so that the two are not read through each other.
var menuPageIn = ui.Animation{Duration: 200 * time.Millisecond, Delay: 90 * time.Millisecond, Ease: motionEase}

// pageIn fades page, named name, in as it takes the place of another page;
// the menu's first page comes with the pane, as it grows.
func (m *paneMenu) pageIn(page ui.Element, name string) {
	if name != m.page {
		m.page = name
		page.Jump("in", 0)
	}
	if f := page.AnimateTo("in", 1, menuPageIn); f < 1 {
		page.Opacity(f)
	}
}

// measure notes how tall page, the page of m showing, was laid out, with
// the pane's padding round it.
func (m *paneMenu) measure(page ui.Element) {
	if b := page.LayoutBounds(); b.H > 0 {
		m.natural = b.H + 12
	}
}
