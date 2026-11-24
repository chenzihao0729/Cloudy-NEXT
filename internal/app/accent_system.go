package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// accentSystem is settings.Accent following the desktop's accent: the
// color of Windows's Personalization > Colors, read again as it changes.
const accentSystem = "system"

// chosenAccent returns the accent the listener chose as #RRGGBB, the
// desktop's where they follow it, or "" for the app's red.
func (a *app) chosenAccent() string {
	if a.settings.Accent == accentSystem {
		return a.sysAccent
	}
	return a.settings.Accent
}

// readSysAccent notes the desktop's accent, as the app opens and as the
// desktop's theme changes, and whether it asks for opaque surfaces
// (Windows's transparency effects off), which makes the app's own glass
// solid (settings.opaque).
func (a *app) readSysAccent() {
	prefs := mygo.Theme.Preferences()
	was := a.sysAccent
	a.sysAccent = ""
	if c := prefs.Accent; c.A != 0 {
		a.sysAccent = hexOf(ui.RGB(c.R, c.G, c.B))
	}
	if a.sysAccent != was && a.settings.Accent == accentSystem {
		a.redraw()
	}
	if prefs.ReduceTransparency != a.sysOpaque {
		a.sysOpaque = prefs.ReduceTransparency
		a.redraw()
	}
}
