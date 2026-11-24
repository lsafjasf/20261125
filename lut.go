// Package lutdiff demonstrates that chaining two nonlinear 3D LUTs is
// order dependent: LUT A followed by LUT B generally differs from B then A.
//
// The package is intentionally implemented with the Go standard library only.
package lutdiff

import "math"

// RGB is a linear-in-code-value RGB color in the normalized range [0, 1].
type RGB [3]float64

// Clamp01 constrains v to [0, 1].
func Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Clamp constrains every channel to the display-referred gamut [0, 1].
func (c RGB) Clamp() RGB {
	return RGB{Clamp01(c[0]), Clamp01(c[1]), Clamp01(c[2])}
}

func lerp(t, a, b float64) float64 { return a + (b-a)*t }

func lerpRGB(t float64, a, b RGB) RGB {
	return RGB{lerp(t, a[0], b[0]), lerp(t, a[1], b[1]), lerp(t, a[2], b[2])}
}

// LUT3D is an N x N x N RGB lookup table with trilinear interpolation.
//
// Grid coordinates are ordered (R, G, B) and the flat storage index is
// 3 * (((r*N + g)*N + b)).
type LUT3D struct {
	Size int
	Data []float64
}

// NewLUT3D allocates a zeroed table. Size must be at least 2.
func NewLUT3D(size int) *LUT3D {
	if size < 2 {
		panic("lutdiff: LUT3D size must be >= 2")
	}
	return &LUT3D{Size: size, Data: make([]float64, 3*size*size*size)}
}

func (l *LUT3D) index(r, g, b int) int {
	return 3 * ((r*l.Size+g)*l.Size + b)
}

// Set stores a grid-node color.
func (l *LUT3D) Set(r, g, b int, c RGB) {
	i := l.index(r, g, b)
	l.Data[i+0] = c[0]
	l.Data[i+1] = c[1]
	l.Data[i+2] = c[2]
}

// At returns a grid-node color.
func (l *LUT3D) At(r, g, b int) RGB {
	i := l.index(r, g, b)
	return RGB{l.Data[i], l.Data[i+1], l.Data[i+2]}
}

// Transform maps one color to another in normalized code values.
type Transform func(RGB) RGB

// BuildLUT samples fn on a uniform Size^3 grid. Outputs are gamut clamped,
// matching real display LUT authoring.
func BuildLUT(size int, fn Transform) *LUT3D {
	lut := NewLUT3D(size)
	last := float64(size - 1)
	for r := 0; r < size; r++ {
		for g := 0; g < size; g++ {
			for b := 0; b < size; b++ {
				c := RGB{float64(r) / last, float64(g) / last, float64(b) / last}
				lut.Set(r, g, b, fn(c).Clamp())
			}
		}
	}
	return lut
}

// IdentityLUT returns a grid that reproduces its input exactly under
// trilinear interpolation.
func IdentityLUT(size int) *LUT3D {
	return BuildLUT(size, func(c RGB) RGB { return c })
}

// Apply evaluates the LUT with trilinear interpolation. Inputs are clamped
// to the table domain and outputs stay within the hull of grid values.
func (l *LUT3D) Apply(c RGB) RGB {
	last := l.Size - 1

	x := Clamp01(c[0]) * float64(last)
	y := Clamp01(c[1]) * float64(last)
	z := Clamp01(c[2]) * float64(last)

	r0 := int(math.Floor(x))
	g0 := int(math.Floor(y))
	b0 := int(math.Floor(z))
	if r0 >= last {
		r0 = last - 1
	}
	if g0 >= last {
		g0 = last - 1
	}
	if b0 >= last {
		b0 = last - 1
	}
	r1, g1, b1 := r0+1, g0+1, b0+1

	fr := x - float64(r0)
	fg := y - float64(g0)
	fb := z - float64(b0)

	c000 := l.At(r0, g0, b0)
	c100 := l.At(r1, g0, b0)
	c010 := l.At(r0, g1, b0)
	c110 := l.At(r1, g1, b0)
	c001 := l.At(r0, g0, b1)
	c101 := l.At(r1, g0, b1)
	c011 := l.At(r0, g1, b1)
	c111 := l.At(r1, g1, b1)

	a00 := lerpRGB(fr, c000, c100)
	a10 := lerpRGB(fr, c010, c110)
	a01 := lerpRGB(fr, c001, c101)
	a11 := lerpRGB(fr, c011, c111)

	d0 := lerpRGB(fg, a00, a10)
	d1 := lerpRGB(fg, a01, a11)

	return lerpRGB(fb, d0, d1)
}
