package app

import "testing"

// Full screen in front is a window that covers its monitor, not the
// desktop or a taskbar, which Windows may report so too (#67), nor a window
// maximized, which leaves the taskbar clear.
func TestFullscreenIsAWindowCoveringItsScreen(t *testing.T) {
	mon := trayRect{0, 0, 1920, 1080}
	for _, c := range []struct {
		name string
		win  trayRect
		want bool
	}{
		{"full screen", trayRect{0, 0, 1920, 1080}, true},
		{"borderless, a pixel over", trayRect{-1, -1, 1921, 1081}, true},
		{"maximized", trayRect{-8, -8, 1928, 1040}, false},
		{"a window", trayRect{200, 100, 1400, 900}, false},
		{"on another screen", trayRect{1920, 0, 3840, 1080}, false},
	} {
		if got := coversMonitor(c.win, mon); got != c.want {
			t.Errorf("%s: covers %v, want %v", c.name, got, c.want)
		}
	}
	for _, class := range []string{"Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd"} {
		if !shellWindowClass(class) {
			t.Errorf("%s taken for an app's window", class)
		}
	}
	if shellWindowClass("UnityWndClass") {
		t.Error("a game taken for the shell's")
	}
}
