package main

import (
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
)

// Frame holds per-pixel planar YCbCr data plus the original RGB values.
// All planes use 0..255 range so every metric is integer/deterministic.
type Frame struct {
	Name string
	W, H int
	R    []uint8
	G    []uint8
	B    []uint8
	Y    []uint8
	Cb   []uint8
	Cr   []uint8
}

func loadFrame(path string) (*Frame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	fr := &Frame{
		Name: filepath.Base(path),
		W:    w,
		H:    h,
		R:    make([]uint8, w*h),
		G:    make([]uint8, w*h),
		B:    make([]uint8, w*h),
		Y:    make([]uint8, w*h),
		Cb:   make([]uint8, w*h),
		Cr:   make([]uint8, w*h),
	}

	i := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b := rgbAt(img, x, y)
			fr.R[i], fr.G[i], fr.B[i] = r, g, b
			yy, cb, cr := rgbToYCbCr(r, g, b)
			fr.Y[i], fr.Cb[i], fr.Cr[i] = yy, cb, cr
			i++
		}
	}
	return fr, nil
}

func rgbAt(img image.Image, x, y int) (uint8, uint8, uint8) {
	c := img.At(x, y)
	switch v := c.(type) {
	case color.YCbCr:
		r, g, b := color.YCbCrToRGB(v.Y, v.Cb, v.Cr)
		return r, g, b
	default:
		r, g, b, _ := c.RGBA()
		return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
	}
}

// rgbToYCbCr uses full-range BT.709, matching typical SDR encode pipelines.
func rgbToYCbCr(r, g, b uint8) (uint8, uint8, uint8) {
	rf := float64(r)
	gf := float64(g)
	bf := float64(b)
	y := 0.2126*rf + 0.7152*gf + 0.0722*bf
	cb := (bf-y)*0.5389 + 128.0
	cr := (rf-y)*0.6350 + 128.0
	return clamp8(y), clamp8(cb), clamp8(cr)
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// listFrames returns supported image files under dir in natural sort order.
// Natural ordering makes frame 2 sort before frame 10, so pair alignment is
// stable across reruns without relying on filesystem iteration order.
func listFrames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch filepath.Ext(e.Name()) {
		case ".png", ".jpg", ".jpeg", ".PNG", ".JPG", ".JPEG":
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		return naturalLess(filepath.Base(paths[i]), filepath.Base(paths[j]))
	})
	return paths, nil
}

func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ad, bd := isDigit(a[ai]), isDigit(b[bi])
		if ad != bd {
			return ad
		}
		if ad {
			as, bs := ai, bi
			for ai < len(a) && isDigit(a[ai]) {
				ai++
			}
			for bi < len(b) && isDigit(b[bi]) {
				bi++
			}
			na, nb := a[as:ai], b[bs:bi]
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if a[ai] != b[bi] {
			return a[ai] < b[bi]
		}
		ai++
		bi++
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

var pngEncoder = png.Encoder{CompressionLevel: png.BestSpeed}
