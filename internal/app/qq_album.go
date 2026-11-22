package app

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/qqmusic"
	"netease-music-mygo/internal/sourcecatalog"
)

var qqAlbumDetail = qqmusic.AlbumDetail
var openSourceAlbum = (*app).openSourceAlbumForSong

func (a *app) trackAlbumLink(c *ui.Context, tr *netease.Track, color, hover ui.Color) {
	if tr.Album.ID > 0 {
		a.link(c, tr.Album.Name, color, hover, route{page: pageAlbum, id: tr.Album.ID})
		return
	}
	if tr.Source == nil || tr.Album.Name == "" {
		ui.Text(c, tr.Album.Name)
		return
	}
	for _, ref := range searchTrackSources(*tr) {
		if ref.Platform == "tx" && ref.AlbumMID != "" {
			a.link(c, tr.Album.Name, color, hover, route{page: pageQQAlbum, sourceID: ref.AlbumMID, query: tr.Album.Name})
			return
		}
	}
	song := *tr
	a.artistActionLink(c, tr.Album.Name, color, hover, func() { openSourceAlbum(a, song) })
}

func (a *app) openSourceAlbumForSong(song netease.Track) {
	for _, ref := range searchTrackSources(song) {
		if ref.Platform == "tx" && ref.AlbumMID != "" {
			a.openPage(route{page: pageQQAlbum, sourceID: ref.AlbumMID, query: song.Album.Name})
			return
		}
	}
	from := a.route
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rows, err := sourcecatalog.Search(ctx, "tx", song.Name+" "+song.ArtistNames(), 20)
		mid := ""
		if err == nil {
			for _, row := range rows {
				if row.AlbumMID != "" && sameSearchRecording(song, providerTrack("tx", row)) {
					mid = row.AlbumMID
					break
				}
			}
		}
		a.post(func() {
			if a.route != from {
				return
			}
			if mid == "" {
				a.toast = "未找到匹配的专辑详情"
				return
			}
			a.openPage(route{page: pageQQAlbum, sourceID: mid, query: song.Album.Name})
		})
	}()
}

type qqAlbumData struct {
	album   *qqmusic.Album
	songs   []netease.Track
	loading bool
	err     string
}

func qqAlbumCover(mid string) string {
	return "https://y.gtimg.cn/music/photo_new/T002R300x300M000" + mid + ".jpg"
}

func (a *app) loadQQAlbum(r route) {
	if a.qqAlbums == nil {
		a.qqAlbums = map[string]*qqAlbumData{}
	}
	d := a.qqAlbums[r.sourceID]
	if d == nil {
		d = &qqAlbumData{}
		a.qqAlbums[r.sourceID] = d
	}
	if d.album != nil || d.loading {
		return
	}
	d.loading, d.err = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		album, err := qqAlbumDetail(ctx, r.sourceID)
		a.post(func() {
			if a.qqAlbums[r.sourceID] != d {
				return
			}
			d.loading = false
			if err != nil {
				d.err = "专辑加载失败"
				return
			}
			d.album = &album
			for _, song := range album.Songs {
				d.songs = append(d.songs, providerTrack("tx", sourcecatalog.Song{ID: song.MID, MID: song.MID, MediaMID: song.MediaMID, Name: song.Name, Artists: song.Artists, ArtistMIDs: song.ArtistMIDs, Album: song.Album, Duration: song.Duration, CoverURL: song.CoverURL}))
			}
		})
	}()
}

func (a *app) qqAlbumPage(c *ui.Context) {
	r := a.route
	d := a.qqAlbums[r.sourceID]
	if d == nil {
		return
	}
	if d.album == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadQQAlbum(r) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, false, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	al := d.album
	a.toolRows(c, d.songs, false, func(c *ui.Context) {
		p := a.pal
		ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
			a.headCover(c, qqAlbumCover(r.sourceID))
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
				ui.Text(c, "专辑").FontSize(12).FontWeight(600).TextColor(p.accent)
				ui.Text(c, al.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
				ui.RichText(c).FontSize(13).TextColor(p.muted).Children(func() {
					for i, name := range al.Artists {
						if i > 0 {
							ui.Text(c, ", ")
						}
						if i < len(al.ArtistMIDs) && al.ArtistMIDs[i] != "" {
							a.link(c.Key(i), name, p.muted, p.text, route{page: pageQQArtist, sourceID: al.ArtistMIDs[i], query: name})
						} else {
							ui.Text(c, name)
						}
					}
					ui.Text(c, fmt.Sprintf(" · %s · %d 首", al.Date, len(d.songs)))
				})
				a.introLine(c, "专辑介绍", al.Description, 0)
				ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
					if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
						a.playList(d.songs, false)
					}
				})
			})
		})
	})
}
