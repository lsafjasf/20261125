package lutdiff

import "math"

// DistRGB is the Euclidean distance between two normalized RGB colors.
func DistRGB(a, b RGB) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) +
		(a[1]-b[1])*(a[1]-b[1]) +
		(a[2]-b[2])*(a[2]-b[2]))
}

// MaxChannelDelta returns the largest absolute channel difference.
func MaxChannelDelta(a, b RGB) float64 {
	d := 0.0
	for i := 0; i < 3; i++ {
		d = math.Max(d, math.Abs(a[i]-b[i]))
	}
	return d
}

// MaxCodeDelta8 returns the largest channel difference expressed in 8-bit
// code values (0..255), before rounding.
func MaxCodeDelta8(a, b RGB) float64 {
	return MaxChannelDelta(a, b) * 255
}

// Average returns the arithmetic mean of two results. This is the operation
// that must NOT be used to pretend an order disagreement does not exist.
func Average(a, b RGB) RGB {
	return RGB{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2, (a[2] + b[2]) / 2}
}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// ToLab converts sRGB code values to CIE L*a*b* under D65.
func ToLab(c RGB) (L, a, b float64) {
	r := srgbToLinear(Clamp01(c[0]))
	g := srgbToLinear(Clamp01(c[1]))
	bl := srgbToLinear(Clamp01(c[2]))

	x := (0.4124564*r + 0.3575761*g + 0.1804375*bl) / 0.95047
	y := 0.2126729*r + 0.7151522*g + 0.0721750*bl
	z := (0.0193339*r + 0.1191920*g + 0.9503041*bl) / 1.08883

	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116.0
	}
	fx, fy, fz := f(x), f(y), f(z)
	L = 116*fy - 16
	a = 500 * (fx - fy)
	b = 200 * (fy - fz)
	return
}

// DeltaE76 is the CIE76 perceptual color difference. Roughly:
// <1 invisible, 1-2 only experienced observers, 2-10 visible at a glance,
// >10 clearly different colors.
func DeltaE76(c1, c2 RGB) float64 {
	l1, a1, b1 := ToLab(c1)
	l2, a2, b2 := ToLab(c2)
	return math.Sqrt((l1-l2)*(l1-l2) + (a1-a2)*(a1-a2) + (b1-b2)*(b1-b2))
}

// HSLToRGB converts HSL in [0,1] ranges to normalized RGB.
func HSLToRGB(h, s, l float64) RGB {
	h = math.Mod(h, 1)
	if h < 0 {
		h += 1
	}
	if s == 0 {
		return RGB{l, l, l}
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6.0:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3.0:
			return p + (q-p)*(2.0/3.0-t)*6
		default:
			return p
		}
	}
	return RGB{hue(h + 1.0/3.0), hue(h), hue(h - 1.0/3.0)}
}

type stop struct {
	t float64
	c RGB
}

// Viridis-like perceptually ordered heat stops.
var heatStops = []stop{
	{0.00, RGB{68 / 255.0, 1 / 255.0, 84 / 255.0}},
	{0.25, RGB{59 / 255.0, 82 / 255.0, 139 / 255.0}},
	{0.50, RGB{33 / 255.0, 145 / 255.0, 140 / 255.0}},
	{0.75, RGB{94 / 255.0, 201 / 255.0, 98 / 255.0}},
	{1.00, RGB{253 / 255.0, 231 / 255.0, 37 / 255.0}},
}

// HeatColor maps t in [0,1] (clamped) to the difference heat map palette.
func HeatColor(t float64) RGB {
	t = Clamp01(t)
	for i := 1; i < len(heatStops); i++ {
		if t <= heatStops[i].t {
			s0, s1 := heatStops[i-1], heatStops[i]
			k := (t - s0.t) / (s1.t - s0.t)
			return lerpRGB(k, s0.c, s1.c)
		}
	}
	return heatStops[len(heatStops)-1].c
}
