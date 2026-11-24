package main

import (
	"fmt"
	"math"
)

// NitsToPreviewSRGB converts absolute Rec.2020 linear nit values to an sRGB
// hex color using whiteNits as the value mapped to sRGB 1.0. Values above
// whiteNits are clipped, which is deliberately useful for absolute-scale
// previews.
func NitsToPreviewSRGB(nits RGB, whiteNits float64) RGB {
	r2020 := nits.R / whiteNits
	g2020 := nits.G / whiteNits
	b2020 := nits.B / whiteNits

	r709 := 1.6604910021*r2020 - 0.5876411388*g2020 - 0.0728498633*b2020
	g709 := -0.1245504745*r2020 + 1.1328998971*g2020 - 0.0083494226*b2020
	b709 := -0.0181507633*r2020 - 0.1005788980*g2020 + 1.1187296613*b2020

	encode := func(x float64) float64 {
		if x <= 0 {
			return 0
		}
		if x >= 1 {
			return 1
		}
		if x <= 0.0031308 {
			return 12.92 * x
		}
		return 1.055*math.Pow(x, 1/2.4) - 0.055
	}
	return RGB{encode(r709), encode(g709), encode(b709)}
}

func RGBHex(v RGB) string {
	toByte := func(x float64) int {
		if x < 0 {
			x = 0
		}
		if x > 1 {
			x = 1
		}
		return int(x*255 + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", toByte(v.R), toByte(v.G), toByte(v.B))
}
