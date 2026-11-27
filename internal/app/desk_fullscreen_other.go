//go:build !windows

package app

// fullscreenInFront reports whether another app is full screen in front:
// asked of Windows only.
func fullscreenInFront(desk uintptr) bool { return false }
