package pipeline

import (
	"math"

	"rec2020-to-rec709-shoulder/internal/colorconv"
)

const (
	VariantHard     = "hard_clip"
	VariantFixed    = "fixed_shoulder"
	VariantAdaptive = "adaptive_shoulder"
)

type RenderParams struct {
	Variant     string
	Knee        float64
	PreserveHue bool
	Transfer    string
	SignalRange string
}

// GamutCompress removes negative Rec.709 channels by scaling RGB deviations
// around Rec.709 luma. Luma stays fixed and saturated colors move to the
// reproducible gamut boundary before highlight mapping.
func GamutCompress(c colorconv.RGB) colorconv.RGB {
	y := colorconv.Luminance709(c)
	minimum := math.Min(c.R, math.Min(c.G, c.B))
	if minimum >= 0 {
		return c
	}
	scale := 1.0
	if y-minimum > 1e-12 {
		scale = y / (y - minimum)
	}
	return colorconv.RGB{
		R: y + scale*(c.R-y),
		G: y + scale*(c.G-y),
		B: y + scale*(c.B-y),
	}
}

// SoftShoulder is C1-continuous at knee and approaches 1 asymptotically.
// For x > knee: y = 1 - (1-knee)*exp(-(x-knee)/(1-knee)).
func SoftShoulder(x, knee float64) float64 {
	if x <= knee {
		return x
	}
	headroom := 1 - knee
	if headroom <= 1e-6 {
		return 1
	}
	return 1 - headroom*math.Exp(-(x-knee)/headroom)
}

func MapWorkingColor(c colorconv.RGB, p RenderParams) colorconv.RGB {
	c = GamutCompress(c)
	switch p.Variant {
	case VariantHard:
		return colorconv.RGB{R: clamp01(c.R), G: clamp01(c.G), B: clamp01(c.B)}
	default:
		knee := p.Knee
		if knee <= 0 || knee >= 1 {
			knee = 0.75
		}
		if p.PreserveHue {
			maximum := math.Max(c.R, math.Max(c.G, c.B))
			if maximum <= 0 {
				return c
			}
			mapped := SoftShoulder(maximum, knee)
			scale := mapped / maximum
			return colorconv.RGB{R: c.R * scale, G: c.G * scale, B: c.B * scale}
		}
		return colorconv.RGB{
			R: SoftShoulder(c.R, knee),
			G: SoftShoulder(c.G, knee),
			B: SoftShoulder(c.B, knee),
		}
	}
}

func RenderFrame(src *FloatImage, p RenderParams) *FloatImage {
	out := NewFloatImage(src.Width, src.Height)
	for i := 0; i < len(src.Pix); i += 3 {
		c := colorconv.RGB{R: src.Pix[i], G: src.Pix[i+1], B: src.Pix[i+2]}
		c = colorconv.ConvertRec2020To709(c)
		c = MapWorkingColor(c, p)
		out.Pix[i] = clamp01(c.R)
		out.Pix[i+1] = clamp01(c.G)
		out.Pix[i+2] = clamp01(c.B)
	}
	return out
}
