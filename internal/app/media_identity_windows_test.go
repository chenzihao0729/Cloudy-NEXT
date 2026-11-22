//go:build windows

package app

import (
	"syscall"
	"testing"
	"unsafe"
)

// COM dispatch preserves the interface address, method slot and release call.
func TestMediaInterfaceDispatch(t *testing.T) {
	var table [18]uintptr
	object := &mediaObject{vtable: &table}
	var gotThis, gotValue uintptr
	released := false
	table[mediaPutAppMediaID] = syscall.NewCallback(func(this, value uintptr) uintptr {
		gotThis, gotValue = this, value
		return 7
	})
	table[2] = syscall.NewCallback(func(this uintptr) uintptr {
		released = this == uintptr(unsafe.Pointer(object))
		return 0
	})
	if got := mediaCall(object, mediaPutAppMediaID, 42); got != 7 {
		t.Fatalf("method returned %d", got)
	}
	if gotThis != uintptr(unsafe.Pointer(object)) || gotValue != 42 {
		t.Fatalf("wrong COM arguments: %x, %d", gotThis, gotValue)
	}
	mediaRelease(object)
	mediaRelease(nil)
	if !released {
		t.Fatal("COM interface was not released")
	}
}
