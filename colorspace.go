package nightwb

import "math"

// Mat3 is a row-major 3x3 matrix.
type Mat3 [9]float64

func (m Mat3) mulVec(v [3]float64) [3]float64 {
	return [3]float64{
		m[0]*v[0] + m[1]*v[1] + m[2]*v[2],
		m[3]*v[0] + m[4]*v[1] + m[5]*v[2],
		m[6]*v[0] + m[7]*v[1] + m[8]*v[2],
	}
}

func mulMat(a, b Mat3) Mat3 {
	var out Mat3
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			out[r*3+c] = a[r*3+0]*b[0*3+c] +
				a[r*3+1]*b[1*3+c] +
				a[r*3+2]*b[2*3+c]
		}
	}
	return out
}

// inv3 inverts a 3x3 matrix via cofactors. Returns false if singular.
func inv3(m Mat3) (Mat3, bool) {
	co := Mat3{
		+(m[4]*m[8] - m[5]*m[7]), -(m[3]*m[8] - m[5]*m[6]), +(m[3]*m[7] - m[4]*m[6]),
		-(m[1]*m[8] - m[2]*m[7]), +(m[0]*m[8] - m[2]*m[6]), -(m[0]*m[7] - m[1]*m[6]),
		+(m[1]*m[5] - m[2]*m[4]), -(m[0]*m[5] - m[2]*m[3]), +(m[0]*m[4] - m[1]*m[3]),
	}
	det := m[0]*co[0] + m[1]*co[1] + m[2]*co[2]
	if math.Abs(det) < 1e-18 {
		return Mat3{}, false
	}
	// co currently holds cofactors; transpose (adjugate) and scale.
	adj := Mat3{
		co[0], co[3], co[6],
		co[1], co[4], co[7],
		co[2], co[5], co[8],
	}
	for i := range adj {
		adj[i] /= det
	}
	return adj, true
}

// SRGBToLinear decodes an sRGB-encoded channel in [0,1] to linear light.
func SRGBToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// LinearToSRGB encodes a linear-light channel, clamped to [0,1].
func LinearToSRGB(c float64) float64 {
	if c <= 0.0 {
		return 0
	}
	if c <= 0.0031308 {
		return 12.92 * c
	}
	v := 1.055*math.Pow(c, 1.0/2.4) - 0.055
	if v > 1 {
		return 1
	}
	return v
}

// SRGBToXYZ (D65) for linear sRGB primaries.
var SRGBToXYZ = Mat3{
	0.4124564, 0.3575761, 0.1804375,
	0.2126729, 0.7151522, 0.0721750,
	0.0193339, 0.1191920, 0.9503041,
}

var XYZToSRGB, _ = inv3(SRGBToXYZ)

// Bradford cone-response matrices.
var Bradford = Mat3{
	0.8951000, 0.2664000, -0.1614000,
	-0.7502000, 1.7135000, 0.0367000,
	0.0389000, -0.0685000, 1.0296000,
}

var BradfordInv, _ = inv3(Bradford)

// D65 reference white in XYZ.
var D65XYZ = [3]float64{0.95047, 1.0, 1.08883}
