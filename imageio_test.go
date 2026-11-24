package nightwb

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDecodeFrameClippingFlags(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 10, B: 10, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{R: 128, G: 128, B: 128, A: 255})
	img.SetNRGBA(1, 1, color.NRGBA{R: 200, G: 255, B: 200, A: 255})

	f := DecodeFrame(img)
	if !f.ChannelClipped[0][0] {
		t.Fatal("red channel at 255 must be flagged clipped")
	}
	if f.ChannelClipped[0][1] {
		t.Fatal("green channel at 10 must not be flagged clipped")
	}
	if !f.ChannelCrushed[1][0] || !f.ChannelCrushed[1][1] {
		t.Fatal("black pixel channels must be flagged crushed")
	}
	if f.ChannelClipped[2][0] {
		t.Fatal("mid-gray must not be clipped")
	}
	if !f.ChannelClipped[3][1] {
		t.Fatal("green at 255 must be flagged clipped")
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	f := &Frame{
		Width: 4, Height: 4,
		RGB:            make([][3]float64, 16),
		ChannelClipped: make([][3]bool, 16),
		ChannelCrushed: make([][3]bool, 16),
	}
	for i := range f.RGB {
		f.RGB[i] = [3]float64{0.2, 0.5, 0.8}
	}
	af := &AdaptedFrame{RGB: f.RGB, OutOfGamut: make([]bool, 16)}
	var buf bytes.Buffer
	if err := EncodePNG(&buf, af, 4, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(&buf); err != nil {
		t.Fatalf("adapted output is not a valid PNG: %v", err)
	}

	mask := make([]uint16, 16)
	mask[0] = FlagClippedRed | FlagOutOfGamut
	var mbuf bytes.Buffer
	if err := EncodeMaskPNG(&mbuf, mask, 4, 4); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(&mbuf)
	if err != nil {
		t.Fatalf("mask is not a valid PNG: %v", err)
	}
	g16 := decoded.(*image.Gray16)
	if g16.Gray16At(0, 0).Y != mask[0] {
		t.Fatalf("mask bits not preserved: got %d want %d", g16.Gray16At(0, 0).Y, mask[0])
	}
}
