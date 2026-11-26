package app

import (
	"fmt"
	"log"
	"strings"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The memberships of who is signed in, as NetEase tells them, and the
// marks that show them and the qualities that need one, in the black and
// gold NetEase's vinyl memberships wear.

// vipTag is a small mark: "VIP" or "SVIP" by a quality that needs it, or a
// membership by the listener's name. SVIP takes the gold, VIP the black.
func vipTag(c *ui.Context, text string, svip bool) ui.Element {
	return vipTagSized(c, text, svip, 9.5)
}

// vipTagSized is vipTag at size, its height that of a line of it.
func vipTagSized(c *ui.Context, text string, svip bool, size float32) ui.Element {
	bg, ink := ui.Hex("#2b2622"), ui.Hex("#f0d3a0")
	if svip {
		bg, ink = ui.Hex("#ecca90"), ui.Hex("#2b1f10")
	}
	return ui.Text(c, text).FontSize(size).FontWeight(800).LetterSpacing(0.2).LineHeight(1.25).TextColor(ink).Background(bg).
		Padding(0, size/2).Radius(4).Shrink(0).SingleLine()
}

// levelNeeds is the membership a stream level needs, as NetEase marks it
// in its quality menu: none for the lossy ones.
func levelNeeds(level string) string {
	switch level {
	case "lossless", "hires":
		return "VIP"
	case "jymaster":
		return "SVIP"
	}
	return ""
}

// optionNeeds is what levelNeeds says of the quality a select's option
// names, playing or downloading; "" for any other option.
func optionNeeds(label string) string {
	for _, q := range qualityLevels {
		if q.label == label {
			return levelNeeds(q.level)
		}
	}
	for _, d := range downloadLevels {
		if d.label == label {
			return levelNeeds(d.level)
		}
	}
	return ""
}

// needsTag marks an option that needs a membership.
func needsTag(c *ui.Context, label string) {
	if t := optionNeeds(label); t != "" {
		vipTag(c, t, t == "SVIP")
	}
}

// memberTag marks a membership by the listener's name: its name and level,
// in full, or short where room is short ("SVIP 7").
func memberTag(c *ui.Context, m netease.Membership, short bool) ui.Element {
	name := m.Name
	if short {
		name = strings.TrimPrefix(name, "黑胶")
	}
	if m.Level > 0 {
		name += fmt.Sprintf(" %d", m.Level)
		if !short {
			name = m.Name + fmt.Sprintf(" · Lv.%d", m.Level)
		}
	}
	if short {
		return vipTagSized(c, name, m.SVIP(), 8.5)
	}
	return vipTag(c, name, m.SVIP())
}

// loadMemberships asks NetEase what memberships the listener signed in
// holds, for the account row and the settings: kept for that sign-in only.
func (a *app) loadMemberships(prof *netease.Profile) {
	a.vip = nil
	a.vipKnown = false
	if prof == nil {
		return
	}
	gen, id := a.accountGen, prof.UserID
	go func() {
		ms, err := netease.Memberships(id)
		if err != nil {
			log.Printf("vip info: %v", err)
			return
		}
		a.post(func() {
			if gen == a.accountGen {
				a.vip = ms
				a.vipKnown = true
				a.redraw()
			}
		})
	}()
}

// levelLocked reports why level is of no use to who listens, or "": a
// lossless level needs 黑胶VIP or SVIP and 超清母带 SVIP, so none of them
// signed out. Signed in, it waits for NetEase to say what they hold.
func (a *app) levelLocked(level string) string {
	// A configured source plugin may supply lossless audio independently
	// of NetEase membership; mastering quality remains NetEase-only.
	if (level == "lossless" || level == "hires") && a.source != nil && a.source.isEnabled() {
		return ""
	}
	need := levelNeeds(level)
	if need == "" {
		return ""
	}
	name := "黑胶 VIP"
	if need == "SVIP" {
		name = "黑胶 SVIP"
	}
	if a.profile == nil {
		return "登录并开通" + name + "后才能使用"
	}
	if !a.vipKnown {
		return ""
	}
	vip, svip := false, false
	for _, m := range a.vip {
		svip = svip || m.SVIP()
		vip = vip || m.SVIP() || m.Name == "黑胶VIP"
	}
	if need == "SVIP" && !svip || need == "VIP" && !vip {
		return "开通" + name + "后才能使用"
	}
	return ""
}
