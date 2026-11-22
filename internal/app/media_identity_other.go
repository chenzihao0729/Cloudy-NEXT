//go:build !windows

package app

func setWindowsMediaAppID(uintptr, string) error { return nil }
