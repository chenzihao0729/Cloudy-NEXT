package app

import (
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var (
	shcore               = syscall.NewLazyDLL("shcore.dll")
	procGetDpiForMonitor = shcore.NewProc("GetDpiForMonitor")
	procSetWindowPosTray = user32.NewProc("SetWindowPos")
)

// trayPlaceAtCursor places the tray menu's window hwnd beside the cursor on
// the monitor under it, in that monitor's own pixels, and reports where the
// click falls on the card for its grow-in origin.
//
// The DIPs mygo's Screen and SetPosition speak are each monitor's pixels over
// its own scale, so with two monitors at different scales their ranges can
// overlap or leave gaps: the menu was measured against one monitor and placed
// on another, half off the screen (issue #73). Pixels have no such gaps.
func trayPlaceAtCursor(hwnd uintptr) (ui.Point, bool) {
	if hwnd == 0 {
		return ui.Point{}, false
	}
	pt := cursorPos()
	mon, _, _ := procMonitorFromPointTray.Call(uintptr(*(*int64)(unsafe.Pointer(&pt))), 2 /* nearest */)
	if mon == 0 {
		return ui.Point{}, false
	}
	var mi struct {
		size          uint32
		monitor, work trayRect
		flags         uint32
	}
	mi.size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return ui.Point{}, false
	}
	dpi := uint32(96)
	if procGetDpiForMonitor.Find() == nil {
		var dx, dy uint32
		if r, _, _ := procGetDpiForMonitor.Call(mon, 0 /* MDT_EFFECTIVE_DPI */, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r == 0 && dx > 0 {
			dpi = dx
		}
	}
	m := mi.monitor
	click, monDIP := trayLocal(pt.x-m.left, pt.y-m.top, m.right-m.left, m.bottom-m.top, int(dpi))
	pos := trayPopupPos(click, mygo.Rectangle{Width: trayPopupWidth, Height: trayPopupHeight}, monDIP)
	x, y := m.left+int32(trayPx(pos.X, int(dpi))), m.top+int32(trayPx(pos.Y, int(dpi)))
	w, h := trayPx(trayPopupWidth, int(dpi)), trayPx(trayPopupHeight, int(dpi))
	place := func() {
		procSetWindowPosTray.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x0004|0x0010 /* NOZORDER|NOACTIVATE */)
	}
	place()
	// Moved onto a monitor of another scale, the window took Windows'
	// suggested rectangle for it: put it back exactly where it goes.
	place()
	return trayPopupOrigin(click, pos), true
}
