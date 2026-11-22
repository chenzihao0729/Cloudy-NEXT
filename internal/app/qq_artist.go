package app

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/qqmusic"
	"netease-music-mygo/internal/sourcecatalog"
	"time"
)

var qqArtistSongs = qqmusic.ArtistSongs

type qqArtistData struct {
	profile        qqmusic.ArtistProfile
	profileLoading bool
	profileLoaded  bool
	tab            int
	profileErr     string
	songs          []netease.Track
	total          int
	page           paged
}

func (a *app) loadQQArtist(r route, more bool) {
	if a.qqArtists == nil {
		a.qqArtists = map[string]*qqArtistData{}
	}
	d := a.qqArtists[r.sourceID]
	if d == nil {
		d = &qqArtistData{}
		a.qqArtists[r.sourceID] = d
	}
	if !d.profileLoaded && !d.profileLoading {
		d.profileLoading = true
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			profile, err := qqmusic.SingerProfile(ctx, r.sourceID)
			a.post(func() {
				if a.qqArtists[r.sourceID] != d {
					return
				}
				d.profileLoading = false
				if err != nil {
					d.profileErr = "歌手资料加载失败"
					return
				}
				d.profile = profile
				d.profileLoaded = true
			})
		}()
	}
	if d.page.loading || (!more && d.page.loaded) {
		return
	}
	d.page.loading = true
	d.page.err = ""
	offset := len(d.songs)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		result, err := qqArtistSongs(ctx, r.sourceID, offset)
		a.post(func() {
			if a.qqArtists[r.sourceID] != d {
				return
			}
			d.page.loading = false
			if err != nil {
				d.page.err = "QQ音乐歌手歌曲加载失败"
				return
			}
			for _, song := range result.Songs {
				d.songs = append(d.songs, providerTrack("tx", sourcecatalog.Song{ID: song.MID, MID: song.MID, MediaMID: song.MediaMID, Name: song.Name, Artists: song.Artists, ArtistMIDs: song.ArtistMIDs, Album: song.Album, Duration: song.Duration, CoverURL: song.CoverURL}))
			}
			d.page.loaded = true
			d.total = result.Total
			d.page.more = len(result.Songs) > 0 && len(d.songs) < result.Total
		})
	}()
}

func (a *app) qqArtistPage(c *ui.Context) {
	r := a.route
	d := a.qqArtists[r.sourceID]
	if d == nil {
		return
	}
	header := func(c *ui.Context) { a.qqArtistHeader(c, r, d) }
	if d.tab != 0 {
		a.scrollPage(c, func() {
			header(c)
			if d.profileLoading {
				ui.Text(c, "正在加载…")
				return
			}
			if d.profileErr != "" {
				if ui.Button(c, "重试加载资料").Clicked() {
					d.profileErr = ""
					a.loadQQArtist(r, false)
				}
				return
			}
			if d.tab == 2 {
				text := d.profile.Description
				if text == "" {
					text = "暂无歌手简介"
				}
				ui.Text(c, text).TextColor(a.pal.text).Margin(16, 0)
				return
			}
			ui.Grid(c).Columns(a.columns(c, 156)).GapX(20).GapY(28).Children(func() {
				for _, album := range d.profile.Albums {
					album := album
					cover := qqAlbumCover(album.MID)
					card := ui.ButtonBase(c.Key(album.MID)).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(album.Name)
					card.Children(func() {
						art := ui.Box(c).AspectRatio(1).Radius(14)
						a.landBack(c, art, cover)
						art.Children(func() { a.coverFill(c, cover, 400, 14) })
						ui.Text(c, album.Name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
						ui.Text(c, album.Date).FontSize(12).TextColor(a.pal.muted).SingleLine().Margin(-6, 0, 0, 0)
					})
					if card.Clicked() {
						a.open(route{page: pageQQAlbum, sourceID: album.MID, query: album.Name})
					}
				}
			})
		})
		return
	}
	a.trackRows(c, d.songs, true, func(c *ui.Context) {
		header(c)
	}, func(c *ui.Context) { a.pageFoot(c, &d.page, func() { a.loadQQArtist(r, true) }) })
}

func (a *app) qqArtistHeader(c *ui.Context, r route, d *qqArtistData) {
	p := a.pal
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, "https://y.gtimg.cn/music/photo_new/T001R300x300M000"+r.sourceID+".jpg", 600, artistPhotoSize/2)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "歌手").FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, r.query).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			ui.Text(c, fmt.Sprintf("QQ音乐 · %d 首歌曲 · %d 张专辑", d.total, len(d.profile.Albums))).FontSize(13).TextColor(p.muted)
			a.introLine(c, r.query+" 的介绍", d.profile.Description, 0)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					a.playList(d.songs, false)
				}
				if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					a.playList(d.songs, true)
				}
			})
		})
	})
	ui.Row(c).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
		ui.Text(c, [...]string{"歌曲", "专辑", "简介"}[d.tab]).FontSize(19).FontWeight(700).LetterSpacing(-0.3)
		ui.Spacer(c)
		a.segmented(c.Key("qq-artist-tabs"), &d.tab, "歌曲", "专辑", "简介")
	})
}
