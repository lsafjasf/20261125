package pipeline

import (
	"math"

	"rec2020-to-rec709-shoulder/internal/colorconv"
)

func HorizontalStrip(images []*FloatImage, gap int) *FloatImage {
	if len(images) == 0 {
		return NewFloatImage(1, 1)
	}
	h := images[0].Height
	w := images[0].Width
	out := NewFloatImage(len(images)*w+(len(images)+1)*gap, h+2*gap)
	fill(out, colorconv.RGB{R: 0.018, G: 0.018, B: 0.022})
	for k, im := range images {
		x0 := gap + k*(w+gap)
		blit(out, im, x0, gap)
	}
	return out
}

func HueErrorStrip(src *FloatImage, outputs []*FloatImage, maxDegrees float64) *FloatImage {
	maps := make([]*FloatImage, 0, len(outputs))
	for _, out := range outputs {
		heat := NewFloatImage(src.Width, src.Height)
		for y := 0; y < src.Height; y++ {
			for x := 0; x < src.Width; x++ {
				i := (y*src.Width + x) * 3
				sc := src.Pixel(x, y)
				oc := out.Pixel(x, y)
				sl := colorconv.Rec2020ToLab(sc)
				ol := colorconv.RGBToLab(oc)
				value := 0.0
				if colorconv.Chroma(sl) >= 10 && colorconv.Chroma(ol) >= 10 {
					delta := math.Abs(colorconv.HueDelta(colorconv.HueDegrees(sl), colorconv.HueDegrees(ol)))
					value = delta / maxDegrees
				} else if colorconv.Chroma(sl) >= 18 && colorconv.Chroma(ol) < 8 {
					value = 1.15
				} else {
					value = -1
				}
				if value < 0 {
					heat.Pix[i], heat.Pix[i+1], heat.Pix[i+2] = 0.04, 0.04, 0.05
				} else {
					c := falseColor(math.Min(1, value))
					if value > 1 {
						c = colorconv.RGB{R: 0.8, G: 0.1, B: 0.9}
					}
					heat.Pix[i], heat.Pix[i+1], heat.Pix[i+2] = c.R, c.G, c.B
				}
			}
		}
		maps = append(maps, heat)
	}
	return HorizontalStrip(maps, 4)
}

func TextureStrip(outputs []*FloatImage) *FloatImage {
	maps := make([]*FloatImage, 0, len(outputs))
	for _, out := range outputs {
		w, h := out.Width, out.Height
		luma := make([]float64, w*h)
		for i := 0; i < w*h; i++ {
			luma[i] = colorconv.Luminance709(colorconv.RGB{R: out.Pix[i*3], G: out.Pix[i*3+1], B: out.Pix[i*3+2]})
		}
		tex := NewFloatImage(w, h)
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				idx := y*w + x
				g := sobel(luma, w, idx)
				v := math.Min(1, g/0.22)
				i := idx * 3
				tex.Pix[i], tex.Pix[i+1], tex.Pix[i+2] = v*0.85, v*0.95, v
			}
		}
		maps = append(maps, tex)
	}
	return HorizontalStrip(maps, 4)
}

func ClipStrip(outputs []*FloatImage) *FloatImage {
	maps := make([]*FloatImage, 0, len(outputs))
	for _, out := range outputs {
		clip := out.Clone()
		for i := 0; i < out.Width*out.Height; i++ {
			r, g, b := out.Pix[i*3], out.Pix[i*3+1], out.Pix[i*3+2]
			m := math.Max(r, math.Max(g, b))
			if m >= 0.998 {
				clip.Pix[i*3], clip.Pix[i*3+1], clip.Pix[i*3+2] = 1, 0.05, 0.02
			}
		}
		maps = append(maps, clip)
	}
	return HorizontalStrip(maps, 4)
}

func falseColor(t float64) colorconv.RGB {
	switch {
	case t < 0.25:
		q := t / 0.25
		return colorconv.RGB{R: 0.05 * q, G: 0.15 + 0.55*q, B: 0.35 + 0.65*q}
	case t < 0.5:
		q := (t - 0.25) / 0.25
		return colorconv.RGB{R: 0.15 + 0.25*q, G: 0.70 + 0.20*q, B: 1 - 0.55*q}
	case t < 0.75:
		q := (t - 0.5) / 0.25
		return colorconv.RGB{R: 0.40 + 0.60*q, G: 0.90 - 0.10*q, B: 0.45 - 0.45*q}
	default:
		q := (t - 0.75) / 0.25
		return colorconv.RGB{R: 1, G: 0.80 - 0.65*q, B: 0.0 + 0.05*q}
	}
}

func fill(im *FloatImage, c colorconv.RGB) {
	for i := 0; i < len(im.Pix); i += 3 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2] = c.R, c.G, c.B
	}
}

func blit(dst, src *FloatImage, x0, y0 int) {
	for y := 0; y < src.Height; y++ {
		dy := y0 + y
		if dy < 0 || dy >= dst.Height {
			continue
		}
		for x := 0; x < src.Width; x++ {
			dx := x0 + x
			if dx < 0 || dx >= dst.Width {
				continue
			}
			si := (y*src.Width + x) * 3
			di := (dy*dst.Width + dx) * 3
			dst.Pix[di], dst.Pix[di+1], dst.Pix[di+2] = src.Pix[si], src.Pix[si+1], src.Pix[si+2]
		}
	}
}
