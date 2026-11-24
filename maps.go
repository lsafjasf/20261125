package sdrquant

import (
	"math"
)

func classificationMap(classes []PixelClass, width, height int) *FloatImage {
	im, _ := NewFloatImage(width, height, 3)
	for i, class := range classes {
		r, g, b := 0.10, 0.10, 0.10
		switch class {
		case ClassSourceTexture:
			r, g, b = 0.20, 0.55, 0.32
		case ClassSourceEdge:
			r, g, b = 0.95, 0.16, 0.12
		case ClassQuantContour:
			r, g, b = 0.98, 0.76, 0.06
		case ClassDitherNoise:
			r, g, b = 0.10, 0.46, 0.95
		case ClassClipped:
			r, g, b = 0.95, 0.06, 0.86
		}
		im.Pix[i*3] = float32(r)
		im.Pix[i*3+1] = float32(g)
		im.Pix[i*3+2] = float32(b)
	}
	return im
}

func signedMap(values []float64, width, height int, minimumScale float64) *FloatImage {
	im, _ := NewFloatImage(width, height, 3)
	scale := math.Max(minimumScale, percentileAbs(values, 0.99))
	if scale <= 0 {
		scale = 1
	}
	for i, value := range values {
		t := math.Max(-1, math.Min(1, value/scale))
		var r, g, b float64
		if t >= 0 {
			r = 0.55 + 0.45*t
			g = 0.55 - 0.10*t
			b = 0.55 - 0.50*t
		} else {
			a := -t
			r = 0.55 - 0.45*a
			g = 0.55 + 0.05*a
			b = 0.55 + 0.45*a
		}
		im.Pix[i*3] = float32(r)
		im.Pix[i*3+1] = float32(g)
		im.Pix[i*3+2] = float32(b)
	}
	return im
}

func heatMap(normalized []float64, width, height int) *FloatImage {
	im, _ := NewFloatImage(width, height, 3)
	for i, value := range normalized {
		t := math.Max(0, math.Min(1, value))
		r, g, b := heatColor(t)
		im.Pix[i*3] = float32(r)
		im.Pix[i*3+1] = float32(g)
		im.Pix[i*3+2] = float32(b)
	}
	return im
}

func heatColor(t float64) (float64, float64, float64) {
	stops := [][3]float64{
		{0.02, 0.02, 0.08},
		{0.05, 0.18, 0.55},
		{0.00, 0.70, 0.78},
		{0.92, 0.84, 0.12},
		{1.00, 1.00, 1.00},
	}
	scaled := t * float64(len(stops)-1)
	index := int(math.Floor(scaled))
	if index >= len(stops)-1 {
		s := stops[len(stops)-1]
		return s[0], s[1], s[2]
	}
	f := scaled - float64(index)
	a, b := stops[index], stops[index+1]
	return a[0] + f*(b[0]-a[0]), a[1] + f*(b[1]-a[1]), a[2] + f*(b[2]-a[2])
}
