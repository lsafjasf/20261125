// Command genframes regenerates the deterministic PNG test frames shipped in
// frames/input. It only uses the Go standard library.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	width  = 96
	height = 64
)

func main() {
	outDir := "frames/input"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}
	for frame := 1; frame <= 3; frame++ {
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				img.Set(x, y, pixel(x, y, frame))
			}
		}
		path := filepath.Join(outDir, fmt.Sprintf("frame_%04d.png", frame))
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		f.Close()
		println("wrote", path)
	}
}

func pixel(x, y, frame int) color.Color {
	// Bottom half: smooth luma ramp (dark shadows -> highlights).
	if y >= height/2 {
		t := float64(y-height/2) / float64(height/2-1)
		ramp := uint8(16 + t*220)
		// Slight horizontal modulation so gains/matrices change means too.
		mod := 1.0 + 0.04*math.Sin(float64(x)*0.3)
		v := clamp255(float64(ramp) * mod)
		return color.RGBA{R: v, G: v, B: v, A: 255}
	}

	// Top half: six color bars.
	bars := []color.RGBA{
		{200, 40, 40, 255},
		{40, 200, 70, 255},
		{40, 80, 220, 255},
		{220, 200, 40, 255},
		{200, 40, 200, 255},
		{40, 200, 220, 255},
	}
	barW := width / len(bars)
	idx := x / barW
	if idx >= len(bars) {
		idx = len(bars) - 1
	}
	c := bars[idx]

	// Moving neutral patch makes consecutive frames distinguishable.
	blockX := 8 + frame*22
	if x >= blockX && x < blockX+12 && y >= 8 && y < 20 {
		v := uint8(200 + frame*15)
		return color.RGBA{R: v, G: v, B: v, A: 255}
	}
	_ = y
	return c
}

func clamp255(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(math.Round(v))
}
