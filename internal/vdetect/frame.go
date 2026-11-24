package vdetect

import (
	"bufio"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Frame is one decoded luma frame with optional RGB color.
type Frame struct {
	Index int
	PTS   float64
	W     int
	H     int
	Y     []uint8
	R     []uint8
	G     []uint8
	B     []uint8
}

// LoadImage decodes a PNG/JPEG file into a Frame.
func LoadImage(path string) (*Frame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(bufio.NewReader(f))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return FrameFromImage(img), nil
}

// FrameFromImage converts a standard image to contiguous luma/RGB planes.
func FrameFromImage(src image.Image) *Frame {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), src, bounds.Min, draw.Src)

	frame := &Frame{W: w, H: h, Y: make([]uint8, w*h), R: make([]uint8, w*h), G: make([]uint8, w*h), B: make([]uint8, w*h)}
	for i := 0; i < w*h; i++ {
		o := i * 4
		r, g, b := rgba.Pix[o], rgba.Pix[o+1], rgba.Pix[o+2]
		frame.R[i], frame.G[i], frame.B[i] = r, g, b
		// BT.601 fixed point, close to ffmpeg's limited/full gray conversion.
		y := (19595*int(r) + 38470*int(g) + 7471*int(b) + 1<<15) >> 16
		frame.Y[i] = uint8(clamp(float64(y), 0, 255))
	}
	return frame
}

// ReadRawFrame reads one frame from gray, rgb24, rgba, or yuv420p data.
func ReadRawFrame(r io.Reader, index int, opts Options) (*Frame, error) {
	w, h := opts.Width, opts.Height
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("raw input requires positive width and height")
	}
	pix := strings.ToLower(opts.PixelFormat)
	switch pix {
	case "gray", "gray8", "y8":
		y := make([]byte, w*h)
		if _, err := io.ReadFull(r, y); err != nil {
			return nil, err
		}
		pts := 0.0
		if opts.FPS > 0 {
			pts = float64(index) / opts.FPS
		}
		return &Frame{Index: index, PTS: pts, W: w, H: h, Y: y}, nil
	case "rgb24", "rgb":
		buf := make([]byte, w*h*3)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		f := &Frame{Index: index, PTS: float64(index) / opts.FPS, W: w, H: h,
			Y: make([]uint8, w*h), R: make([]uint8, w*h), G: make([]uint8, w*h), B: make([]uint8, w*h)}
		for i := 0; i < w*h; i++ {
			f.R[i], f.G[i], f.B[i] = buf[i*3], buf[i*3+1], buf[i*3+2]
			f.Y[i] = uint8((19595*int(f.R[i]) + 38470*int(f.G[i]) + 7471*int(f.B[i]) + 1<<15) >> 16)
		}
		return f, nil
	case "rgba", "rgba32":
		buf := make([]byte, w*h*4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		f := &Frame{Index: index, PTS: float64(index) / opts.FPS, W: w, H: h,
			Y: make([]uint8, w*h), R: make([]uint8, w*h), G: make([]uint8, w*h), B: make([]uint8, w*h)}
		for i := 0; i < w*h; i++ {
			f.R[i], f.G[i], f.B[i] = buf[i*4], buf[i*4+1], buf[i*4+2]
			f.Y[i] = uint8((19595*int(f.R[i]) + 38470*int(f.G[i]) + 7471*int(f.B[i]) + 1<<15) >> 16)
		}
		return f, nil
	case "yuv420p", "i420":
		y := make([]byte, w*h)
		if _, err := io.ReadFull(r, y); err != nil {
			return nil, err
		}
		cw, ch := (w+1)/2, (h+1)/2
		cb := make([]byte, cw*ch)
		cr := make([]byte, cw*ch)
		if _, err := io.ReadFull(r, cb); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(r, cr); err != nil {
			return nil, err
		}
		f := &Frame{Index: index, PTS: float64(index) / opts.FPS, W: w, H: h, Y: y}
		f.YUVToRGB(cb, cr, cw, ch)
		return f, nil
	default:
		return nil, fmt.Errorf("unsupported raw pixel format %q", opts.PixelFormat)
	}
}

// YUVToRGB populates optional color planes for block-color diagnostics.
func (f *Frame) YUVToRGB(cb, cr []byte, cw, ch int) {
	f.R, f.G, f.B = make([]uint8, f.W*f.H), make([]uint8, f.W*f.H), make([]uint8, f.W*f.H)
	for y := 0; y < f.H; y++ {
		cy := clampInt(y/2, 0, ch-1)
		for x := 0; x < f.W; x++ {
			cx := clampInt(x/2, 0, cw-1)
			ci := cy*cw + cx
			u := int(cb[ci]) - 128
			v := int(cr[ci]) - 128
			yy := int(f.Y[y*f.W+x])
			r := yy + (91881*v)>>16
			g := yy - (22554*u+46802*v)>>16
			b := yy + (116130*u)>>16
			i := y*f.W + x
			f.R[i] = uint8(clampInt(r, 0, 255))
			f.G[i] = uint8(clampInt(g, 0, 255))
			f.B[i] = uint8(clampInt(b, 0, 255))
		}
	}
}

// ListImagePaths expands files/directories/globs into sorted image paths.
func ListImagePaths(inputs []string) ([]string, error) {
	var paths []string
	for _, input := range inputs {
		matches, err := filepath.Glob(input)
		if err != nil {
			return nil, err
		}
		if matches == nil {
			matches = []string{input}
		}
		for _, p := range matches {
			st, err := os.Stat(p)
			if err != nil {
				return nil, err
			}
			if st.IsDir() {
				err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
					if err != nil || d.IsDir() {
						return err
					}
					if isImageExt(path) {
						paths = append(paths, path)
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			} else if isImageExt(p) {
				paths = append(paths, p)
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func isImageExt(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png", ".jpg", ".jpeg":
		return true
	default:
		return false
	}
}

// MeanLuma returns average luma.
func (f *Frame) MeanLuma() float64 {
	var sum int64
	for _, v := range f.Y {
		sum += int64(v)
	}
	return float64(sum) / float64(len(f.Y))
}

// DownscaleGray returns a nearest-box-ish averaged luma frame no wider than maxWidth.
func (f *Frame) DownscaleGray(maxWidth int) *GrayFrame {
	if maxWidth <= 0 || f.W <= maxWidth {
		g := &GrayFrame{W: f.W, H: f.H, Y: append([]uint8(nil), f.Y...), ScaleX: 1, ScaleY: 1}
		return g
	}
	nw := maxWidth
	nh := int(math.Max(1, math.Round(float64(f.H)*float64(nw)/float64(f.W))))
	out := &GrayFrame{W: nw, H: nh, Y: make([]uint8, nw*nh), ScaleX: float64(f.W) / float64(nw), ScaleY: float64(f.H) / float64(nh)}
	for y := 0; y < nh; y++ {
		y0 := int(float64(y) * float64(f.H) / float64(nh))
		y1 := maxInt(y0+1, int(float64(y+1)*float64(f.H)/float64(nh)))
		for x := 0; x < nw; x++ {
			x0 := int(float64(x) * float64(f.W) / float64(nw))
			x1 := maxInt(x0+1, int(float64(x+1)*float64(f.W)/float64(nw)))
			var sum, n int
			for sy := y0; sy < y1; sy++ {
				base := sy * f.W
				for sx := x0; sx < x1; sx++ {
					sum += int(f.Y[base+sx])
					n++
				}
			}
			out.Y[y*nw+x] = uint8(sum / maxInt(1, n))
		}
	}
	return out
}

// GrayFrame is an analysis-resolution luma plane with source scale factors.
type GrayFrame struct {
	W, H           int
	Y              []uint8
	ScaleX, ScaleY float64
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
