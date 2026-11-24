package lutdiff

import (
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// FloatImage is an in-memory RGB image kept in floating point so that
// difference maps are computed before 8-bit quantization.
type FloatImage struct {
	W, H   int
	Pixels []RGB
}

// NewFloatImage allocates a W x H image.
func NewFloatImage(w, h int) *FloatImage {
	return &FloatImage{W: w, H: h, Pixels: make([]RGB, w*h)}
}

func (f *FloatImage) at(x, y int) RGB { return f.Pixels[y*f.W+x] }

func (f *FloatImage) set(x, y int, c RGB) { f.Pixels[y*f.W+x] = c }

// RGBA quantizes to 8-bit sRGB code values.
func (f *FloatImage) RGBA() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, f.W, f.H))
	for i, c := range f.Pixels {
		img.Pix[4*i+0] = byte(math.Round(Clamp01(c[0]) * 255))
		img.Pix[4*i+1] = byte(math.Round(Clamp01(c[1]) * 255))
		img.Pix[4*i+2] = byte(math.Round(Clamp01(c[2]) * 255))
		img.Pix[4*i+3] = 255
	}
	return img
}

// SavePNG writes an image as PNG, creating parent directories as needed.
func SavePNG(img image.Image, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// OrderRenders bundles both real pipeline orders and their arithmetic
// average. Avg exists only to demonstrate why averaging is not a valid fix.
type OrderRenders struct {
	Src *FloatImage
	AB  *FloatImage
	BA  *FloatImage
	Avg *FloatImage
}

// RenderOrders applies both orders to every pixel of src.
func RenderOrders(p *Pipeline, src *FloatImage) *OrderRenders {
	ab := NewFloatImage(src.W, src.H)
	ba := NewFloatImage(src.W, src.H)
	avg := NewFloatImage(src.W, src.H)
	for y := 0; y < src.H; y++ {
		for x := 0; x < src.W; x++ {
			c := src.at(x, y)
			first := p.AB(c)
			second := p.BA(c)
			ab.set(x, y, first)
			ba.set(x, y, second)
			avg.set(x, y, Average(first, second))
		}
	}
	return &OrderRenders{Src: src, AB: ab, BA: ba, Avg: avg}
}

// DiffStats returns the maximum Euclidean RGB distance and maximum channel
// code-value delta between two float images.
func DiffStats(a, b *FloatImage) (maxDist, maxCode float64) {
	for i := range a.Pixels {
		maxDist = math.Max(maxDist, DistRGB(a.Pixels[i], b.Pixels[i]))
		maxCode = math.Max(maxCode, MaxCodeDelta8(a.Pixels[i], b.Pixels[i]))
	}
	return maxDist, maxCode
}

// DiffHeatmap paints per-pixel Euclidean distance through HeatColor.
// Distances are divided by scale; pass a positive fixed scale so separate
// maps share one absolute legend.
func DiffHeatmap(a, b *FloatImage, scale float64) *image.RGBA {
	if scale <= 0 {
		scale = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, a.W, a.H))
	for i := 0; i < len(a.Pixels); i++ {
		t := DistRGB(a.Pixels[i], b.Pixels[i]) / scale
		c := HeatColor(t)
		img.Pix[4*i+0] = byte(math.Round(c[0] * 255))
		img.Pix[4*i+1] = byte(math.Round(c[1] * 255))
		img.Pix[4*i+2] = byte(math.Round(c[2] * 255))
		img.Pix[4*i+3] = 255
	}
	return img
}

// AmplifiedDiff renders the maximum per-channel difference as an amplified
// grayscale map. gain multiplies normalized differences before clipping.
func AmplifiedDiff(a, b *FloatImage, gain float64) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, a.W, a.H))
	for i := 0; i < len(a.Pixels); i++ {
		d := MaxChannelDelta(a.Pixels[i], b.Pixels[i]) * gain
		v := byte(math.Round(Clamp01(d) * 255))
		img.Pix[i] = v
	}
	return img
}

// AverageReferenceHeatmap shows distance(avg(AB,BA), AB), i.e. the error
// caused by presenting the average as if it were the A->B result.
func AverageReferenceHeatmap(r *OrderRenders, scale float64) *image.RGBA {
	return DiffHeatmap(r.Avg, r.AB, scale)
}
