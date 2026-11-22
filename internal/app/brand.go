package app

import (
	"bytes"
	_ "embed"
	"image"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
	"netease-music-mygo/resources"
)

const (
	appProject = "https://github.com/chenzihao0729/Cloudy-NEXT"
)

//go:embed brand/mascot.png
var mascotPNG []byte

func embeddedBitmap(b []byte) func() *ui.Bitmap {
	return sync.OnceValue(func() *ui.Bitmap {
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
			log.Printf("brand: %v", err)
			return nil
		}
		return ui.NewBitmap(img)
	})
}

var (
	mascot    = embeddedBitmap(mascotPNG)
	appIcon   = embeddedBitmap(resources.Icon)
	brandIcon = appIcon
	roundIcon = embeddedBitmap(resources.RoundIcon)
	splashArt = sync.OnceValue(func() *ui.Bitmap {
		src, _, err := image.Decode(bytes.NewReader(resources.RoundIcon))
		if err != nil {
			return mascot()
		}
		// The splash only needs a small image. Downsample once so every
		// rotating frame stays cheap even when the source icon is large.
		size := 128
		dst := image.NewRGBA(image.Rect(0, 0, size, size))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
		return ui.NewBitmap(dst)
	})
)

func splashIcon() *ui.Bitmap {
	return splashArt()
}

func mascotView(c *ui.Context, size float32) ui.Element {
	return ui.Image(c, mascot()).Size(size, size).Fit(ui.Cover)
}

func (a *app) aboutCard(c *ui.Context, k *settingsKit) {
	ink := k.ink
	ui.Row(c).Padding(16, 20).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Image(c, brandIcon()).Size(52, 52).Fit(ui.Cover).Shrink(0)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, mygo.App.Name()+" "+mygo.App.Version()).FontSize(14).FontWeight(700)
			ui.Text(c, "Cloudy-NEXT 由 Zihao C 制作，基于 nya-owo/cloudy 修改，以AGPL-3.0开源。与网易云音乐官方无关").FontSize(12).TextColor(ink.muted).LineHeight(1.4)
		})
		if k.button(c, "project", "项目", iconExternal, false, false).Clicked() {
			openLink(appProject)
		}
	})
}

func openLink(url string) {
	go func() {
		if err := mygo.Shell.OpenExternal(url); err != nil {
			log.Printf("open %s: %v", url, err)
		}
	}()
}

const (
	splashTime = 1150 * time.Millisecond
	splashDisc = 168
)

func (a *app) splash(c *ui.Context) {
	if a.splashed {
		return
	}
	now := c.Now()
	if a.splashAt.IsZero() {
		a.splashAt = now
	}
	t := float32(now.Sub(a.splashAt)) / float32(splashTime)
	if t >= 1 {
		a.splashed = true
		return
	}
	c.AnimationFrame()
	in := 1 - easeRange(1-t, 0.45, 1)
	gone := easeRange(t, 0.68, 1)
	s := lerp(0.72, 1, in) + 0.06*gone
	ui.Box(c.Key("splash")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Center().Background(a.pal.bg).Opacity(1 - gone).Children(func() {
		disc := ui.Box(c.Key("disc")).Size(splashDisc, splashDisc).Radius(999).Center().PassThrough().Background(ui.Hex("#232327")).Border(1, white.Alpha(0.14)).Shadow(0, 16, 44, -10, ui.RGBA(0, 0, 0, 0.5)).Opacity(easeRange(t, 0, 0.3)).Scale(s, s).Rotate(lerp(-150, 0, in))
		disc.Draw(func(p *ui.Painter, r ui.Rect) {
			for i := float32(0); i < 5; i++ {
				inset := 8 + i*6
				p.Stroke(ui.Rect{X: r.X + inset, Y: r.Y + inset, W: r.W - 2*inset, H: r.H - 2*inset}, white.Alpha(0.09), (r.W-2*inset)/2, 1)
			}
		})
		disc.Children(func() { ui.Image(c, splashIcon()).Size(76, 76).Fit(ui.Cover) })
	})
}
