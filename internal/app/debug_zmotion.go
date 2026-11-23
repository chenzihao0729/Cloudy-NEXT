package app

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/player"
)

// With NM_SCENE=motion, the app goes through what moves, for the frame log
// of MYGO_FRAME_STATS and the memory sampled outside: the sidebar's pages,
// playlists scrolled to their foot, the settings and their categories, the
// queue, the volume, a menu and the account menu, each a few times, then
// the home page left alone a minute. Marks on stderr tell where each part
// begins, with the Go heap. The same scene builds without the motion of
// 1.3.5-motion (sceneFrom), for A/B.
func init() {
	if os.Getenv("NM_SCENE") != "motion" {
		return
	}
	shotHook = func(a *app) {
		a.settings.path = "" // the listener's settings stay as they were
		go motionScene(a)
	}
}

func motionScene(a *app) {
	step := func(d time.Duration, fn func()) {
		time.Sleep(d)
		mygo.RunOnMain(func() { fn(); a.redraw() })
	}
	mark := func(s string) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		var playing bool
		mygo.RunOnMain(func() { playing = a.pl.Playing() })
		println("mark", time.Now().Format("15:04:05.000"), s, "inuse", m.HeapInuse>>20, "idle", m.HeapIdle>>20, "sys", m.Sys>>20, "gc", m.NumGC, "playing", playing)
	}
	// Above other windows: a covered window draws no frames.
	mygo.RunOnMain(func() {
		a.win.SetAlwaysOnTop(true)
		a.win.Show()
		a.win.Focus()
	})
	for range 120 {
		var ready bool
		mygo.RunOnMain(func() { ready = a.profile != nil && len(a.mine) > 0 })
		if ready {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	time.Sleep(3 * time.Second)
	// The same song over and over, on both builds, for the bar to move
	// alike as music plays.
	step(0, func() { a.volume = 0.02; a.pl.SetVolume(0.02); a.pl.SetMode(player.ModeOne) })
	mark("start")
	want := func(part string) bool { return xParts == "" || strings.Contains(xParts, part) }
	// NM_LOOP=n goes through it all n times, for what stays behind.
	loops := 1
	fmt.Sscan(os.Getenv("NM_LOOP"), &loops)
	for loop := range max(loops, 1) {
		if loop > 0 {
			mark(fmt.Sprint("loop", loop))
		}

		if want("pages") {
			pages := []route{{page: pageHome}, {page: pageDaily}, {page: pageCharts}, {page: pageRecent}, {page: pageDownloads}, {page: pageCloud}, {page: pageLocal}, {page: pagePodcasts}}
			for range 3 {
				for _, r := range pages {
					step(0, func() { a.open(r) })
					time.Sleep(900 * time.Millisecond)
				}
			}
		}
		mark("pages")

		if want("lists") {
			var lists []route
			mygo.RunOnMain(func() {
				for _, pl := range a.mine[:min(len(a.mine), 8)] {
					lists = append(lists, route{page: pagePlaylist, id: pl.ID})
				}
			})
			for _, r := range lists {
				step(0, func() { a.open(r) })
				time.Sleep(1500 * time.Millisecond)
				step(0, func() { a.pageScroll.ScrollTo(0, 100000, ui.Smooth) })
				time.Sleep(1200 * time.Millisecond)
			}
			step(0, func() { a.open(route{page: pageHome}) })
			time.Sleep(1500 * time.Millisecond)
		}
		mark("lists")

		if want("settings") {
			for range 5 {
				step(0, func() { sceneFrom(a, "settings"); a.openSettings() })
				time.Sleep(900 * time.Millisecond)
				for cat := settingsCategory(1); cat < min(settingsCategoryCount, 5); cat++ {
					step(0, func() { a.settingsView.tab = cat })
					time.Sleep(500 * time.Millisecond)
				}
				step(0, func() { a.settingsView.open = false })
				time.Sleep(900 * time.Millisecond)
			}
		}
		mark("settings")

		if want("queue") {
			for range 6 {
				step(0, func() { a.more.queue.open = true })
				time.Sleep(900 * time.Millisecond)
				step(0, func() { a.more.queue.open = false })
				time.Sleep(700 * time.Millisecond)
			}
		}
		mark("queue")

		if want("volume") {
			for range 6 {
				step(0, func() { a.volumeOpen = true })
				time.Sleep(700 * time.Millisecond)
				step(0, func() { a.volumeOpen = false })
				time.Sleep(600 * time.Millisecond)
			}
		}
		mark("volume")

		if want("menu") {
			groups := [][]menuAction{{{label: "下一首播放"}, {label: "复制歌名"}, {label: "复制链接"}}, {{label: "下载到本机"}, {label: "添加到歌单"}}}
			for range 6 {
				step(0, func() { a.openMenu(420, 300, 300, false, groups, nil) })
				time.Sleep(800 * time.Millisecond)
				step(0, func() { a.menu.open = false })
				time.Sleep(600 * time.Millisecond)
			}
		}
		mark("menu")

		if want("account") {
			for range 5 {
				step(0, func() { a.accountOpen = true })
				time.Sleep(800 * time.Millisecond)
				step(0, func() { a.accountOpen = false })
				time.Sleep(600 * time.Millisecond)
			}
		}
		mark("account")

		if want("np") {
			for range 4 {
				step(0, func() { a.nowPlaying = true })
				time.Sleep(2500 * time.Millisecond)
				step(0, func() { a.nowPlaying = false })
				time.Sleep(1500 * time.Millisecond)
			}
		}
		mark("np")

	}
	step(0, func() { a.open(route{page: pageHome}) })
	for i := range 6 {
		time.Sleep(10 * time.Second)
		mark(fmt.Sprint("idle", i))
	}
	mark("end")
	mygo.RunOnMain(mygo.App.Quit)
}
