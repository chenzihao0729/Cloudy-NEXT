//go:build windows

package app

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	mediaCombase            = syscall.NewLazyDLL("combase.dll")
	mediaGetActivation      = mediaCombase.NewProc("RoGetActivationFactory")
	mediaCreateString       = mediaCombase.NewProc("WindowsCreateString")
	mediaDeleteString       = mediaCombase.NewProc("WindowsDeleteString")
	mediaControlsInteropIID = mediaGUID{0xddb0472d, 0xc911, 0x4a1f, [8]byte{0x86, 0xd9, 0xdc, 0x3d, 0x71, 0xa9, 0x5f, 0x5a}}
	mediaControlsIID        = mediaGUID{0x99fa3ff4, 0x1742, 0x42a6, [8]byte{0x90, 0x2e, 0x08, 0x7d, 0x41, 0xf9, 0x65, 0xec}}
)

type mediaGUID struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

const (
	mediaInteropGetForWindow = 6
	mediaGetDisplayUpdater   = 8
	mediaPutAppMediaID       = 9
	mediaUpdaterUpdate       = 17
)

func setWindowsMediaAppID(hwnd uintptr, appName string) error {
	if hwnd == 0 || appName == "" {
		return nil
	}
	className := mediaHString("Windows.Media.SystemMediaTransportControls")
	if className == 0 {
		return fmt.Errorf("could not create SystemMediaTransportControls class name")
	}
	defer mediaDeleteString.Call(className)
	var factory uintptr
	hr, _, _ := mediaGetActivation.Call(className, uintptr(unsafe.Pointer(&mediaControlsInteropIID)), uintptr(unsafe.Pointer(&factory)))
	if int32(hr) < 0 || factory == 0 {
		return fmt.Errorf("RoGetActivationFactory failed: 0x%08x", uint32(hr))
	}
	defer mediaRelease(factory)

	var controls uintptr
	hr = mediaCall(factory, mediaInteropGetForWindow, hwnd, uintptr(unsafe.Pointer(&mediaControlsIID)), uintptr(unsafe.Pointer(&controls)))
	if int32(hr) < 0 || controls == 0 {
		return fmt.Errorf("GetForWindow failed: 0x%08x", uint32(hr))
	}
	defer mediaRelease(controls)

	var updater uintptr
	hr = mediaCall(controls, mediaGetDisplayUpdater, uintptr(unsafe.Pointer(&updater)))
	if int32(hr) < 0 || updater == 0 {
		return fmt.Errorf("getting media display updater failed: 0x%08x", uint32(hr))
	}
	defer mediaRelease(updater)

	id := mediaHString(appName)
	if id == 0 {
		return fmt.Errorf("could not create media identity string")
	}
	defer mediaDeleteString.Call(id)
	// SMTC exposes the app media ID separately from the current track metadata.
	if hr = mediaCall(updater, mediaPutAppMediaID, id); int32(hr) < 0 {
		return fmt.Errorf("setting app media id failed: 0x%08x", uint32(hr))
	}
	if hr = mediaCall(updater, mediaUpdaterUpdate); int32(hr) < 0 {
		return fmt.Errorf("updating media display failed: 0x%08x", uint32(hr))
	}
	return nil
}

func mediaHString(value string) uintptr {
	units, err := syscall.UTF16FromString(value)
	if err != nil {
		return 0
	}
	var hstring uintptr
	r, _, _ := mediaCreateString.Call(uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)-1), uintptr(unsafe.Pointer(&hstring)))
	if int32(r) < 0 {
		return 0
	}
	return hstring
}

func mediaCall(object uintptr, method uintptr, args ...uintptr) uintptr {
	vtable := *(*uintptr)(unsafe.Pointer(object))
	function := *(*uintptr)(unsafe.Pointer(vtable + method*unsafe.Sizeof(uintptr(0))))
	callArgs := make([]uintptr, 1, len(args)+1)
	callArgs[0] = object
	callArgs = append(callArgs, args...)
	r, _, _ := syscall.SyscallN(function, callArgs...)
	return r
}

func mediaRelease(object uintptr) {
	if object != 0 {
		mediaCall(object, 2)
	}
}
