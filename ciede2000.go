package main

import "math"

// ciede2000 computes the CIEDE2000 color difference (kL=kC=kH=1) between
// two CIE L*a*b* colors, following Sharma, Wu & Dalal (2005).
func ciede2000(c1, c2 Lab) float64 {
	l1, a1, b1 := c1.L, c1.A, c1.B
	l2, a2, b2 := c2.L, c2.A, c2.B

	c1ab := math.Hypot(a1, b1)
	c2ab := math.Hypot(a2, b2)
	cBarAb := (c1ab + c2ab) / 2

	cBarAb7 := math.Pow(cBarAb, 7)
	g := 0.5 * (1 - math.Sqrt(cBarAb7/(cBarAb7+math.Pow(25, 7))))

	a1p := (1 + g) * a1
	a2p := (1 + g) * a2

	c1p := math.Hypot(a1p, b1)
	c2p := math.Hypot(a2p, b2)

	h1p := hueAngleDeg(a1p, b1)
	h2p := hueAngleDeg(a2p, b2)

	dLp := l2 - l1
	dCp := c2p - c1p

	var dHp float64
	switch {
	case c1p*c2p == 0:
		dHp = 0
	case math.Abs(h2p-h1p) <= 180:
		dHp = h2p - h1p
	case h2p-h1p > 180:
		dHp = h2p - h1p - 360
	default:
		dHp = h2p - h1p + 360
	}

	dEHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(deg2rad(dHp)/2)

	lBarP := (l1 + l2) / 2
	cBarP := (c1p + c2p) / 2

	var hBarP float64
	switch {
	case c1p*c2p == 0:
		hBarP = h1p + h2p
	case math.Abs(h1p-h2p) <= 180:
		hBarP = (h1p + h2p) / 2
	case h1p+h2p < 360:
		hBarP = (h1p + h2p + 360) / 2
	default:
		hBarP = (h1p + h2p - 360) / 2
	}

	t := 1 - 0.17*math.Cos(deg2rad(hBarP-30)) +
		0.24*math.Cos(deg2rad(2*hBarP)) +
		0.32*math.Cos(deg2rad(3*hBarP+6)) -
		0.20*math.Cos(deg2rad(4*hBarP-63))

	dTheta := 30 * math.Exp(-math.Pow((hBarP-275)/25, 2))
	cBarP7 := math.Pow(cBarP, 7)
	rc := 2 * math.Sqrt(cBarP7/(cBarP7+math.Pow(25, 7)))
	sl := 1 + (0.015*math.Pow(lBarP-50, 2))/
		math.Sqrt(20+math.Pow(lBarP-50, 2))
	sc := 1 + 0.045*cBarP
	sh := 1 + 0.015*cBarP*t
	rt := -math.Sin(deg2rad(2*dTheta)) * rc

	dL := dLp / sl
	dC := dCp / sc
	dH := dEHp / sh
	return math.Sqrt(dL*dL + dC*dC + dH*dH + rt*dC*dH)
}

func deg2rad(d float64) float64 { return d * math.Pi / 180 }

func hueAngleDeg(a, b float64) float64 {
	if a == 0 && b == 0 {
		return 0
	}
	h := math.Atan2(b, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}
