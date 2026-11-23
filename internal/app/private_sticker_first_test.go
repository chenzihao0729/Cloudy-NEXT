package app

import (
	"testing"

	"netease-music-mygo/internal/netease"
)

// The picker opens on the first pack with stickers: an empty favorites pack
// is skipped once its page says so, but not while it has not loaded.
func TestPrivateStickerPickerSkipsEmptyFirstGroup(t *testing.T) {
	p := privateStickerPicker{groups: []netease.PrivateStickerGroup{{ID: "1"}, {ID: "2"}}}
	if got := p.firstGroup(); got != "1" {
		t.Fatalf("before any page loaded it opened on %q", got)
	}
	p.cachePage("1", netease.PrivateStickerPage{})
	if got := p.firstGroup(); got != "2" {
		t.Fatalf("an empty favorites pack was still first: %q", got)
	}
	p.cachePage("1", netease.PrivateStickerPage{Cursor: "next"})
	if got := p.firstGroup(); got != "1" {
		t.Fatalf("a pack with more pages was skipped: %q", got)
	}
	p.cachePage("1", netease.PrivateStickerPage{})
	p.cachePage("2", netease.PrivateStickerPage{})
	if got := p.firstGroup(); got != "1" {
		t.Fatalf("with every pack empty it opened on %q", got)
	}
}
