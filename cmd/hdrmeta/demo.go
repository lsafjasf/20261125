package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"hdrmeta/pkg/pq"
)

// generateDemoFrames writes synthetic 16-bit PQ PNGs that reproduce the
// false-positive review cases: a deliberately overbright frame, a pure
// black frame, and a small subtitle-like highlight. One normal frame is
// included so FALL behavior is visible too.
func generateDemoFrames(outDir string, depth int) ([]string, error) {
	const w, h = 128, 128
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	type spec struct {
		name string
		draw func(*image.RGBA64)
	}
	base100 := gray16(pq.NitsToCode(100, depth), depth)
	base200 := gray16(pq.NitsToCode(200, depth), depth)
	hot4000 := gray16(pq.NitsToCode(4000, depth), depth)

	specs := []spec{
		{
			name: "frame01_normal.png",
			draw: func(img *image.RGBA64) {
				fillGray(img, base200)
			},
		},
		{
			name: "frame02_overbright.png",
			draw: func(img *image.RGBA64) {
				fillGray(img, base200)
				// 32x32 block (6.25% of frame) at 4000 nits.
				fillRect(img, 40, 40, 72, 72, hot4000)
			},
		},
		{
			name: "frame03_black.png",
			draw: func(img *image.RGBA64) {
				// leave zero-initialized -> pure black
			},
		},
		{
			name: "frame04_subtitle.png",
			draw: func(img *image.RGBA64) {
				fillGray(img, base100)
				// 2x6 glyph (0.073% of frame) at 4000 nits.
				fillRect(img, 20, 100, 22, 106, hot4000)
			},
		},
	}

	paths := make([]string, 0, len(specs))
	for _, s := range specs {
		img := image.NewRGBA64(image.Rect(0, 0, w, h))
		s.draw(img)
		p := filepath.Join(outDir, s.name)
		if err := writePNG16(p, img); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// gray16 converts a code value at depth to a 16-bit container value by
// left-justification, matching the loader's normalization assumption.
func gray16(code, depth int) uint16 {
	return uint16(code << (16 - depth))
}

func fillGray(img *image.RGBA64, v uint16) {
	c := color.RGBA64{R: v, G: v, B: v, A: 0xffff}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetRGBA64(x, y, c)
		}
	}
}

func fillRect(img *image.RGBA64, x0, y0, x1, y1 int, v uint16) {
	c := color.RGBA64{R: v, G: v, B: v, A: 0xffff}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA64(x, y, c)
		}
	}
}

func writePNG16(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return nil
}
