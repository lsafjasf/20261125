package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Frame is an in-memory RGB image whose pixels are tagged with Space.
type Frame struct {
	Name  string
	W, H  int
	Space string
	Pix   []RGB
}

func NewFrame(name string, w, h int, space string) *Frame {
	return &Frame{Name: name, W: w, H: h, Space: space, Pix: make([]RGB, w*h)}
}

func (f *Frame) At(x, y int) RGB { return f.Pix[y*f.W+x] }

func (f *Frame) Set(x, y int, c RGB) { f.Pix[y*f.W+x] = c }

// ReadPNG loads a PNG; it is retained for test/readability and delegates to
// the format-dispatching ReadFrame.
func ReadPNG(path, spaceName string) (*Frame, error) { return ReadFrame(path, spaceName) }

// ReadFrame loads a PNG or JPEG. Encoded bytes are interpreted in spaceName
// (typically sRGB, the usual default for both formats).
func ReadFrame(path, spaceName string) (*Frame, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var img image.Image
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(file)
	default:
		img, err = png.Decode(file)
	}
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	frame := NewFrame(filepath.Base(path), w, h, spaceName)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			// RGBA() returns alpha-premultiplied 16-bit values; recover
			// straight RGB so translucent grader frames do not read as dark.
			if a != 0 && a != 65535 {
				r = r * 65535 / a
				g = g * 65535 / a
				b = b * 65535 / a
			}
			frame.Set(x, y, RGB{
				math.Min(1, float64(r)/65535.0),
				math.Min(1, float64(g)/65535.0),
				math.Min(1, float64(b)/65535.0),
			})
		}
	}
	return frame, nil
}

// WritePNG clamps to [0,1], quantizes to 8 bits and writes an opaque PNG.
func WritePNG(path string, f *Frame) error {
	img := image.NewRGBA(image.Rect(0, 0, f.W, f.H))
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			px := f.At(x, y)
			img.SetRGBA(x, y, color.RGBA{
				R: clampByte(px[0]),
				G: clampByte(px[1]),
				B: clampByte(px[2]),
				A: 255,
			})
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func clampByte(v float64) uint8 {
	v = math.Round(math.Min(1, math.Max(0, v)) * 255)
	return uint8(v)
}

// naturalLess compares strings where digit runs compare numerically, so
// frame_2.png sorts before frame_10.png.
func naturalLess(a, b string) bool {
	ia, ib := 0, 0
	for ia < len(a) && ib < len(b) {
		ca, cb := a[ia], b[ib]
		da, db := ca >= '0' && ca <= '9', cb >= '0' && cb <= '9'
		switch {
		case da && db:
			ja := ia
			for ja < len(a) && a[ja] >= '0' && a[ja] <= '9' {
				ja++
			}
			jb := ib
			for jb < len(b) && b[jb] >= '0' && b[jb] <= '9' {
				jb++
			}
			na, _ := strconv.Atoi(a[ia:ja])
			nb, _ := strconv.Atoi(b[ib:jb])
			if na != nb {
				return na < nb
			}
			ia, ib = ja, jb
		default:
			if ca != cb {
				return ca < cb
			}
			ia++
			ib++
		}
	}
	return len(a) < len(b)
}

// ListFrames resolves a directory or glob pattern to naturally sorted PNG paths.
func ListFrames(target string) ([]string, error) {
	if target == "" {
		target = filepath.Join("frames", "input")
	}
	info, err := os.Stat(target)
	if err == nil && info.IsDir() {
		entries, err := os.ReadDir(target)
		if err != nil {
			return nil, err
		}
		var paths []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if isImageName(e.Name()) {
				paths = append(paths, filepath.Join(target, e.Name()))
			}
		}
		sort.Slice(paths, func(i, j int) bool {
			return naturalLess(filepath.Base(paths[i]), filepath.Base(paths[j]))
		})
		return paths, nil
	}
	matches, err := filepath.Glob(target)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, m := range matches {
		if isImageName(m) {
			paths = append(paths, m)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		return naturalLess(filepath.Base(paths[i]), filepath.Base(paths[j]))
	})
	return paths, nil
}

func isImageName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg":
		return true
	}
	return false
}
