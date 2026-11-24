package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The comments of Now Playing have no tab of their own: the comments
// button by the heart brings them out from the right in a drawer of
// glass, over whatever pane shows, and the lyrics beside the cover make
// room for it where the window is wide enough; the button again, or the
// cross on the drawer, puts them away.

// npDrawerW is how wide the drawer is, at most.
const npDrawerW = 404

// npDrawerMove is how the drawer comes out and goes back.
var npDrawerMove = ui.Animation{Spring: ui.SpringOptions{Duration: 480 * time.Millisecond, Bounce: 0.06}}

// npDrawerWidth is how wide the drawer is in a view w wide.
func npDrawerWidth(w float32) float32 {
	return min(npDrawerW, max(300, w*0.36))
}

// npDrawerRoom is how much more room the pane beside the cover leaves at
// its right in a view w wide, the drawer out, to stay clear of it: none in
// a window too narrow to spare it, where the drawer goes over the pane.
func npDrawerRoom(w float32) float32 {
	if w < 1100 {
		return 0
	}
	return max(0, npDrawerWidth(w)+48-npSideX(w))
}

// commentDrawer is the drawer of tr's comments, as far out as a.npDrawer
// says, in a view w wide.
func (a *app) commentDrawer(c *ui.Context, tr *netease.Track, w float32) {
	k := a.npDrawer
	if k <= 0.001 {
		return
	}
	dw := npDrawerWidth(w)
	drawer := ui.Column(c.Key("comment-drawer")).Absolute().Top(topBar(c)+4).Bottom(16).Width(dw).
		Right(16-(1-k)*(dw+40)).Padding(20, 14, 0, 20).Opacity(clamp01(k * 1.6)).Label("评论").TextColor(a.cmInk())
	a.paneSurfaceAt(drawer, 26, false, dw, sceneMenu)
	if !a.npComments {
		// Going back, it takes no clicks.
		drawer.PassThrough()
	}
	drawer.Children(func() {
		// The count rolls as it changes, from one song's to the next, and
		// holds the last song's while the next one's loads.
		n, text := a.commentCount(), ""
		if n > 0 {
			text = " " + count(n)
		}
		ui.Row(c.Key("comment-drawer-head")).Label(a.commentsTab()).Gap(4).AlignItems(ui.Center).Padding(0, 6, 6, 0).Children(func() {
			ui.Text(c, "评论").FontSize(18).FontWeight(800).TextColor(a.cmInk())
			rollCount(c, &a.tabCount, n, text, a.cmInk().Alpha(0.55))
			ui.Spacer(c)
			if iconButton(c.Key("close-comments"), iconX, 16, a.cmInk().Alpha(0.75), "收起评论").Clicked() {
				a.npComments = false
			}
		})
		ui.Column(c.Key("comment-drawer-body")).Grow(1).MinHeight(0).Children(func() {
			a.commentsPane(c, tr)
		})
	})
}

// cmInk is the ink of the comments, on the drawer: a pane that takes the
// theme, dark ink in a light one (the user, 1.3.7-test.6: it stayed dark).
func (a *app) cmInk() ui.Color { return a.inkOn(false).text }

// cmShimmer is the skeleton of comments loading, in the drawer's ink.
func (a *app) cmShimmer(c *ui.Context) shimmer {
	ink := a.cmInk()
	return shimmer{base: ink.Alpha(0.1), sheen: ink.Alpha(0.2)}.in(c)
}
