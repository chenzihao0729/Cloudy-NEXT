package resources

import (
	"bytes"
	"image/png"
	"testing"
)

// The tray source is scaled to the native 16-point, 2x image size.
func TestTrayIconUsesNativeSize(t *testing.T) {
	config, err := png.DecodeConfig(bytes.NewReader(TrayIcon))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 32 || config.Height != 32 {
		t.Fatalf("tray icon size = %dx%d, want 32x32", config.Width, config.Height)
	}
}
