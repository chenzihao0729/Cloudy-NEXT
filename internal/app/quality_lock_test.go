package app

import (
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// What a listener's membership cannot play is locked, and only once NetEase
// has said what they hold (#60).
func TestQualityLockedByMembership(t *testing.T) {
	a := newTestApp()
	a.profile = nil
	if a.levelLocked("exhigh") != "" || a.levelLocked("lossless") == "" || a.levelLocked("jymaster") == "" {
		t.Error("signed out: lossy open, lossless and 超清母带 locked")
	}
	a.profile = &netease.Profile{UserID: 1}
	a.vipKnown = false
	if a.levelLocked("jymaster") == "" {
		t.Error("mastering unlocked before SVIP was confirmed")
	}
	a.vipKnown, a.vip = true, nil
	if a.levelLocked("hires") == "" || a.levelLocked("higher") != "" {
		t.Error("no membership: Hi-Res open or 较高 locked")
	}
	a.vip = []netease.Membership{{Name: "黑胶VIP"}}
	if a.levelLocked("lossless") != "" || a.levelLocked("jymaster") == "" {
		t.Error("黑胶VIP: lossless locked or 超清母带 open")
	}
	a.vip = []netease.Membership{{Name: "黑胶SVIP"}}
	if a.levelLocked("jymaster") != "" || a.levelLocked("hires") != "" {
		t.Error("黑胶SVIP: something locked")
	}
	a.vip = []netease.Membership{{Name: "音乐包"}}
	if a.levelLocked("lossless") == "" {
		t.Error("音乐包 opened lossless")
	}
	for _, act := range a.qualityAction().sub[0] {
		if act.label == "超清母带" && act.locked == "" {
			t.Error("the menu left 超清母带 open to 音乐包")
		}
	}
}

// A persisted mastering preference must not outlive the verified account's
// SVIP, and direct quality actions must obey the same gate as the menu.
func TestMasteringRequiresConfirmedActiveSVIP(t *testing.T) {
	a := newTestApp()
	a.settings.Quality = "jymaster"
	if a.playbackQuality() != "exhigh" {
		t.Fatal("logged-out mastering request")
	}
	a.profile = &netease.Profile{UserID: 1, VIPType: 11}
	a.vip = []netease.Membership{{Name: "黑胶SVIP", Expires: time.Now().Add(time.Hour)}}
	if a.playbackQuality() != "exhigh" {
		t.Fatal("unconfirmed mastering request")
	}
	a.vipKnown = true
	if a.playbackQuality() != "jymaster" {
		t.Fatal("active SVIP mastering blocked")
	}
	if a.preferSource(netease.Track{ID: 1, Fee: 1}) {
		t.Fatal("SVIP mastering preferred plugin")
	}
	a.vip[0].Expires = time.Now().Add(-time.Hour)
	if a.playbackQuality() != "exhigh" {
		t.Fatal("expired mastering request")
	}
	a.settings.Quality = "exhigh"
	a.setQuality("jymaster")
	if a.settings.Quality != "exhigh" {
		t.Fatal("direct action bypassed SVIP gate")
	}
}
