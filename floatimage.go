// Package sdrquant provides deterministic high-bit-depth to 8-bit SDR
// quantization with switchable dithering, noise shaping, and diagnostics.
package sdrquant

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
)

// FloatImage is a row-major, top-down linear or display-referred float image.
// Values are normally in [0,1], but the loader keeps out-of-range values so
// clipping can be reported rather than silently hidden.
type FloatImage struct {
	Width    int
	Height   int
	Channels int
	Pix      []float32
}

// NewFloatImage allocates a zero-valued float image. Channels must be 1 or 3.
func NewFloatImage(width, height, channels int) (*FloatImage, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid image size %dx%d", width, height)
	}
	if channels != 1 && channels != 3 {
		return nil, fmt.Errorf("unsupported channel count %d", channels)
	}
	return &FloatImage{
		Width:    width,
		Height:   height,
		Channels: channels,
		Pix:      make([]float32, width*height*channels),
	}, nil
}

func (im *FloatImage) offset(x, y, channel int) int {
	return (y*im.Width+x)*im.Channels + channel
}

// At returns a pixel channel. The caller must provide valid coordinates.
func (im *FloatImage) At(x, y, channel int) float32 {
	return im.Pix[im.offset(x, y, channel)]
}

// Set stores a pixel channel. The caller must provide valid coordinates.
func (im *FloatImage) Set(x, y, channel int, value float32) {
	im.Pix[im.offset(x, y, channel)] = value
}

// Clone returns an independent copy.
func (im *FloatImage) Clone() *FloatImage {
	out := *im
	out.Pix = append([]float32(nil), im.Pix...)
	return &out
}

// Luma returns Rec.709 luminance, clamped to [0,1] for diagnostic rendering.
// For a grayscale image it returns the single channel.
func (im *FloatImage) Luma(x, y int) float64 {
	if im.Channels == 1 {
		return float64(im.At(x, y, 0))
	}
	r := float64(im.At(x, y, 0))
	g := float64(im.At(x, y, 1))
	b := float64(im.At(x, y, 2))
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// LoadFloatImage loads PNG, JPEG, GIF, or PFM. Raster images are interpreted
// as display-referred values in [0,1]. PFM values are passed through exactly.
func LoadFloatImage(path string) (*FloatImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	head := make([]byte, 2)
	if _, err := io.ReadFull(f, head); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if string(head) == "PF" || string(head) == "Pf" {
		return readPFM(f)
	}

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return floatImageFromImage(src), nil
}

func floatImageFromImage(src image.Image) *FloatImage {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	channels := 3
	switch src.(type) {
	case *image.Gray, *image.Gray16:
		channels = 1
	}
	out, _ := NewFloatImage(width, height, channels)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.NRGBA64Model.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA64)
			if channels == 1 {
				out.Set(x, y, 0, float32(c.R)/65535)
				continue
			}
			out.Set(x, y, 0, float32(c.R)/65535)
			out.Set(x, y, 1, float32(c.G)/65535)
			out.Set(x, y, 2, float32(c.B)/65535)
		}
	}
	return out
}

func writePFM(w io.Writer, im *FloatImage, littleEndian bool) error {
	if im.Channels != 1 && im.Channels != 3 {
		return errors.New("PFM supports only gray or RGB")
	}
	magic := "Pf"
	if im.Channels == 3 {
		magic = "PF"
	}
	scale := float32(1)
	if littleEndian {
		scale = -1
	}
	if _, err := fmt.Fprintf(w, "%s\n%d %d\n%g\n", magic, im.Width, im.Height, scale); err != nil {
		return err
	}
	for y := im.Height - 1; y >= 0; y-- {
		for x := 0; x < im.Width; x++ {
			for c := 0; c < im.Channels; c++ {
				v := im.At(x, y, c)
				if err := writeFloat32(w, v, littleEndian); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// SavePFM writes a binary little-endian Portable Float Map.
func SavePFM(path string, im *FloatImage) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := writePFM(f, im, true); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}

// Clamp01 maps non-finite values to zero and clamps the result to [0,1].
func Clamp01(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Max(0, math.Min(1, v))
}
