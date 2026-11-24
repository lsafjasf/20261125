package main

import (
	"bufio"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Frame is a single 8-bit luma image plus presentation metadata.
type Frame struct {
	Y     []byte
	W, H  int
	Index int
	PTS   float64
}

func (f *Frame) At(x, y int) int {
	return int(f.Y[y*f.W+x])
}

// GrayImage renders the luma plane as a standard grayscale image.
type GrayImage struct {
	Pix    []byte
	Stride int
	Rect   image.Rectangle
}

func (g *GrayImage) ColorModel() color.Model { return color.GrayModel }
func (g *GrayImage) Bounds() image.Rectangle { return g.Rect }
func (g *GrayImage) GrayAt(x, y int) color.Gray {
	i := (y-g.Rect.Min.Y)*g.Stride + (x - g.Rect.Min.X)
	return color.Gray{Y: g.Pix[i]}
}
func (g *GrayImage) At(x, y int) color.Color { return g.GrayAt(x, y) }

func grayImage(f *Frame) *GrayImage {
	return &GrayImage{Pix: f.Y, Stride: f.W, Rect: image.Rect(0, 0, f.W, f.H)}
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if err := png.Encode(w, img); err != nil {
		return err
	}
	return w.Flush()
}

// readPNGFrame decodes an 8/16-bit PNG and returns its luma (BT.601) plane.
func readPNGFrame(path string, index int, pts float64) (*Frame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(bufio.NewReader(f))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	y := make([]byte, w*h)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			r, g, bl, _ := img.At(b.Min.X+col, b.Min.Y+row).RGBA()
			y[row*w+col] = byte(((299*r + 587*g + 114*bl) / 1000) >> 8)
		}
	}
	return &Frame{Y: y, W: w, H: h, Index: index, PTS: pts}, nil
}

// readRawYUV420 reads one YUV420p (I420) frame: Y plane followed by U and V.
func readRawYUV420(r *bufio.Reader, w, h, index int, pts float64) (*Frame, error) {
	ySize := w * h
	buf := make([]byte, ySize+(w*h/2))
	if _, err := readFull(r, buf); err != nil {
		return nil, err
	}
	y := make([]byte, ySize)
	copy(y, buf[:ySize])
	return &Frame{Y: y, W: w, H: h, Index: index, PTS: pts}, nil
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// loadPNGSequence loads every *.png in dir in natural filename order.
func loadPNGSequence(dir string, fps float64) ([]*Frame, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".png") {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
	frames := make([]*Frame, 0, len(names))
	for i, name := range names {
		fr, err := readPNGFrame(filepath.Join(dir, name), i, float64(i)/fps)
		if err != nil {
			return nil, err
		}
		frames = append(frames, fr)
	}
	return frames, nil
}

// loadRawYUV reads all frames from a planar I420 file.
func loadRawYUV(path string, w, h int, fps float64) ([]*Frame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 4<<20)
	var frames []*Frame
	for i := 0; ; i++ {
		fr, err := readRawYUV420(r, w, h, i, float64(i)/fps)
		if err != nil {
			break
		}
		frames = append(frames, fr)
	}
	return frames, nil
}

// naturalLess orders "f2.png" before "f10.png".
func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ad, bd := isDigit(a[ai]), isDigit(b[bi])
		if ad != bd {
			return a[ai] < b[bi]
		}
		if ad {
			as, ae := spanDigits(a, ai)
			bs, be := spanDigits(b, bi)
			an, bn := a[as:ae], b[bs:be]
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			ai, bi = ae, be
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
func spanDigits(s string, i int) (int, int) {
	start := i
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return start, i
}
