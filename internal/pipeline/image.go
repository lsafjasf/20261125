package pipeline

import "rec2020-to-rec709-shoulder/internal/colorconv"

// FloatImage is packed linear RGB. Values may exceed 1 in highlights.
type FloatImage struct {
	Width  int
	Height int
	Pix    []float64
}

func NewFloatImage(w, h int) *FloatImage {
	return &FloatImage{Width: w, Height: h, Pix: make([]float64, w*h*3)}
}

func (im *FloatImage) Pixel(x, y int) colorconv.RGB {
	i := (y*im.Width + x) * 3
	return colorconv.RGB{R: im.Pix[i], G: im.Pix[i+1], B: im.Pix[i+2]}
}

func (im *FloatImage) SetPixel(x, y int, c colorconv.RGB) {
	i := (y*im.Width + x) * 3
	im.Pix[i], im.Pix[i+1], im.Pix[i+2] = c.R, c.G, c.B
}

func (im *FloatImage) Clone() *FloatImage {
	out := NewFloatImage(im.Width, im.Height)
	copy(out.Pix, im.Pix)
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
