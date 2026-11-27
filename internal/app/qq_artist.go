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

type sourceArtistAlbum struct {
	id, name, cover, date string
}

type sourceArtistProfile struct {
	name, description, photo string
	albums                   []sourceArtistAlbum
}

type sourceArtistPage struct {
	songs []netease.Track
	total int
	photo string
}

var loadSourceArtistProfile = func(ctx context.Context, platform, id string) (sourceArtistProfile, error) {
	if platform == "kw" || platform == "kg" || platform == "mg" {
		var profile sourcecatalog.ArtistProfile
		var err error
		switch platform {
		case "kw":
			profile, err = sourcecatalog.KuwoArtist(ctx, id)
		case "kg":
			profile, err = sourcecatalog.KugouArtist(ctx, id)
		case "mg":
			profile, err = sourcecatalog.MiguArtist(ctx, id)
		}
		out := sourceArtistProfile{name: profile.Name, description: profile.Description, photo: profile.PhotoURL}
		for _, album := range profile.Albums {
			out.albums = append(out.albums, sourceArtistAlbum{id: album.ID, name: album.Name, cover: album.CoverURL, date: album.Date})
		}
		return out, err
	}
	profile, err := qqmusic.SingerProfile(ctx, id)
	out := sourceArtistProfile{description: profile.Description, photo: "https://y.gtimg.cn/music/photo_new/T001R300x300M000" + id + ".jpg"}
	for _, album := range profile.Albums {
		out.albums = append(out.albums, sourceArtistAlbum{id: album.MID, name: album.Name, date: album.Date, cover: qqAlbumCover(album.MID)})
	}
	return out, err
}

var loadSourceArtistSongs = func(ctx context.Context, platform, id string, offset int) (sourceArtistPage, error) {
	if platform == "kw" || platform == "kg" || platform == "mg" {
		var page sourcecatalog.ArtistPage
		var err error
		switch platform {
		case "kw":
			page, err = sourcecatalog.KuwoArtistSongs(ctx, id, offset)
		case "kg":
			page, err = sourcecatalog.KugouArtistSongs(ctx, id, offset)
		case "mg":
			page, err = sourcecatalog.MiguArtistSongs(ctx, id, offset)
		}
		out := sourceArtistPage{total: page.Total, photo: page.PhotoURL}
		for _, song := range page.Songs {
			out.songs = append(out.songs, providerTrack(platform, song))
		}
		if out.total == 0 {
			out.total = offset + len(out.songs)
		}
		return out, err
	}
	page, err := qqArtistSongs(ctx, id, offset)
	out := sourceArtistPage{total: page.Total}
	for _, song := range page.Songs {
		out.songs = append(out.songs, providerTrack("tx", sourcecatalog.Song{ID: song.MID, MID: song.MID, MediaMID: song.MediaMID, Name: song.Name, Artists: song.Artists, ArtistMIDs: song.ArtistMIDs, Album: song.Album, AlbumMID: song.AlbumMID, Duration: song.Duration, CoverURL: song.CoverURL}))
	}
	return out, err
}

func sourceArtistPlatform(r route) string {
	if r.platform == "" {
		return "tx"
	}
	return r.platform
}

func sourceArtistKey(r route) string {
	if platform := sourceArtistPlatform(r); platform != "tx" {
		return platform + ":" + r.sourceID
	}
	return r.sourceID
}

type qqArtistData struct {
	profile        sourceArtistProfile
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
	key := sourceArtistKey(r)
	d := a.qqArtists[key]
	if d == nil {
		d = &qqArtistData{}
		a.qqArtists[key] = d
	}
	if !d.profileLoaded && !d.profileLoading {
		d.profileLoading = true
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			profile, err := loadSourceArtistProfile(ctx, sourceArtistPlatform(r), r.sourceID)
			a.post(func() {
				if a.qqArtists[key] != d {
					return
				}
				d.profileLoading = false
				if err != nil {
					d.profileErr = "歌手资料加载失败"
					return
				}
				// Migu returns the portrait with its song page. Preserve it when
				// that faster reply arrived before the profile description.
				if profile.photo == "" {
					profile.photo = d.profile.photo
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
		result, err := loadSourceArtistSongs(ctx, sourceArtistPlatform(r), r.sourceID, offset)
		a.post(func() {
			if a.qqArtists[key] != d {
				return
			}
			d.page.loading = false
			if err != nil {
				d.page.err = platformName(sourceArtistPlatform(r)) + "歌手歌曲加载失败"
				return
			}
			d.songs = append(d.songs, result.songs...)
			if d.profile.photo == "" && result.photo != "" {
				d.profile.photo = result.photo
			}
			d.page.loaded = true
			d.total = result.total
			d.page.more = len(result.songs) > 0 && len(d.songs) < result.total
		})
	}()
}

func (a *app) qqArtistPage(c *ui.Context) {
	r := a.route
	d := a.qqArtists[sourceArtistKey(r)]
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
				text := d.profile.description
				if text == "" {
					text = "暂无歌手简介"
				}
				ui.Text(c, text).TextColor(a.pal.text).Margin(16, 0)
				return
			}
			ui.Grid(c).Columns(a.columns(c, 156)).GapX(20).GapY(28).Children(func() {
				for _, album := range d.profile.albums {
					album := album
					content := func(card ui.Element) {
						card.Children(func() {
							art := ui.Box(c).AspectRatio(1).Radius(14)
							a.landBack(c, art, album.cover)
							art.Children(func() { a.coverFill(c, album.cover, 400, 14) })
							ui.Text(c, album.name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
							ui.Text(c, album.date).FontSize(12).TextColor(a.pal.muted).SingleLine().Margin(-6, 0, 0, 0)
						})
					}
					if sourceArtistPlatform(r) == "tx" {
						card := ui.ButtonBase(c.Key(album.id)).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(album.name)
						content(card)
						if card.Clicked() {
							a.open(route{page: pageQQAlbum, sourceID: album.id, query: album.name})
						}
					} else {
						content(ui.Column(c.Key(album.id)).Gap(10).AlignItems(ui.Stretch))
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
	platform := sourceArtistPlatform(r)
	provider := platformName(platform)
	name := r.query
	if d.profile.name != "" {
		name = d.profile.name
	}
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, d.profile.photo, 600, artistPhotoSize/2)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "歌手").FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			ui.Text(c, fmt.Sprintf("%s · %d 首歌曲 · %d 张专辑", provider, d.total, len(d.profile.albums))).FontSize(13).TextColor(p.muted)
			a.introLine(c, name+" 的介绍", d.profile.description, 0)
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
