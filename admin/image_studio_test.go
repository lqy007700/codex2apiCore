package admin

import (
	"bytes"

	"encoding/base64"

	"image"
	"image/color"
	"image/png"

	"testing"
)

// An exact requested size is still requested exactly, but it is fitted
// inside the box rather than cropped to fill it.

// Two outputs so a failure on one cannot be confused with an empty batch.

func squarePNG(t *testing.T, side int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADUlEQVR4nGNgYPgPAAEDAQC0wS7EAAAAAElFTkSuQmCC")
	if err != nil {
		t.Fatalf("decode tiny png: %v", err)
	}
	return data
}
