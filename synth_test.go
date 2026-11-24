package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// writePNG renders an *image.RGBA frame to dir/name.png.
func writePNG(t *testing.T, dir, name string, img *image.RGBA) {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// baseFrame builds a 128x128 frame: left half is a dark smooth gradient
// (luma ~20..50, the banding-prone region), right half a bright mid-tone.
func baseFrame() *image.RGBA {
	const size = 128
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b uint8
			if x < size/2 {
				v := uint8(18 + x/4) // 18..33 across the dark half
				r, g, b = v, v, v
			} else {
				v := uint8(120 + (x-size/2)/3)
				r, g, b = v, v, v
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

// withBlockArtifacts raises luma on 8px transform-block boundaries in the
// bright half, imitating block-based coding without touching dark gradients.
func withBlockArtifacts(src *image.RGBA) *image.RGBA {
	out := src
	b := out.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if x < b.Max.X/2 {
				continue
			}
			if x%blockSize == 0 || y%blockSize == 0 {
				c := out.RGBAAt(x, y)
				c.R = clamp8Int(int(c.R) + 12)
				c.G = clamp8Int(int(c.G) + 12)
				c.B = clamp8Int(int(c.B) + 12)
				out.SetRGBA(x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: 255})
			}
		}
	}
	return out
}

// withBanding posterizes the dark smooth half into wider luma steps and adds
// a small chroma drift — the signature of a color-conversion/bit-depth issue.
func withBanding(src *image.RGBA) *image.RGBA {
	out := image.NewRGBA(src.Bounds())
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := src.RGBAAt(x, y)
			if x < b.Max.X/2 {
				v := c.R
				step := uint8(bandStepMin * 2)
				v = (v / step) * step
				c.R = v
				c.G = v
				c.B = v
				c.B = clamp8Int(int(c.B) + 3) // blue-ish chroma drift in shadows
			}
			out.SetRGBA(x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: 255})
		}
	}
	return out
}

// withMildNoise applies small symmetric luma jitter, mimicking a parameter
// tweak that barely moves mean PSNR but produces no structured artifact.
func withMildNoise(src *image.RGBA) *image.RGBA {
	out := image.NewRGBA(src.Bounds())
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := src.RGBAAt(x, y)
			d := int8(((x*7 + y*13) % 5) - 2) // -2..2, deterministic
			c.R = clamp8Int(int(c.R) + int(d))
			c.G = clamp8Int(int(c.G) + int(d))
			c.B = clamp8Int(int(c.B) + int(d))
			out.SetRGBA(x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: 255})
		}
	}
	return out
}

func clamp8Int(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
