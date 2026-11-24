package colorcal

import "math"

var d65White = [3]float64{0.95047, 1.0, 1.08883}

func linearRGBToLab(rgb RGB) [3]float64 {
	x := 0.4124564*rgb[0] + 0.3575761*rgb[1] + 0.1804375*rgb[2]
	y := 0.2126729*rgb[0] + 0.7151522*rgb[1] + 0.0721750*rgb[2]
	z := 0.0193339*rgb[0] + 0.1191920*rgb[1] + 0.9503041*rgb[2]

	f := func(t float64) float64 {
		const delta = 6.0 / 29.0
		if t > delta*delta*delta {
			return math.Cbrt(t)
		}
		return t/(3*delta*delta) + 4.0/29.0
	}
	fx, fy, fz := f(x/d65White[0]), f(y/d65White[1]), f(z/d65White[2])
	return [3]float64{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

// DeltaE2000 computes CIEDE2000 for two linear sRGB/D65 colors.
func DeltaE2000(a, b RGB) float64 {
	return labDeltaE2000(linearRGBToLab(a), linearRGBToLab(b))
}

func labDeltaE2000(l1, l2 [3]float64) float64 {
	l1v, a1, b1 := l1[0], l1[1], l1[2]
	l2v, a2, b2 := l2[0], l2[1], l2[2]

	c1 := math.Hypot(a1, b1)
	c2 := math.Hypot(a2, b2)
	avgC := (c1 + c2) / 2
	c7 := math.Pow(avgC, 7)
	g := 0.5 * (1 - math.Sqrt(c7/(c7+math.Pow(25, 7))))

	a1p, a2p := (1+g)*a1, (1+g)*a2
	c1p := math.Hypot(a1p, b1)
	c2p := math.Hypot(a2p, b2)
	avgCp := (c1p + c2p) / 2

	hue := func(ap, bv, cp float64) float64 {
		if cp == 0 {
			return 0
		}
		h := math.Atan2(bv, ap) * 180 / math.Pi
		if h < 0 {
			h += 360
		}
		return h
	}
	h1p := hue(a1p, b1, c1p)
	h2p := hue(a2p, b2, c2p)

	dlp := l2v - l1v
	dcp := c2p - c1p
	dhp := 0.0
	if c1p*c2p != 0 {
		dh := h2p - h1p
		if dh > 180 {
			dh -= 360
		} else if dh < -180 {
			dh += 360
		}
		dhp = 2 * math.Sqrt(c1p*c2p) * math.Sin((dh/2)*math.Pi/180)
	}

	avgLp := (l1v + l2v) / 2
	avgHp := 0.0
	switch {
	case c1p*c2p == 0:
		avgHp = h1p + h2p
	case math.Abs(h1p-h2p) <= 180:
		avgHp = (h1p + h2p) / 2
	case h1p+h2p < 360:
		avgHp = (h1p + h2p + 360) / 2
	default:
		avgHp = (h1p + h2p - 360) / 2
	}

	t := 1 -
		0.17*math.Cos((avgHp-30)*math.Pi/180) +
		0.24*math.Cos((2*avgHp)*math.Pi/180) +
		0.32*math.Cos((3*avgHp+6)*math.Pi/180) -
		0.20*math.Cos((4*avgHp-63)*math.Pi/180)

	dTheta := 30 * math.Exp(-math.Pow((avgHp-275)/25, 2))
	cp7 := math.Pow(avgCp, 7)
	rc := 2 * math.Sqrt(cp7/(cp7+math.Pow(25, 7)))
	sl := 1 + (0.015*math.Pow(avgLp-50, 2))/math.Sqrt(20+math.Pow(avgLp-50, 2))
	sc := 1 + 0.045*avgCp
	sh := 1 + 0.015*avgCp*t
	rt := -math.Sin((2*dTheta)*math.Pi/180) * rc

	termL := dlp / sl
	termC := dcp / sc
	termH := dhp / sh
	return math.Sqrt(termL*termL + termC*termC + termH*termH + rt*termC*termH)
}

func sRGBByteToLinear(v uint8) float64 {
	return SRGBToLinear(float64(v) / 255.0)
}

// SRGBToLinear decodes an sRGB gamma-compressed channel.
func SRGBToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}
