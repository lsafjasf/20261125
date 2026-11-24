package display

import "math"

// StandardGamut returns a named reference gamut (D65 throughout).
func StandardGamut(name string) (Primaries, bool) {
	switch name {
	case "sRGB", "Rec709":
		return Primaries{
			RedX: 0.64, RedY: 0.33,
			GreenX: 0.30, GreenY: 0.60,
			BlueX: 0.15, BlueY: 0.06,
			WhiteX: 0.3127, WhiteY: 0.3290,
		}, true
	case "P3-D65", "DisplayP3":
		return Primaries{
			RedX: 0.68, RedY: 0.32,
			GreenX: 0.265, GreenY: 0.690,
			BlueX: 0.15, BlueY: 0.06,
			WhiteX: 0.3127, WhiteY: 0.3290,
		}, true
	case "Rec2020":
		return Primaries{
			RedX: 0.708, RedY: 0.292,
			GreenX: 0.170, GreenY: 0.797,
			BlueX: 0.131, BlueY: 0.046,
			WhiteX: 0.3127, WhiteY: 0.3290,
		}, true
	default:
		return Primaries{}, false
	}
}

// Mat3 is a row-major 3x3 linear-light color matrix.
type Mat3 [9]float64

func (m Mat3) mulVec(v [3]float64) [3]float64 {
	return [3]float64{
		m[0]*v[0] + m[1]*v[1] + m[2]*v[2],
		m[3]*v[0] + m[4]*v[1] + m[5]*v[2],
		m[6]*v[0] + m[7]*v[1] + m[8]*v[2],
	}
}

func (m Mat3) determinant() float64 {
	return m[0]*(m[4]*m[8]-m[5]*m[7]) -
		m[1]*(m[3]*m[8]-m[5]*m[6]) +
		m[2]*(m[3]*m[7]-m[4]*m[6])
}

func (m Mat3) inverse() (Mat3, bool) {
	d := m.determinant()
	if math.Abs(d) < 1e-12 {
		return Mat3{}, false
	}
	inv := Mat3{
		(m[4]*m[8] - m[5]*m[7]) / d,
		(m[2]*m[7] - m[1]*m[8]) / d,
		(m[1]*m[5] - m[2]*m[4]) / d,
		(m[5]*m[6] - m[3]*m[8]) / d,
		(m[0]*m[8] - m[2]*m[6]) / d,
		(m[2]*m[3] - m[0]*m[5]) / d,
		(m[3]*m[7] - m[4]*m[6]) / d,
		(m[1]*m[6] - m[0]*m[7]) / d,
		(m[0]*m[4] - m[1]*m[3]) / d,
	}
	return inv, true
}

// RGBToXYZ builds the linear RGB -> CIE XYZ matrix implied by primaries whose
// white point maps to XYZ = (Wx/Wy, 1, ...). Derivation follows the standard
// primary-chromaticity construction used by sRGB/P3/Rec2020.
func RGBToXYZ(p Primaries) (Mat3, bool) {
	white := xyzFromXY(p.WhiteX, p.WhiteY, 1)
	xyzR := xyzFromXY(p.RedX, p.RedY, 1)
	xyzG := xyzFromXY(p.GreenX, p.GreenY, 1)
	xyzB := xyzFromXY(p.BlueX, p.BlueY, 1)

	primary := Mat3{
		xyzR[0], xyzG[0], xyzB[0],
		xyzR[1], xyzG[1], xyzB[1],
		xyzR[2], xyzG[2], xyzB[2],
	}
	inv, ok := primary.inverse()
	if !ok {
		return Mat3{}, false
	}
	scale := inv.mulVec(white)
	return Mat3{
		primary[0] * scale[0], primary[1] * scale[1], primary[2] * scale[2],
		primary[3] * scale[0], primary[4] * scale[1], primary[5] * scale[2],
		primary[6] * scale[0], primary[7] * scale[1], primary[8] * scale[2],
	}, true
}

// GamutMapMatrix returns the matrix that converts linear RGB in src primaries
// into linear RGB in dst primaries via XYZ (white points match, both D65).
func GamutMapMatrix(src, dst Primaries) (Mat3, bool) {
	srcM, ok1 := RGBToXYZ(src)
	dstM, ok2 := RGBToXYZ(dst)
	if !ok1 || !ok2 {
		return Mat3{}, false
	}
	dstInv, ok := dstM.inverse()
	if !ok {
		return Mat3{}, false
	}
	return multiply(dstInv, srcM), true
}

func multiply(a, b Mat3) Mat3 {
	var out Mat3
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			sum := 0.0
			for k := 0; k < 3; k++ {
				sum += a[row*3+k] * b[k*3+col]
			}
			out[row*3+col] = sum
		}
	}
	return out
}

func xyzFromXY(x, y, yY float64) [3]float64 {
	if y == 0 {
		return [3]float64{0, 0, 0}
	}
	return [3]float64{x * yY / y, yY, (1 - x - y) * yY / y}
}
