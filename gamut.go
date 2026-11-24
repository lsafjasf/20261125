package main

import "math"

// mat3 is a row-major 3x3 matrix.
type mat3 [9]float64

func (m mat3) mul(v [3]float64) [3]float64 {
	return [3]float64{
		m[0]*v[0] + m[1]*v[1] + m[2]*v[2],
		m[3]*v[0] + m[4]*v[1] + m[5]*v[2],
		m[6]*v[0] + m[7]*v[1] + m[8]*v[2],
	}
}

// invert returns the inverse of a 3x3 matrix using Gaussian elimination
// with partial pivoting. The bool is false when the matrix is singular.
func (m mat3) invert() (mat3, bool) {
	var a [3][6]float64
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			a[r][c] = m[r*3+c]
		}
		a[r][3+r] = 1
	}
	for col := 0; col < 3; col++ {
		pivot := col
		for r := col + 1; r < 3; r++ {
			if math.Abs(a[r][col]) > math.Abs(a[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(a[pivot][col]) < 1e-15 {
			return mat3{}, false
		}
		a[col], a[pivot] = a[pivot], a[col]
		pe := a[col][col]
		for c := 0; c < 6; c++ {
			a[col][c] /= pe
		}
		for r := 0; r < 3; r++ {
			if r == col {
				continue
			}
			f := a[r][col]
			for c := 0; c < 6; c++ {
				a[r][c] -= f * a[col][c]
			}
		}
	}
	var inv mat3
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			inv[r*3+c] = a[r][3+c]
		}
	}
	return inv, true
}

// Gamut describes an RGB display gamut through its linear-RGB<->XYZ matrices.
// In-gamut linear components lie in [0,1].
type Gamut struct {
	name     string
	rgbToXYZ mat3
	xyzToRGB mat3
}

// newSRGBGamut builds the standard sRGB (D65) gamut.
func newSRGBGamut() Gamut {
	inv, ok := srgbToXYZ.invert()
	if !ok {
		panic("sRGB matrix is singular")
	}
	return Gamut{name: "sRGB", rgbToXYZ: srgbToXYZ, xyzToRGB: inv}
}

// newNarrowGamut builds a smaller gamut by moving each sRGB primary toward
// the D65 white point in XYZ: P' = D65 + s*(P-D65). The RGB->XYZ matrix is
// then solved from M*(1,1,1)=D65, so paper white maps identically while the
// achievable chroma volume is reduced by factor s.
func newNarrowGamut(name string, s float64) Gamut {
	d65 := [3]float64{whiteX, whiteY, whiteZ}
	// Columns of MsRGB are the sRGB primaries in XYZ.
	var prims [3][3]float64
	for c := 0; c < 3; c++ {
		for r := 0; r < 3; r++ {
			prims[c][r] = d65[r] + s*(srgbToXYZ[r*3+c]-d65[r])
		}
	}
	// Solve M so that M*(1,1,1) = D65: assemble columns and adjust.
	var m mat3
	for c := 0; c < 3; c++ {
		for r := 0; r < 3; r++ {
			m[r*3+c] = prims[c][r]
		}
	}
	inv, ok := m.invert()
	if !ok {
		panic("narrow gamut matrix is singular")
	}
	weights := inv.mul(d65)
	for c := 0; c < 3; c++ {
		for r := 0; r < 3; r++ {
			m[r*3+c] *= weights[c]
		}
	}
	back, ok := m.invert()
	if !ok {
		panic("narrow gamut matrix is singular")
	}
	return Gamut{name: name, rgbToXYZ: m, xyzToRGB: back}
}

func (g Gamut) xyzToLinear(x XYZ) LinRGB {
	v := g.xyzToRGB.mul([3]float64{x.X, x.Y, x.Z})
	return LinRGB{v[0], v[1], v[2]}
}

func (g Gamut) linearToXYZ(r LinRGB) XYZ {
	v := g.rgbToXYZ.mul([3]float64{r.R, r.G, r.B})
	return XYZ{v[0], v[1], v[2]}
}

func inUnitRange(v float64) bool {
	const eps = 1e-9
	return v >= -eps && v <= 1.0+eps
}

// containsLCh reports whether the LCh color lies inside the gamut.
func (g Gamut) containsLCh(p LCh) bool {
	rgb := g.xyzToLinear(labToXYZ(lchToLab(p)))
	return inUnitRange(rgb.R) && inUnitRange(rgb.G) && inUnitRange(rgb.B)
}

// maxChroma returns the gamut-boundary chroma at fixed L and hue via
// bisection. The feasible chroma interval starts at 0 and is contiguous,
// so binary search tracks the largest in-gamut value.
func (g Gamut) maxChroma(L, H float64) float64 {
	lo, hi := 0.0, 300.0
	if !g.containsLCh(LCh{L: L, C: 0, H: H}) {
		return 0
	}
	for i := 0; i < 60; i++ {
		mid := (lo + hi) / 2
		if g.containsLCh(LCh{L: L, C: mid, H: H}) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

// clipLab performs naive per-channel clipping in device linear RGB.
// It does not preserve hue: the projected point can land on a different
// RGB-cube face and jump to another hue in LCh.
func (g Gamut) clipLab(src Lab) Lab {
	rgb := g.xyzToLinear(labToXYZ(src))
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	out := LinRGB{clamp(rgb.R), clamp(rgb.G), clamp(rgb.B)}
	return xyzToLab(g.linearToXYZ(out))
}
