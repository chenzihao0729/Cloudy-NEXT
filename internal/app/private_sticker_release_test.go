package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

// Closing the messages lets go of the stickers they loaded: the cover cache
// no longer counts them, though one still held elsewhere stays found.
func TestPrivateMessagesLetGoOfStickers(t *testing.T) {
	a, _ := privateTestApp(t)
	user := a.dm.contacts[0].User
	a.openPrivateMessages(&user)
	tt := ui.NewTester(a.view, 1180, 780)
	tt.Frame()
	privateShowContents(tt)

	still := netease.PrivateSticker{ID: "1", Group: "1", URL: "https://p1.music.126.net/still.png"}
	moving := netease.PrivateSticker{ID: "2", Group: "1", URL: "https://p1.music.126.net/moving.gif"}
	a.privateStickerArtwork(still, false)
	a.privateStickerArtwork(moving, true)
	held := &artwork{used: time.Now(), size: 1000}
	a.art.add(still.URL, held)
	a.art.add(moving.URL+"#sticker", &artwork{used: time.Now(), size: 5000})
	cover := "https://p1.music.126.net/cover.jpg"
	a.art.add(cover, &artwork{used: time.Now(), size: 300})
	before := a.art.size

	a.requestPrivateClose()
	for range 30 { // the exit animation, frame by frame
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
	}
	if a.dm.shown || a.dm.stickerArt != nil {
		t.Fatalf("the closed panel still kept its sticker keys (shown %v, keys %d)", a.dm.shown, len(a.dm.stickerArt))
	}
	if _, ok := a.art.art[still.URL]; ok {
		t.Fatal("a still sticker stayed in the cache")
	}
	if _, ok := a.art.art[moving.URL+"#sticker"]; ok {
		t.Fatal("an animated sticker stayed in the cache")
	}
	if a.art.art[cover] == nil || a.art.size != before-6000 {
		t.Fatalf("the rest of the cache changed: size %d, want %d", a.art.size, before-6000)
	}
	if a.art.peek(still.URL) != held {
		t.Fatal("a sticker still held elsewhere was not found again")
	}
}
