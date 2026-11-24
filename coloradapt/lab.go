package coloradapt

import "math"

type Lab struct {
	L float64
	A float64
	B float64
}

func LabFromXYZ(xyz, white Vec3) Lab {
	fx := labPivot(xyz[0] / white[0])
	fy := labPivot(xyz[1] / white[1])
	fz := labPivot(xyz[2] / white[2])
	return Lab{
		L: 116*fy - 16,
		A: 500 * (fx - fy),
		B: 200 * (fy - fz),
	}
}

func labPivot(value float64) float64 {
	const delta = 6.0 / 29.0
	if value > math.Pow(delta, 3) {
		return math.Cbrt(value)
	}
	return value/(3*delta*delta) + 4.0/29.0
}

func DeltaE2000(reference, sample Lab) float64 {
	lBar := (reference.L + sample.L) / 2
	c1 := math.Hypot(reference.A, reference.B)
	c2 := math.Hypot(sample.A, sample.B)
	cBar := (c1 + c2) / 2

	cBar7 := math.Pow(cBar, 7)
	g := 0.5 * (1 - math.Sqrt(cBar7/(cBar7+math.Pow(25, 7))))
	a1Prime := (1 + g) * reference.A
	a2Prime := (1 + g) * sample.A
	c1Prime := math.Hypot(a1Prime, reference.B)
	c2Prime := math.Hypot(a2Prime, sample.B)
	cPrimeBar := (c1Prime + c2Prime) / 2

	h1Prime := hueAngle(a1Prime, reference.B)
	h2Prime := hueAngle(a2Prime, sample.B)

	var deltaHPrime float64
	switch {
	case c1Prime*c2Prime == 0:
		deltaHPrime = 0
	case math.Abs(h2Prime-h1Prime) <= 180:
		deltaHPrime = h2Prime - h1Prime
	case h2Prime-h1Prime > 180:
		deltaHPrime = h2Prime - h1Prime - 360
	default:
		deltaHPrime = h2Prime - h1Prime + 360
	}

	deltaLPrime := sample.L - reference.L
	deltaCPrime := c2Prime - c1Prime
	deltaH := 2 * math.Sqrt(c1Prime*c2Prime) * math.Sin(radians(deltaHPrime/2))

	var hPrimeBar float64
	switch {
	case c1Prime*c2Prime == 0:
		hPrimeBar = h1Prime + h2Prime
	case math.Abs(h1Prime-h2Prime) <= 180:
		hPrimeBar = (h1Prime + h2Prime) / 2
	case h1Prime+h2Prime < 360:
		hPrimeBar = (h1Prime + h2Prime + 360) / 2
	default:
		hPrimeBar = (h1Prime + h2Prime - 360) / 2
	}

	t := 1 -
		0.17*math.Cos(radians(hPrimeBar-30)) +
		0.24*math.Cos(radians(2*hPrimeBar)) +
		0.32*math.Cos(radians(3*hPrimeBar+6)) -
		0.20*math.Cos(radians(4*hPrimeBar-63))

	deltaTheta := 30 * math.Exp(-math.Pow((hPrimeBar-275)/25, 2))
	cPrimeBar7 := math.Pow(cPrimeBar, 7)
	rc := 2 * math.Sqrt(cPrimeBar7/(cPrimeBar7+math.Pow(25, 7)))
	sl := 1 + (0.015*math.Pow(lBar-50, 2))/math.Sqrt(20+math.Pow(lBar-50, 2))
	sc := 1 + 0.045*cPrimeBar
	sh := 1 + 0.015*cPrimeBar*t
	rt := -math.Sin(radians(2*deltaTheta)) * rc

	return math.Sqrt(
		math.Pow(deltaLPrime/sl, 2) +
			math.Pow(deltaCPrime/sc, 2) +
			math.Pow(deltaH/sh, 2) +
			rt*(deltaCPrime/sc)*(deltaH/sh),
	)
}

func hueAngle(a, b float64) float64 {
	if a == 0 && b == 0 {
		return 0
	}
	angle := degrees(math.Atan2(b, a))
	if angle < 0 {
		angle += 360
	}
	return angle
}

func radians(degrees float64) float64 {
	return degrees * math.Pi / 180
}

func degrees(radians float64) float64 {
	return radians * 180 / math.Pi
}
