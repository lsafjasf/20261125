package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func srgbEnc(c float64) uint8 {
	if c <= 0.0031308 {
		c *= 12.92
	} else {
		c = 1.055*math.Pow(c, 1/2.4) - 0.055
	}
	if c < 0 {
		c = 0
	}
	if c > 1 {
		c = 1
	}
	return uint8(c*255 + 0.5)
}

func main() {
	w, h := 240, 180
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	warm := [3]float64{1.0, 0.55, 0.28}
	cool := [3]float64{0.55, 0.75, 1.0}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ill := warm
			if x >= w/2 {
				ill = cool
			}
			var refl [3]float64
			switch {
			case x >= 80 && x < 160 && y >= 60 && y < 120:
				refl = [3]float64{0.5, 0.5, 0.5} // gray cards
			case (x+y)%23 == 0:
				refl = [3]float64{0.85, 0.15, 0.2}
			case (x*3+y)%29 == 0:
				refl = [3]float64{0.15, 0.2, 0.85}
			case y < 12:
				refl = [3]float64{1, 1, 1} // blown highlight band
			default:
				v := 0.15 + 0.55*float64((x*7+y*13)%19)/19
				refl = [3]float64{v, v, v}
			}
			o := (y*w + x) * 4
			img.Pix[o+0] = srgbEnc(refl[0] * ill[0])
			img.Pix[o+1] = srgbEnc(refl[1] * ill[1])
			img.Pix[o+2] = srgbEnc(refl[2] * ill[2])
			img.Pix[o+3] = 255
		}
	}
	f, _ := os.Create(os.Args[1])
	defer f.Close()
	png.Encode(f, img)
	_ = color.Black
}
