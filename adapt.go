package nightwb

import "math"

// AdaptMatrix builds a global linear-sRGB adaptation matrix that maps the
// estimated illuminant white toward the D65 reference white using the
// Bradford von Kries transform. strength interpolates between identity and
// the full transform (conservative scheme uses partial strength).
func AdaptMatrix(srcWhiteRGB [3]float64, strength float64) Mat3 {
	if strength <= 0 {
		return Mat3{1, 0, 0, 0, 1, 0, 0, 0, 1}
	}
	srcXYZ := SRGBToXYZ.mulVec(srcWhiteRGB)
	dstXYZ := D65XYZ

	srcCone := Bradford.mulVec(srcXYZ)
	dstCone := Bradford.mulVec(dstXYZ)
	gain := [3]float64{
		dstCone[0] / srcCone[0],
		dstCone[1] / srcCone[1],
		dstCone[2] / srcCone[2],
	}
	gainMat := Mat3{
		gain[0], 0, 0,
		0, gain[1], 0,
		0, 0, gain[2],
	}
	// full: XYZ' = BradfordInv * gain * Bradford * XYZ
	fullXYZ := mulMat(BradfordInv, mulMat(gainMat, Bradford))
	fullRGB := mulMat(XYZToSRGB, mulMat(fullXYZ, SRGBToXYZ))

	if strength >= 1 {
		return fullRGB
	}
	// Linear blend with identity; stable for partial correction.
	id := Mat3{1, 0, 0, 0, 1, 0, 0, 0, 1}
	var blended Mat3
	for i := range blended {
		blended[i] = id[i]*(1-strength) + fullRGB[i]*strength
	}
	return blended
}

// AdaptedFrame is the output of an adaptation pass.
type AdaptedFrame struct {
	RGB        [][3]float64 // adapted linear values, may leave [0,1]
	OutOfGamut []bool       // any adapted channel outside [0,1]
	Headroom   []float64    // min distance to 1 across channels, pre-adaptation
}

// ApplyAdaptation runs the scheme over the frame. The aggressive scheme
// applies one global matrix per pixel. The conservative scheme additionally
// relaxes adaptation in bright highlight pixels so clipped sodium/neon
// highlights are not pushed into unnatural magenta/cyan blocks.
func ApplyAdaptation(f *Frame, ill Illuminant, s Scheme) *AdaptedFrame {
	n := len(f.RGB)
	base := AdaptMatrix(ill.RGB, s.Strength)
	id := Mat3{1, 0, 0, 0, 1, 0, 0, 0, 1}

	out := &AdaptedFrame{
		RGB:        make([][3]float64, n),
		OutOfGamut: make([]bool, n),
		Headroom:   make([]float64, n),
	}
	for i, px := range f.RGB {
		m := base
		if s.DesatLimit > 0 {
			lum := 0.2126*px[0] + 0.7152*px[1] + 0.0722*px[2]
			// Knee: above DesatLimit start blending toward identity; the
			// HighlightKnee width controls how abruptly protection ramps in.
			t := clamp01((lum - s.DesatLimit) / math.Max(s.HighlightKnee, 1e-6))
			if t > 0 {
				var relaxed Mat3
				for k := range relaxed {
					relaxed[k] = base[k]*(1-t) + id[k]*t
				}
				m = relaxed
			}
		}
		adapted := m.mulVec(px)
		out.RGB[i] = adapted
		out.Headroom[i] = math.Min(math.Min(1-px[0], 1-px[1]), 1-px[2])
		if adapted[0] < 0 || adapted[1] < 0 || adapted[2] < 0 ||
			adapted[0] > 1 || adapted[1] > 1 || adapted[2] > 1 {
			out.OutOfGamut[i] = true
		}
	}
	return out
}
