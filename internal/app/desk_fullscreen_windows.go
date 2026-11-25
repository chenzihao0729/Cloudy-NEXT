package app

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	procSHQueryUserNotificationState = syscall.NewLazyDLL("shell32.dll").NewProc("SHQueryUserNotificationState")
	procGetWindowThreadProcessIdDesk = user32.NewProc("GetWindowThreadProcessId")
	procMonitorFromWindowDesk        = user32.NewProc("MonitorFromWindow")
	procIsIconicDesk                 = user32.NewProc("IsIconic")
)

// fullscreenInFront reports whether another app is full screen in front: a
// game, in a full-screen mode of its own or in a window as large as the
// screen, a video played full screen, or a presentation. Windows says so as
// it holds back its notifications; the window in front must also cover its
// screen. Windows says so of the desktop too, which the app's window
// minimized or left behind often hands the front to: the lyrics hid as
// soon as the app did (#67, found by jiangpiguai). The app's own full
// screen is left out.
func fullscreenInFront() bool {
	if procSHQueryUserNotificationState.Find() != nil {
		return false
	}
	var state int32
	if hr, _, _ := procSHQueryUserNotificationState.Call(uintptr(unsafe.Pointer(&state))); hr != 0 {
		return false
	}
	const (
		qunsBusy             = 2 // a full-screen app
		qunsRunningD3DFull   = 3 // a full-screen Direct3D app, a game
		qunsPresentationMode = 4
	)
	if state != qunsBusy && state != qunsRunningD3DFull && state != qunsPresentationMode {
		return false
	}
	hwnd, _, _ := procGetForegroundWnd.Call()
	if hwnd == 0 {
		return false
	}
	var pid uint32
	procGetWindowThreadProcessIdDesk.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if int(pid) == os.Getpid() {
		return false
	}
	if v, _, _ := procIsIconicDesk.Call(hwnd); v != 0 {
		return false
	}
	if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
		return false
	}
	var buf [64]uint16
	procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if shellWindowClass(syscall.UTF16ToString(buf[:])) {
		return false
	}
	var win trayRect
	if r, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&win))); r == 0 {
		return false
	}
	mon, _, _ := procMonitorFromWindowDesk.Call(hwnd, 2 /* nearest */)
	var mi struct {
		size          uint32
		monitor, work trayRect
		flags         uint32
	}
	mi.size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return false
	}
	return coversMonitor(win, mi.monitor)
}

// shellWindowClass reports whether a window of class name is the shell's:
// the desktop, or a taskbar.
func shellWindowClass(name string) bool {
	switch name {
	case "Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd":
		return true
	}
	return false
}

// coversMonitor reports whether a window at win covers all of a monitor at
// mon, give or take a pixel at an edge, as a full-screen one does: a
// window maximized leaves the taskbar clear.
func coversMonitor(win, mon trayRect) bool {
	const slack = 2
	return win.left <= mon.left+slack && win.top <= mon.top+slack &&
		win.right >= mon.right-slack && win.bottom >= mon.bottom-slack
}
