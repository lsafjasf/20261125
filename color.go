package main

import "math"

// D65 reference white in XYZ.
const (
	whiteX = 0.95047
	whiteY = 1.0
	whiteZ = 1.08883
)

type XYZ struct{ X, Y, Z float64 }
type Lab struct{ L, A, B float64 }
type LCh struct{ L, C, H float64 }    // H in degrees, range [0,360)
type LinRGB struct{ R, G, B float64 } // linear-light components

// srgbToLinear decodes an sRGB component (0..1) to linear light.
func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// linearToSRGB encodes a linear-light component (0..1) to sRGB.
func linearToSRGB(c float64) float64 {
	if c <= 0.0031308 {
		return 12.92 * c
	}
	return 1.055*math.Pow(c, 1.0/2.4) - 0.055
}

// srgbMatrix maps linear sRGB to XYZ (D65).
var srgbToXYZ = mat3{
	0.4124564, 0.3575761, 0.1804375,
	0.2126729, 0.7151522, 0.0721750,
	0.0193339, 0.1191920, 0.9503041,
}

// srgbRGBToXYZ converts linear-light sRGB to XYZ.
func srgbRGBToXYZ(r LinRGB) XYZ {
	v := srgbToXYZ.mul([3]float64{r.R, r.G, r.B})
	return XYZ{v[0], v[1], v[2]}
}

func labF(t float64) float64 {
	const eps = 216.0 / 24389.0
	const k = 24389.0 / 27.0
	if t > eps {
		return math.Cbrt(t)
	}
	return (k*t + 16.0) / 116.0
}

func labFInv(t float64) float64 {
	const eps = 216.0 / 24389.0
	const k = 24389.0 / 27.0
	if t3 := t * t * t; t3 > eps {
		return t3
	}
	return (116.0*t - 16.0) / k
}

// xyzToLab converts XYZ (D65) to CIE L*a*b*.
func xyzToLab(x XYZ) Lab {
	fx := labF(x.X / whiteX)
	fy := labF(x.Y / whiteY)
	fz := labF(x.Z / whiteZ)
	return Lab{
		L: 116.0*fy - 16.0,
		A: 500.0 * (fx - fy),
		B: 200.0 * (fy - fz),
	}
}

// labToXYZ is the inverse of xyzToLab.
func labToXYZ(l Lab) XYZ {
	fy := (l.L + 16.0) / 116.0
	fx := l.A/500.0 + fy
	fz := fy - l.B/200.0
	return XYZ{
		X: whiteX * labFInv(fx),
		Y: whiteY * labFInv(fy),
		Z: whiteZ * labFInv(fz),
	}
}

// labToLCh converts Lab to polar LCh (hue in degrees).
func labToLCh(l Lab) LCh {
	c := math.Hypot(l.A, l.B)
	h := math.Atan2(l.B, l.A) * 180.0 / math.Pi
	if h < 0 {
		h += 360.0
	}
	if c < 1e-12 {
		h = 0
	}
	return LCh{L: l.L, C: c, H: h}
}

// lchToLab converts LCh back to Lab.
func lchToLab(p LCh) Lab {
	rad := p.H * math.Pi / 180.0
	return Lab{
		L: p.L,
		A: p.C * math.Cos(rad),
		B: p.C * math.Sin(rad),
	}
}
