package resources

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"

	"golang.org/x/image/draw"
)

// Icon is the Cloudy-NEXT app icon shared by the splash screen and the window.
//
//go:embed amicon.png
var Icon []byte

// TrayIcon is Icon at the native 16-point, 2x tray size.
var TrayIcon = makeTrayIcon()

func makeTrayIcon() []byte {
	source, err := png.Decode(bytes.NewReader(Icon))
	if err != nil {
		return Icon
	}
	image := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	draw.CatmullRom.Scale(image, image.Bounds(), source, source.Bounds(), draw.Over, nil)
	var out bytes.Buffer
	if err := png.Encode(&out, image); err != nil {
		return Icon
	}
	return out.Bytes()
}
