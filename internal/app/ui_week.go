package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// 听歌周报: the listener's weeks, Monday to Sunday by NetEase's (Beijing's)
// clock, from the ledger Cloudy keeps (listen_log.go): this week, the weeks
// before it, and all of it with NetEase's own figures. The calendar has a
// page of the week, which opens it.

// weekState is what the page shows: which of its tabs, and in 往期 which
// week.
type weekState struct {
	tab  int
	pick int64 // the week 往期 shows, by its start; 0 for last week
}

const (
	weekTabNow = iota
	weekTabPast
	weekTabAll
)

// weekLength says seconds heard as "2 小时 47 分" or "35 分钟".
func weekLength(secs int) string {
	m := secs / 60
	switch {
	case m < 60:
		return fmt.Sprintf("%d 分钟", m)
	case m%60 == 0:
		return fmt.Sprintf("%d 小时", m/60)
	}
	return fmt.Sprintf("%d 小时 %d 分", m/60, m%60)
}

// weekRange names the week starting at start, as "11.23 – 11.29".
func weekRange(start time.Time) string {
	s := start.In(beijing)
	e := s.AddDate(0, 0, 6)
	return fmt.Sprintf("%d.%02d – %d.%02d", s.Month(), s.Day(), e.Month(), e.Day())
}

// weekNumber is the ISO week of the year start is in.
func weekNumber(start time.Time) int {
	_, n := start.In(beijing).ISOWeek()
	return n
}

var weekdays = [7]string{"一", "二", "三", "四", "五", "六", "日"}

// weekdayOf is t's day of the week, 0 for Monday, by Beijing's clock.
func weekdayOf(t time.Time) int {
	return (int(t.In(beijing).Weekday()) + 6) % 7
}

// topArtists names the singers of r, the most played first, n of them at
// most.
func topArtists(r *weekReport, n int) string {
	var names []string
	for i := range r.artists[:min(n, len(r.artists))] {
		names = append(names, r.artists[i].Name)
	}
	return strings.Join(names, "、")
}

// deckWeekReport is the week the calendar's page shows: this one, or last
// week while this one has nothing yet.
func (a *app) deckWeekReport(now time.Time) (*weekReport, bool) {
	start := weekStartOf(now)
	if r := a.report(start); len(r.songs) > 0 {
		return r, true
	}
	return a.report(start.AddDate(0, 0, -7)), false
}

// percentChange says how secs compares with before, as "+18%", or "" when
// there is nothing to compare with.
func percentChange(secs, before int) string {
	if before <= 0 {
		return ""
	}
	p := (secs - before) * 100 / before
	if p >= 0 {
		return fmt.Sprintf("+%d%%", p)
	}
	return fmt.Sprintf("%d%%", p)
}

// devicesLine says how a week's plays split between phones and computers.
func devicesLine(r *weekReport) string {
	n := r.phone + r.computer
	switch {
	case n == 0:
		return ""
	case r.phone == 0:
		return "全在电脑上听"
	case r.computer == 0:
		return "全在手机上听"
	}
	p := (r.phone*100 + n/2) / n
	return fmt.Sprintf("手机 %d%% · 电脑 %d%%", p, 100-p)
}

// The calendar's page.

// weekSheet is 听歌周报's page of the calendar: how long the listener
// listened this week, how many songs and whose most, how they split
// between phones and computers, a button that plays the week's songs; the
// first three, their covers numbered; and each day, as bars. It opens the
// 听歌周报 page.
func (a *app) weekSheet(c *ui.Context, back bool) {
	now := time.Now()
	r, current := a.deckWeekReport(now)
	if len(r.songs) == 0 {
		return
	}
	tracks := r.tracks()
	a.knownPrivileges(tracks)
	inner := a.deckInner(c)
	ours := a.queueIs(tracks)
	playing := a.pl.Playing()
	label := "听歌周报 · " + weekRange(r.start)
	if !current {
		label = "上周听歌 · " + weekRange(r.start)
	}
	a.deckSheet(c, deckWeek, back, r.songs[0].Track.Album.PicURL, func() {
		a.week = weekState{tab: weekTabNow}
		if !current {
			a.week = weekState{tab: weekTabPast, pick: r.start.Unix()}
		}
		a.open(route{page: pageWeek})
	}, func() {
		deckLabel(c, iconHistory, label)
		deckTitle(c, weekLength(r.secs))
		line := fmt.Sprintf("听了 %d 首", len(r.songs))
		if who := topArtists(r, 2); who != "" {
			line += " · 最常听 " + who
		}
		ui.Text(c, line).FontSize(13).TextColor(white.Alpha(0.8)).MaxLines(1)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Margin(12, 0, 0, 0).Children(func() {
			a.dailyPlayButton(c.Key("play"), tracks, ours, ours && playing)
			if d := devicesLine(r); d != "" {
				ui.Row(c).Gap(5).AlignItems(ui.Center).TextColor(white.Alpha(0.72)).Children(func() {
					ui.Icon(c, iconMiniWin).Size(13, 13)
					ui.Text(c, d).FontSize(12).FontWeight(600).FontFeatures("tnum").SingleLine()
				})
			}
		})
	}, func(hovered bool) {
		if inner >= 860 {
			a.weekTop(c, r, tracks, ours)
		}
		switch {
		case inner >= 620:
			today := -1
			if current {
				today = weekdayOf(now)
			}
			weekBars(c, r.days, today)
		case inner >= deckSmallMin:
			a.weekMosaic(c, tracks, hovered, deckSmallCover)
		}
	})
}

// weekTop is the week's first three songs, their covers numbered, each
// playing the week's songs from it.
func (a *app) weekTop(c *ui.Context, r *weekReport, tracks []netease.Track, ours bool) {
	cur := a.pl.Current()
	playing := a.pl.Playing()
	ui.Row(c.Key("top")).Gap(14).Shrink(0).AlignItems(ui.Start).Children(func() {
		for i := range min(3, len(r.songs)) {
			s := &r.songs[i]
			current := ours && cur != nil && cur.ID == s.Track.ID
			label := "播放 " + s.Track.Name
			if current && playing {
				label = "暂停 " + s.Track.Name
			}
			b := ui.ButtonBase(c.Key(playKey(&s.Track))).Column().Width(84).Gap(6).FocusRing(false).Label(label)
			up := b.AnimateTo("up", b2f(b.Hovered()), ui.Snappy)
			b.Children(func() {
				ui.Box(c).Size(84, 84).Radius(14).Shadow(0, 8, 18, -6, ui.RGBA(0, 0, 0, 0.45)).Translate(0, -3*up).Children(func() {
					a.coverFill(c, s.Track.Album.PicURL, 200, 14)
					ui.Box(c).Absolute().Top(6).Left(6).Size(22, 22).Radius(999).Center().Background(ui.RGBA(0, 0, 0, 0.55)).Children(func() {
						ui.Text(c, fmt.Sprint(i+1)).FontSize(11.5).FontWeight(800).TextColor(white)
					})
					weekPlayMark(c, b.Hovered(), current, current && playing)
				})
				ui.Text(c, s.Track.Name).FontSize(12).FontWeight(600).SingleLine()
				ui.Textf(c, "%d 次", s.Plays).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.6)).SingleLine()
			})
			if b.DoubleClicked() {
				if current {
					a.pl.TogglePlay()
				} else {
					a.playPicked(tracks, i)
				}
			}
		}
	})
}

// weekBars is how long each day of the week was listened to, as bars, the
// day now is in (today, or -1) lit and the days after it flat.
func weekBars(c *ui.Context, days [7]int, today int) {
	most := 1
	for _, n := range days {
		most = max(most, n)
	}
	ui.Column(c.Key("bars")).Gap(6).Shrink(0).Margin(0, 6, 0, 0).Children(func() {
		ui.Text(c, "每天听了多久").FontSize(11).FontWeight(600).TextColor(white.Alpha(0.6))
		ui.Row(c).Height(96).Gap(8).AlignItems(ui.End).Children(func() {
			for i, n := range days {
				share := float32(n) / float32(most)
				ui.Column(c).Width(16).Height(96).Justify(ui.End).Tooltip("周" + weekdays[i] + " · " + weekLength(n)).Children(func() {
					if today >= 0 && i > today {
						ui.Box(c).Height(3).Radius(4).Background(white.Alpha(0.2))
						return
					}
					fill := white.Alpha(0.5)
					if i == today {
						fill = white.Alpha(0.95)
					}
					ui.Box(c).Height(max(3, 96*share)).Radius(4).Background(fill)
				})
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			for i := range 7 {
				ui.Text(c, weekdays[i]).Width(16).FontSize(10.5).TextColor(white.Alpha(0.6)).TextAlign(ui.Center)
			}
		})
	})
}

// weekMosaic is the week's first four covers, two by two, where the page
// is narrow.
func (a *app) weekMosaic(c *ui.Context, tracks []netease.Track, hovered bool, size float32) {
	box := ui.Box(c.Key("mosaic")).Size(size, size).Shrink(0).Radius(14).ClipY().Shadow(0, 10, 24, -6, ui.RGBA(0, 0, 0, 0.45)).Margin(0, 12, 0, 0)
	pose := ui.Pose{}
	if hovered {
		pose = ui.Pose{Y: -3, Scale: 1.03}
	}
	box.AnimatePose(pose, ui.Snappy)
	box.Children(func() {
		for i := range min(4, len(tracks)) {
			ui.Box(c).Absolute().Left(float32(i%2)*size/2).Top(float32(i/2)*size/2).Size(size/2, size/2).Children(func() {
				a.coverFill(c, tracks[i].Album.PicURL, 200, 0)
			})
		}
	})
}

// weekPlayMark is the small round button at the foot of one of the week's
// first covers, its tile's click being its own: play, under the pointer;
// on the song playing, pause, or play while it is paused.
func weekPlayMark(c *ui.Context, hovered, current, playing bool) {
	const size = 26
	shown := ui.Box(c.Key("mark")).Absolute().Right(6).Bottom(6).Size(size, size).Radius(999).Center().PassThrough().
		Background(white).Shadow(0, 4, 10, -2, ui.RGBA(0, 0, 0, 0.35))
	on := shown.AnimateTo("on", b2f(hovered || current), ui.Snappy)
	shown.Opacity(on).AnimatePose(ui.Pose{Scale: 0.7 + 0.3*on}, ui.Snappy)
	icon := iconPlay
	if playing {
		icon = iconPause
	}
	shown.TextColor(ui.Hex("#1d1d1f")).Children(func() {
		// A play icon sits a little right of its middle, as it looks.
		m := ui.Icon(c, icon).Size(11, 11)
		if !playing {
			m.Margin(0, 0, 0, 1)
		}
	})
}

// The page.

// weekPage is 听歌周报: this week, the weeks before, or all of it, each
// over its songs.
func (a *app) weekPage(c *ui.Context) {
	if a.profile == nil {
		a.centered(c, "登录后才能查看听歌报告", nil)
		return
	}
	now := time.Now()
	switch a.week.tab {
	case weekTabPast:
		a.pastWeekPage(c, now)
	case weekTabAll:
		a.allListenPage(c, now)
	default:
		a.thisWeekPage(c, now)
	}
}

// weekInner is how wide the page's content is.
func (a *app) weekInner(c *ui.Context) float32 {
	w, _ := c.Size()
	return w - a.sideWidth(w) - 2*pagePad
}

// weekHead is the page's title, the line over it, and the tabs at its
// right.
func (a *app) weekHead(c *ui.Context, title, sub string) {
	ui.Row(c.Key("head")).Gap(20).AlignItems(ui.End).Justify(ui.SpaceBetween).Margin(0, 0, 18, 0).Children(func() {
		ui.Column(c).Gap(4).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, sub).FontSize(12).FontWeight(600).TextColor(a.pal.muted).LetterSpacing(0.4).MaxLines(1)
			ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6).MaxLines(1)
		})
		tab := a.week.tab
		if a.segmented(c.Key("tabs"), &tab, "本周", "往期", "全部") && tab != a.week.tab {
			a.week.tab = tab
			if tab == weekTabAll {
				a.loadAllListening()
			}
		}
	})
}

// weekCard is a panel of the page.
func (a *app) weekCard(c *ui.Context) ui.Element {
	bg := ui.RGBA(255, 255, 255, 0.55)
	if a.pal.dark {
		bg = ui.RGBA(255, 255, 255, 0.05)
	}
	return ui.Column(c).Padding(18, 20).Radius(20).Background(bg).Border(1, a.pal.hairline).Gap(12).MinWidth(0)
}

// weekCardTitle is a panel's title, with words at its right if any.
func (a *app) weekCardTitle(c *ui.Context, title, aside string) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
		ui.Text(c, title).FontSize(16).FontWeight(700).MaxLines(1)
		if aside != "" {
			ui.Text(c, aside).FontSize(12).TextColor(a.pal.muted).MaxLines(1)
		}
	})
}

// weekTile is a figure of the page: what it is, the figure, and a line
// under it, with more under that if any.
func (a *app) weekTile(c *ui.Context, key, label, value, note string, more func()) {
	a.weekCard(c.Key(key)).Gap(6).Children(func() {
		ui.Text(c, label).FontSize(12.5).FontWeight(600).TextColor(a.pal.muted).MaxLines(1)
		ui.Text(c, value).FontSize(26).FontWeight(700).LetterSpacing(-0.5).FontFeatures("tnum").MaxLines(1)
		if more != nil {
			more()
		}
		if note != "" {
			ui.Text(c, note).FontSize(12.5).TextColor(a.pal.muted).MaxLines(2)
		}
	})
}

// weekGrid lays out cells in as many columns as fit cells minW wide, most
// at most.
func (a *app) weekGrid(c *ui.Context, key string, minW float32, most int, cells func()) {
	n := max(1, min(most, int((a.weekInner(c)+14)/(minW+14))))
	ui.Grid(c.Key(key)).Columns(n).GapX(14).GapY(14).Margin(0, 0, 14, 0).Children(cells)
}

// weekSection is a title over part of the page.
func (a *app) weekSection(c *ui.Context, title, aside string) {
	ui.Row(c).Gap(10).AlignItems(ui.End).Justify(ui.SpaceBetween).Margin(16, 0, 12, 0).Children(func() {
		ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3)
		if aside != "" {
			ui.Text(c, aside).FontSize(12).TextColor(a.pal.muted)
		}
	})
}

// weekNote is a line saying where the page's figures come from.
func (a *app) weekNote(c *ui.Context, text string) {
	ui.Row(c.Key("note")).Gap(8).AlignItems(ui.Start).Padding(12, 16).Radius(14).Background(a.pal.hover).Margin(4, 0, 22, 0).Children(func() {
		ui.Icon(c, iconInfo).Size(14, 14).Shrink(0).TextColor(a.pal.muted).Margin(2, 0, 0, 0)
		ui.Text(c, text).FontSize(12.5).LineHeight(1.5).TextColor(a.pal.muted).Grow(1).Shrink(1).MinWidth(0)
	})
}

// weekPlayButtons are the buttons that play the week's songs, in order or
// shuffled.
func (a *app) weekPlayButtons(c *ui.Context, tracks []netease.Track) {
	p := a.pal
	ui.Row(c.Key("buttons")).Gap(10).Wrap().Margin(0, 0, 18, 0).Children(func() {
		if pillButton(c.Key("play"), iconPlay, "播放全部", p.accent, p.onAccent).Clicked() {
			a.playList(tracks, false)
		}
		if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
			a.playList(tracks, true)
		}
	})
}

// thisWeekPage is the week now is in.
func (a *app) thisWeekPage(c *ui.Context, now time.Time) {
	start := weekStartOf(now)
	r := a.report(start)
	tracks := r.tracks()
	a.knownPrivileges(tracks)
	today := weekdayOf(now)
	header := func(c *ui.Context) {
		sub := fmt.Sprintf("%s · 第 %d 周", weekRange(start), weekNumber(start))
		if left := 6 - today; left > 0 {
			sub += fmt.Sprintf(" · 还剩 %d 天", left)
		} else {
			sub += " · 今天是最后一天"
		}
		a.weekHead(c, "本周听歌", sub)
		if len(r.songs) == 0 {
			a.empty(c, "这周还没有听歌记录")
			return
		}
		a.weekPlayButtons(c, tracks)
		last := a.report(start.AddDate(0, 0, -7))
		a.weekFigures(c, r, last.secsUntil(now.AddDate(0, 0, -7)), "比上周同期")
		a.weekTime(c, r, &last.days, today)
		a.weekRanks(c, r, tracks)
		a.weekFacts(c, r, now)
		a.weekNote(c, "按北京时间算一周（周一到周日），和网易云的日子一致。在 Cloudy 里听的逐次记录，时长按实际听的秒数；手机等其他设备从网易云「最近播放」补记，同一首歌在两次补记之间重复听只算一次。")
		a.weekSection(c, "这周听过的歌", fmt.Sprintf("共 %d 首 · 按播放次数", len(r.songs)))
	}
	a.toolRows(c, tracks, true, header)
}

// pastWeekPage is one of the weeks before this one, chosen from a strip of
// them.
func (a *app) pastWeekPage(c *ui.Context, now time.Time) {
	l := a.ledger()
	weeks := listenWeeks(l.plays, now)
	weeks = weeks[:len(weeks)-1] // not this one
	header := func(c *ui.Context) {
		sub := "还没有往期记录"
		if len(weeks) > 0 {
			sub = fmt.Sprintf("记录了 %d 周 · 从 %s 开始", len(weeks), weeks[0].Format("2006.01.02"))
		}
		a.weekHead(c, "往期", sub)
	}
	if len(weeks) == 0 {
		a.toolRows(c, nil, true, func(c *ui.Context) {
			header(c)
			a.empty(c, "上周及以前还没有听歌记录，过完这周就有了")
		})
		return
	}
	pick := weeks[len(weeks)-1]
	for _, w := range weeks {
		if w.Unix() == a.week.pick {
			pick = w
		}
	}
	r := a.report(pick)
	tracks := r.tracks()
	a.knownPrivileges(tracks)
	before := a.report(pick.AddDate(0, 0, -7))
	a.toolRows(c, tracks, true, func(c *ui.Context) {
		header(c)
		a.weekStrip(c, weeks, pick)
		a.weekPicker(c, weeks, pick, r, tracks)
		if len(r.songs) == 0 {
			a.empty(c, "这一周没有听歌记录")
			return
		}
		a.weekFigures(c, r, before.secs, "比前一周")
		a.weekTime(c, r, &before.days, -1)
		a.weekRanks(c, r, tracks)
		a.weekFacts(c, r, now)
		a.weekSection(c, "这周听过的歌", fmt.Sprintf("共 %d 首 · 按播放次数", len(r.songs)))
	})
}

// weekStrip is the last weeks, each a bar as long as it was listened to:
// one clicked shows it.
func (a *app) weekStrip(c *ui.Context, weeks []time.Time, pick time.Time) {
	shown := weeks[max(0, len(weeks)-12):]
	most := 1
	for _, w := range shown {
		most = max(most, a.report(w).secs)
	}
	p := a.pal
	a.weekCard(c.Key("strip")).Margin(0, 0, 16, 0).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.End).Height(118).Children(func() {
			for _, w := range shown {
				r := a.report(w)
				on := w.Equal(pick)
				b := ui.ButtonBase(c.Key(w.Unix())).Column().Grow(1).Basis(0).MinWidth(0).MaxWidth(64).Height(118).Justify(ui.End).AlignItems(ui.Stretch).Gap(5).
					Label(weekRange(w) + " · " + weekLength(r.secs))
				hovered := b.Hovered()
				b.Children(func() {
					ui.Text(c, fmt.Sprintf("%.1f", float32(r.secs)/3600)).FontSize(10.5).FontFeatures("tnum").TextColor(p.muted).TextAlign(ui.Center).SingleLine()
					fill := p.accent.Alpha(0.45)
					if on {
						fill = p.accent
					} else if hovered {
						fill = p.accent.Alpha(0.65)
					}
					bar := ui.Box(c).FillWidth().Height(max(4, 80*float32(r.secs)/float32(most))).Radius(8).Background(fill)
					if r.partial {
						bar.Background(fill.Alpha(0.25)).Border(1, fill).BorderStyle(ui.BorderDashed)
					}
					label := ui.Text(c, fmt.Sprintf("%d.%02d", w.Month(), w.Day())).FontSize(11).FontFeatures("tnum").TextAlign(ui.Center).SingleLine()
					if on {
						label.FontWeight(700)
					} else {
						label.TextColor(p.muted)
					}
				})
				if b.Clicked() {
					a.week.pick = w.Unix()
				}
			}
		})
		ui.Text(c, "柱子是每周听了几个小时；虚线的周在开始记录之前，只有每首歌的最后一次播放。").FontSize(12).TextColor(p.muted)
	})
}

// weekPicker is the week shown, the buttons to the ones beside it, and
// what can be done with it.
func (a *app) weekPicker(c *ui.Context, weeks []time.Time, pick time.Time, r *weekReport, tracks []netease.Track) {
	at := 0
	for i, w := range weeks {
		if w.Equal(pick) {
			at = i
		}
	}
	p := a.pal
	ui.Row(c.Key("picker")).Gap(12).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Margin(0, 0, 16, 0).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			prev := iconButton(c.Key("prev"), iconBack, 16, p.text, "上一周").Disabled(at == 0)
			if prev.Clicked() && at > 0 {
				a.week.pick = weeks[at-1].Unix()
			}
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, weekRange(pick)).FontSize(22).FontWeight(700).FontFeatures("tnum")
				ui.Textf(c, "第 %d 周", weekNumber(pick)).FontSize(12).TextColor(p.muted)
			})
			next := iconButton(c.Key("next"), iconForward, 16, p.text, "下一周").Disabled(at == len(weeks)-1)
			if next.Clicked() && at < len(weeks)-1 {
				a.week.pick = weeks[at+1].Unix()
			}
			chip := "已结束"
			if r.partial {
				chip = "补记 · 只有每首歌最后一次"
			}
			ui.Text(c, chip).FontSize(12).FontWeight(600).Padding(5, 10).Radius(999).Background(p.accent.Alpha(0.14))
		})
		if len(tracks) == 0 {
			return
		}
		ui.Row(c).Gap(10).Children(func() {
			if pillButton(c.Key("play"), iconPlay, "播放这一周", p.accent, p.onAccent).Clicked() {
				a.playList(tracks, false)
			}
			if pillButton(c.Key("save"), iconListPlus, "存为歌单", p.hover.Over(p.bg), p.text).Clicked() {
				a.newPlaylistOf(tracks)
				if a.naming.open {
					a.naming.name = "听歌周报 " + weekRange(pick)
				}
			}
		})
	})
}

// weekFigures is the week's four figures: how long, how many songs, how
// many new and where they were played. before is how long was listened to
// to compare with, said as compared.
func (a *app) weekFigures(c *ui.Context, r *weekReport, before int, compared string) {
	p := a.pal
	a.weekGrid(c, "figures", 200, 4, func() {
		note := "按实际听的秒数"
		if d := percentChange(r.secs, before); d != "" {
			note += " · " + compared + " " + d
		}
		a.weekTile(c, "time", "听歌时长", weekLength(r.secs), note, nil)
		a.weekTile(c, "songs", "听过的歌", fmt.Sprintf("%d 首", len(r.songs)), fmt.Sprintf("%d 位歌手 · 共播放 %d 次", len(r.artists), r.count), nil)
		if r.freshKnown {
			a.weekTile(c, "fresh", "新发现", fmt.Sprintf("%d 首", r.fresh), "这周第一次听到的歌", nil)
		} else {
			liked := 0
			for i := range r.songs {
				if a.liked[r.songs[i].Track.ID] {
					liked++
				}
			}
			a.weekTile(c, "liked", "红心的歌", fmt.Sprintf("%d 首", liked), fmt.Sprintf("占这周听过的 %d%%", liked*100/max(1, len(r.songs))), nil)
		}
		n := r.phone + r.computer
		value, note := "—", ""
		if n > 0 {
			pc := (r.computer*100 + n/2) / n
			value, note = fmt.Sprintf("电脑 %d%%", pc), fmt.Sprintf("手机 %d%%", 100-pc)
		}
		a.weekTile(c, "devices", "在哪听", value, note, func() {
			if n == 0 {
				return
			}
			ui.Row(c).Height(8).Radius(999).ClipY().Background(p.hover).Children(func() {
				if r.computer > 0 {
					ui.Box(c).Grow(float32(r.computer)).Basis(0).Height(8).Background(p.accent)
				}
				if r.phone > 0 {
					ui.Box(c).Grow(float32(r.phone)).Basis(0).Height(8).Background(p.accent.Alpha(0.35))
				}
			})
		})
	})
}

// weekTime is how long each day was listened to, beside the week before's
// and, today being in the week (not -1), with the days to come flat; and
// the hours of the day the plays started at.
func (a *app) weekTime(c *ui.Context, r *weekReport, last *[7]int, today int) {
	p := a.pal
	a.weekGrid(c, "when", 380, 2, func() {
		a.weekCard(c.Key("days")).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				ui.Text(c, "每天听了多久").FontSize(16).FontWeight(700)
				ui.Row(c).Gap(12).Children(func() {
					weekLegend(c, p.accent, "这周", p.muted)
					weekLegend(c, p.text.Alpha(0.14), "前一周", p.muted)
				})
			})
			most := 1
			for i := range 7 {
				most = max(most, r.days[i], last[i])
			}
			const tall = 120
			ui.Row(c).Height(tall + 18).Gap(6).AlignItems(ui.End).Children(func() {
				for i := range 7 {
					ui.Column(c.Key(i)).Grow(1).Basis(0).Height(tall + 18).Justify(ui.End).AlignItems(ui.Center).Gap(4).
						Tooltip(fmt.Sprintf("周%s · %s", weekdays[i], weekLength(r.days[i]))).Children(func() {
						if r.days[i] > 0 {
							ui.Text(c, fmt.Sprint(r.days[i]/60)).FontSize(10.5).FontFeatures("tnum").TextColor(p.muted)
						}
						ui.Row(c).Gap(3).AlignItems(ui.End).Height(tall).Children(func() {
							ui.Box(c).Width(8).Height(max(2, tall*float32(last[i])/float32(most))).Radius(3).Background(p.text.Alpha(0.14))
							if today >= 0 && i > today {
								ui.Box(c).Width(14).Height(8).Radius(4).Border(1, p.faint).BorderStyle(ui.BorderDashed)
								return
							}
							fill := p.accent.Alpha(0.6)
							if i == today {
								fill = p.accent
							}
							ui.Box(c).Width(14).Height(max(2, tall*float32(r.days[i])/float32(most))).Radius(4).Background(fill)
						})
					})
				}
			})
			ui.Row(c).Gap(6).Children(func() {
				for i := range 7 {
					t := ui.Text(c, weekdays[i]).Grow(1).Basis(0).FontSize(12).TextAlign(ui.Center)
					if i == today {
						t.FontWeight(700)
					} else {
						t.TextColor(p.muted)
					}
				}
			})
			ui.Text(c, "柱子上的数字是分钟").FontSize(11.5).TextColor(p.muted)
		})
		a.weekCard(c.Key("hours")).Children(func() {
			ui.Text(c, "一天里什么时候听").FontSize(16).FontWeight(700)
			most := 1
			for _, n := range r.hours {
				most = max(most, n)
			}
			ui.Row(c).Height(64).Gap(3).Children(func() {
				for h, n := range r.hours {
					fill := p.hover
					if n > 0 {
						fill = p.accent.Alpha(0.15 + 0.85*float32(n)/float32(most))
					}
					ui.Box(c.Key(h)).Grow(1).Basis(0).Height(64).Radius(4).Background(fill).Tooltip(fmt.Sprintf("%d 点 · %d 首", h, n))
				}
			})
			ui.Row(c).Justify(ui.SpaceBetween).Children(func() {
				for _, h := range []int{0, 6, 12, 18, 24} {
					ui.Textf(c, "%d 点", h).FontSize(11).FontFeatures("tnum").TextColor(p.muted)
				}
			})
			peak, night := 0, 0
			for h, n := range r.hours {
				if n > r.hours[peak] {
					peak = h
				}
				if h >= 22 || h < 5 {
					night += n
				}
			}
			ui.Row(c).Gap(8).Wrap().Children(func() {
				weekChip(c, p, fmt.Sprintf("最常在 %d 点前后听", peak))
				if r.count > 0 {
					weekChip(c, p, fmt.Sprintf("夜里（22 点到 5 点）听的占 %d%%", night*100/r.count))
				}
			})
			ui.Text(c, "按这台电脑的时间").FontSize(11.5).TextColor(p.muted)
		})
	})
}

func weekLegend(c *ui.Context, fill ui.Color, label string, color ui.Color) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(10, 10).Radius(3).Background(fill)
		ui.Text(c, label).FontSize(12).TextColor(color)
	})
}

func weekChip(c *ui.Context, p palette, text string) {
	ui.Text(c, text).FontSize(12.5).Padding(5, 11).Radius(999).Background(p.accent.Alpha(0.12))
}

// weekRanks is the week's first ten songs as bars as long as they were
// played, each playing the week from it; its singers and its albums.
func (a *app) weekRanks(c *ui.Context, r *weekReport, tracks []netease.Track) {
	p := a.pal
	a.weekSection(c, "最常听的歌", "次数 · 听了多久")
	top := max(1, r.songs[0].Plays)
	cols := 1
	if a.weekInner(c) >= 760 {
		cols = 2
	}
	ui.Grid(c.Key("top-ten")).Columns(cols).GapX(32).GapY(6).Margin(0, 0, 18, 0).Children(func() {
		for i := range min(10, len(r.songs)) {
			s := &r.songs[i]
			b := ui.ButtonBase(c.Key(playKey(&s.Track))).Row().Gap(12).AlignItems(ui.Center).Padding(4).Radius(8).MinWidth(0).Label("播放 " + s.Track.Name)
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Children(func() {
				ui.Text(c, fmt.Sprint(i+1)).Width(20).FontSize(13).FontWeight(700).FontFeatures("tnum").TextColor(p.muted)
				ui.Box(c).Size(40, 40).Radius(8).Shrink(0).ClipY().Children(func() {
					a.coverFill(c, s.Track.Album.PicURL, 100, 8)
				})
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
					ui.Row(c).Gap(8).Justify(ui.SpaceBetween).Children(func() {
						ui.Text(c, s.Track.Name).FontSize(13.5).FontWeight(600).SingleLine().Shrink(1).MinWidth(0)
						ui.Textf(c, "%d 次 · %s", s.Plays, weekLength(s.Secs)).FontSize(12).FontFeatures("tnum").TextColor(p.muted).SingleLine().Shrink(0)
					})
					ui.Box(c).Height(4).Radius(999).Background(p.hover).Children(func() {
						ui.Box(c).Absolute().Top(0).Left(0).Bottom(0).WidthPercent(100 * float32(s.Plays) / float32(top)).Radius(999).Background(p.accent)
					})
				})
			})
			if b.DoubleClicked() {
				a.playPicked(tracks, i)
			}
		}
	})
	a.weekGrid(c, "people", 380, 2, func() {
		a.weekCard(c.Key("artists")).Children(func() {
			a.weekCardTitle(c, "最常听的歌手", "")
			for i := range min(5, len(r.artists)) {
				ar := r.artists[i]
				a.weekNameRow(c, &ar, func() {
					if ar.ID > 0 {
						a.open(route{page: pageArtist, id: ar.ID})
					}
				})
			}
		})
		a.weekCard(c.Key("albums")).Children(func() {
			a.weekCardTitle(c, "最常听的专辑", "")
			ui.Grid(c).Columns(4).GapX(12).GapY(12).Children(func() {
				for i := range min(4, len(r.albums)) {
					al := r.albums[i]
					b := ui.ButtonBase(c.Key(al.Name)).Column().Gap(6).MinWidth(0).Label(al.Name)
					up := b.AnimateTo("up", b2f(b.Hovered()), ui.Snappy)
					b.Children(func() {
						ui.Box(c).FillWidth().AspectRatio(1).Radius(14).ClipY().Translate(0, -2*up).Shadow(0, 6, 14, -6, p.cardShade).Children(func() {
							a.coverFill(c, al.Cover, 200, 12)
						})
						ui.Text(c, al.Name).FontSize(12.5).FontWeight(600).SingleLine()
						ui.Textf(c, "%d 次 · %d 首", al.Plays, al.Songs).FontSize(11.5).TextColor(p.muted).SingleLine()
					})
					if b.Clicked() && al.ID > 0 {
						a.open(route{page: pageAlbum, id: al.ID})
					}
				}
			})
		})
	})
}

// weekNameRow is a singer: a cover of theirs, round, their name, their
// songs, and their plays; clicked, open.
func (a *app) weekNameRow(c *ui.Context, n *nameStat, open func()) {
	p := a.pal
	b := ui.ButtonBase(c.Key(n.Name)).Row().Gap(12).AlignItems(ui.Center).Padding(3).Radius(8).Label(n.Name)
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		ui.Box(c).Size(36, 36).Radius(999).Shrink(0).ClipY().Children(func() {
			a.coverFill(c, n.Cover, 100, 18)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, n.Name).FontSize(13.5).FontWeight(600).SingleLine()
			ui.Textf(c, "%d 首", n.Songs).FontSize(12).TextColor(p.muted)
		})
		ui.Textf(c, "%d 次", n.Plays).FontSize(13).FontFeatures("tnum").TextColor(p.muted)
	})
	if b.Clicked() {
		open()
	}
}

// weekFacts are the small things of the week: the latest play at night,
// the song played most in one day, where Cloudy's plays came from, and the
// days in a row.
func (a *app) weekFacts(c *ui.Context, r *weekReport, now time.Time) {
	p := a.pal
	days := listenDays(a.ledger().plays)
	streak, from := streakTo(days, now)
	if r.late == nil && r.loop.Plays < 2 && len(r.sources) == 0 && streak < 2 {
		return
	}
	a.weekSection(c, "这周的小事", "")
	a.weekGrid(c, "facts", 220, 4, func() {
		if r.late != nil {
			t := r.late.at().In(time.Local)
			a.weekFact(c, "late", iconMoonStar, "最晚的一次", fmt.Sprintf("周%s %02d:%02d", weekdays[(int(t.Weekday())+6)%7], t.Hour(), t.Minute()), "还在听《"+r.late.Track.Name+"》")
		}
		if r.loop.Plays >= 2 {
			a.weekFact(c, "loop", iconRepeat, "一天里循环最多", r.loop.Track.Name, fmt.Sprintf("周%s听了 %d 次", weekdays[weekdayOf(r.loopDay)], r.loop.Plays))
		}
		if len(r.sources) > 0 {
			a.weekCard(c.Key("sources")).Gap(10).Children(func() {
				ui.Text(c, "从哪儿点开的").FontSize(12.5).FontWeight(600).TextColor(p.muted)
				total := 0
				for _, s := range r.sources {
					total += s.Plays
				}
				shown := r.sources[:min(4, len(r.sources))]
				rest := total
				for _, s := range shown {
					rest -= s.Plays
				}
				alphas := []float32{1, 0.66, 0.4, 0.22}
				ui.Row(c).Height(10).Radius(999).ClipY().Gap(2).Children(func() {
					for i, s := range shown {
						ui.Box(c).Grow(float32(s.Plays)).Basis(0).Height(10).Background(p.accent.Alpha(alphas[i]))
					}
					if rest > 0 {
						ui.Box(c).Grow(float32(rest)).Basis(0).Height(10).Background(p.text.Alpha(0.12))
					}
				})
				ui.Grid(c).Columns(2).GapX(10).GapY(6).Children(func() {
					for i, s := range shown {
						weekLegend(c, p.accent.Alpha(alphas[i]), fmt.Sprintf("%s %d%%", s.Name, s.Plays*100/total), p.text)
					}
					if rest > 0 {
						weekLegend(c, p.text.Alpha(0.12), fmt.Sprintf("其他 %d%%", rest*100/total), p.text)
					}
				})
				ui.Text(c, "只算在 Cloudy 里听的").FontSize(11.5).TextColor(p.muted)
			})
		}
		if streak >= 2 {
			a.weekFact(c, "streak", iconFlame, "连续听歌", fmt.Sprintf("%d 天", streak), fmt.Sprintf("从 %d.%02d 起每天都听", from.Month(), from.Day()))
		}
	})
}

// weekFact is one of the week's small things.
func (a *app) weekFact(c *ui.Context, key string, icon *ui.SVG, label, value, note string) {
	p := a.pal
	a.weekCard(c.Key(key)).Gap(6).Children(func() {
		ui.Icon(c, icon).Size(18, 18).TextColor(p.accent)
		ui.Text(c, label).FontSize(12.5).FontWeight(600).TextColor(p.muted)
		ui.Text(c, value).FontSize(20).FontWeight(700).MaxLines(1)
		ui.Text(c, note).FontSize(12.5).TextColor(p.muted).MaxLines(2)
	})
}

// allListenPage is all of the listener's listening: NetEase's total and
// all-time ranking, and what Cloudy kept.
func (a *app) allListenPage(c *ui.Context, now time.Time) {
	l := a.ledger()
	a.loadAllListening()
	tracks := make([]netease.Track, len(l.all))
	for i := range l.all {
		tracks[i] = l.all[i].Track
	}
	a.knownPrivileges(tracks)
	days := listenDays(l.plays)
	a.toolRows(c, tracks, true, func(c *ui.Context) {
		a.weekHead(c, "全部听歌记录", "网易云账号的数据 + Cloudy 自己记下的")
		a.allFigures(c, l, days, now)
		a.allCalendar(c, days, l.since, now)
		a.allRanks(c, l, tracks)
		a.weekNote(c, "「累计听过」和「历史最常听」来自网易云账号，含手机上听的；网易云会在第二天剔除 Cloudy 上报的播放，所以这两项基本不含在 Cloudy 里听的。时长、天数和日历是 Cloudy 自己记下的。")
		if len(tracks) > 0 {
			a.weekSection(c, "历史最常听的歌", fmt.Sprintf("网易云「所有时间」排行 · %d 首", len(tracks)))
		}
	})
}

// allFigures is NetEase's total, with the next round number, and the
// ledger's figures.
func (a *app) allFigures(c *ui.Context, l *listenState, days map[int64]int, now time.Time) {
	p := a.pal
	secs, songs := 0, map[string]bool{}
	for i := range l.plays {
		secs += l.plays[i].Secs
		songs[playKey(&l.plays[i].Track)] = true
	}
	var best time.Time
	bestSecs := 0
	for _, w := range listenWeeks(l.plays, now) {
		if s := a.report(w).secs; s > bestSecs {
			best, bestSecs = w, s
		}
	}
	first := now
	if len(l.plays) > 0 {
		first = l.plays[0].at()
	}
	span := int(dayStartOf(now).Sub(dayStartOf(first))/(24*time.Hour)) + 1
	a.weekGrid(c, "all", 300, 2, func() {
		ui.Column(c.Key("hero")).Padding(22, 24).Radius(20).Gap(10).Background(p.accent).TextColor(p.onAccent).Justify(ui.Center).Children(func() {
			ui.Text(c, "这个账号在网易云累计听过").FontSize(13).Opacity(0.85)
			total := l.total.Songs
			if total == 0 {
				ui.Text(c, "正在读取…").FontSize(28).FontWeight(700)
				return
			}
			ui.Row(c).Gap(6).AlignItems(ui.End).Children(func() {
				ui.Text(c, groupDigits(total)).FontSize(48).FontWeight(800).LetterSpacing(-1).FontFeatures("tnum")
				ui.Text(c, "首").FontSize(18).Margin(0, 0, 8, 0)
			})
			next := (total/1000 + 1) * 1000
			ui.Box(c).Height(8).Radius(999).Background(p.onAccent.Alpha(0.22)).Children(func() {
				ui.Box(c).Absolute().Top(0).Left(0).Bottom(0).WidthPercent(float32(total%1000) / 10).Radius(999).Background(p.onAccent)
			})
			line := fmt.Sprintf("再听 %d 首就到 %s 首", next-total, groupDigits(next))
			if l.total.Days > 0 {
				line += fmt.Sprintf(" · 注册第 %s 天", groupDigits(l.total.Days))
			}
			ui.Text(c, line).FontSize(13).Opacity(0.9)
		})
		ui.Grid(c.Key("mine")).Columns(2).GapX(14).GapY(14).Children(func() {
			since := time.UnixMilli(l.since).In(beijing)
			a.weekTile(c, "secs", "Cloudy 记下的时长", weekLength(secs), fmt.Sprintf("%d.%02d.%02d 起逐次记录", since.Year(), since.Month(), since.Day()), nil)
			a.weekTile(c, "count", "播放次数", groupDigits(len(l.plays))+" 次", groupDigits(len(songs))+" 首不同的歌", nil)
			a.weekTile(c, "days", "听歌的天数", fmt.Sprintf("%d / %d 天", len(days), span), fmt.Sprintf("最长连续 %d 天", longestStreak(days)), nil)
			if bestSecs > 0 {
				a.weekTile(c, "best", "听得最多的一周", fmt.Sprintf("%d.%02d 那周", best.Month(), best.Day()), weekLength(bestSecs), nil)
			} else {
				a.weekTile(c, "best", "听得最多的一周", "—", "", nil)
			}
		})
	})
}

// groupDigits writes n with commas, as 10,016.
func groupDigits(n int) string {
	s := fmt.Sprint(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// allCalendar is the last 26 weeks, a column each, a square a day as dark
// as it was listened to; the days before the ledger began are outlined.
func (a *app) allCalendar(c *ui.Context, days map[int64]int, since int64, now time.Time) {
	p := a.pal
	const weeks = 26
	first := weekStartOf(now).AddDate(0, 0, -7*(weeks-1))
	today := dayStartOf(now)
	sinceDay := dayStartOf(time.UnixMilli(since))
	most := 1
	for _, s := range days {
		most = max(most, s)
	}
	a.weekCard(c.Key("calendar")).Margin(0, 0, 14, 0).Children(func() {
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "每天听了多少").FontSize(16).FontWeight(700)
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "少").FontSize(11.5).TextColor(p.muted)
				for _, k := range []float32{0, 0.25, 0.5, 0.75, 1} {
					fill := p.hover
					if k > 0 {
						fill = p.accent.Alpha(0.15 + 0.85*k)
					}
					ui.Box(c).Size(11, 11).Radius(3).Background(fill)
				}
				ui.Text(c, "多").FontSize(11.5).TextColor(p.muted)
			})
		})
		ui.Row(c).Gap(4).Children(func() {
			for w := range weeks {
				ui.Column(c.Key(w)).Grow(1).Basis(0).Gap(4).MaxWidth(20).Children(func() {
					for d := range 7 {
						day := first.AddDate(0, 0, 7*w+d)
						cell := ui.Box(c.Key(d)).FillWidth().AspectRatio(1).Radius(3)
						s := days[day.Unix()]
						tip := fmt.Sprintf("%d.%02d · %s", day.Month(), day.Day(), weekLength(s))
						switch {
						case day.After(today):
						case day.Before(sinceDay) && s == 0:
							cell.Border(1, p.hairline)
							tip = fmt.Sprintf("%d.%02d · 还没开始记录", day.Month(), day.Day())
						case s == 0:
							cell.Background(p.hover)
						default:
							cell.Background(p.accent.Alpha(0.15 + 0.85*float32(s)/float32(most)))
						}
						cell.Tooltip(tip)
					}
				})
			}
		})
		ui.Text(c, "最近 26 周，一列一周，从上到下是周一到周日（北京时间）。").FontSize(11.5).TextColor(p.muted)
	})
}

// allRanks is NetEase's all-time first eight as bars, and the singers its
// hundred add up to.
func (a *app) allRanks(c *ui.Context, l *listenState, tracks []netease.Track) {
	p := a.pal
	if len(l.all) == 0 {
		if l.allLoading {
			a.empty(c, "正在读取网易云的历史排行…")
		}
		return
	}
	a.weekGrid(c, "ranks", 380, 2, func() {
		a.weekCard(c.Key("all-top")).Children(func() {
			a.weekCardTitle(c, "历史最常听", "网易云「所有时间」排行")
			top := max(1, l.all[0].Plays)
			for i := range min(8, len(l.all)) {
				s := &l.all[i]
				b := ui.ButtonBase(c.Key(s.Track.ID)).Row().Gap(12).AlignItems(ui.Center).Padding(3).Radius(8).Label("播放 " + s.Track.Name)
				if b.Hovered() {
					b.Background(p.hover)
				}
				b.Children(func() {
					ui.Text(c, fmt.Sprint(i+1)).Width(18).FontSize(13).FontWeight(700).FontFeatures("tnum").TextColor(p.muted)
					ui.Box(c).Size(36, 36).Radius(8).Shrink(0).ClipY().Children(func() {
						a.coverFill(c, s.Track.Album.PicURL, 100, 8)
					})
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
						ui.Row(c).Gap(8).Justify(ui.SpaceBetween).Children(func() {
							ui.Text(c, s.Track.Name).FontSize(13.5).FontWeight(600).SingleLine().Shrink(1).MinWidth(0)
							ui.Textf(c, "%d 次", s.Plays).FontSize(12).FontFeatures("tnum").TextColor(p.muted).Shrink(0)
						})
						ui.Box(c).Height(4).Radius(999).Background(p.hover).Children(func() {
							ui.Box(c).Absolute().Top(0).Left(0).Bottom(0).WidthPercent(100 * float32(s.Plays) / float32(top)).Radius(999).Background(p.accent)
						})
					})
				})
				if b.DoubleClicked() {
					a.playPicked(tracks, i)
				}
			}
		})
		a.weekCard(c.Key("all-artists")).Children(func() {
			a.weekCardTitle(c, "一直在听的歌手", "按历史排行加起来")
			artists := rankedArtists(l.all)
			for i := range min(8, len(artists)) {
				ar := artists[i]
				a.weekNameRow(c, &ar, func() {
					if ar.ID > 0 {
						a.open(route{page: pageArtist, id: ar.ID})
					}
				})
			}
		})
	})
}

// weekdayShort names t's day of the week in one character, 一 to 日.
func weekdayShort(t time.Time) string {
	return [...]string{"日", "一", "二", "三", "四", "五", "六"}[t.Weekday()]
}
