package main

import "math"

// mat3 is a row-major 3x3 matrix used for RGB<->XYZ primaries conversion.
type mat3 [9]float64

func (m mat3) mul(v [3]float64) [3]float64 {
	return [3]float64{
		m[0]*v[0] + m[1]*v[1] + m[2]*v[2],
		m[3]*v[0] + m[4]*v[1] + m[5]*v[2],
		m[6]*v[0] + m[7]*v[1] + m[8]*v[2],
	}
}

func (a mat3) mulMat(b mat3) mat3 {
	var out mat3
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			out[r*3+c] = a[r*3+0]*b[0*3+c] + a[r*3+1]*b[1*3+c] + a[r*3+2]*b[2*3+c]
		}
	}
	return out
}

func (m mat3) inverse() (mat3, bool) {
	a := m
	det := a[0]*(a[4]*a[8]-a[5]*a[7]) -
		a[1]*(a[3]*a[8]-a[5]*a[6]) +
		a[2]*(a[3]*a[7]-a[4]*a[6])
	if math.Abs(det) < 1e-15 {
		return mat3{}, false
	}
	invDet := 1.0 / det
	return mat3{
		(a[4]*a[8] - a[5]*a[7]) * invDet,
		(a[2]*a[7] - a[1]*a[8]) * invDet,
		(a[1]*a[5] - a[2]*a[4]) * invDet,
		(a[5]*a[6] - a[3]*a[8]) * invDet,
		(a[0]*a[8] - a[2]*a[6]) * invDet,
		(a[2]*a[3] - a[0]*a[5]) * invDet,
		(a[3]*a[7] - a[4]*a[6]) * invDet,
		(a[1]*a[6] - a[0]*a[7]) * invDet,
		(a[0]*a[4] - a[1]*a[3]) * invDet,
	}, true
}

// primariesMatrix derives the linear-RGB -> XYZ matrix from xy chromaticities.
func primariesMatrix(rx, ry, gx, gy, bx, by, wx, wy float64) mat3 {
	xyz := func(x, y float64) [3]float64 {
		return [3]float64{x / y, 1.0, (1.0 - x - y) / y}
	}
	r := xyz(rx, ry)
	g := xyz(gx, gy)
	b := xyz(bx, by)
	white := xyz(wx, wy)
	// Columns are the primary XYZ vectors.
	p := mat3{
		r[0], g[0], b[0],
		r[1], g[1], b[1],
		r[2], g[2], b[2],
	}
	pInv, ok := p.inverse()
	if !ok {
		// Chromaticity sets in this program are always invertible.
		panic("degenerate primaries")
	}
	scale := pInv.mul(white)
	return mat3{
		r[0] * scale[0], g[0] * scale[1], b[0] * scale[2],
		r[1] * scale[0], g[1] * scale[1], b[1] * scale[2],
		r[2] * scale[0], g[2] * scale[1], b[2] * scale[2],
	}
}
