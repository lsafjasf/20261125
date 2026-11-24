package colorcal

import "math"

// NormalizeChart converts raw integer camera units to normalized [0,1] when
// needed. Normalized charts (all measured channels <= 1 and saturation <= 1)
// are returned unchanged. The input is not mutated.
func NormalizeChart(input Chart) Chart {
	out := input
	out.Patches = append([]Patch(nil), input.Patches...)

	maxMeasured := 0.0
	for _, patch := range input.Patches {
		for _, value := range patch.MeasuredRGB {
			maxMeasured = math.Max(maxMeasured, value)
		}
	}

	scale := 0.0
	switch {
	case input.SaturationLevel > 1:
		scale = input.SaturationLevel
	case maxMeasured > 1 && input.BitDepth >= 2 && input.BitDepth <= 31:
		scale = float64((uint64(1) << input.BitDepth) - 1)
	case maxMeasured > 1:
		scale = maxMeasured
	}
	if scale <= 1 {
		return out
	}

	if input.SaturationLevel > 1 {
		out.SaturationLevel = input.SaturationLevel / scale
	} else {
		out.SaturationLevel = 1
	}
	if input.BlackLevel > 1 {
		out.BlackLevel = input.BlackLevel / scale
	}

	for i := range out.Patches {
		for c := 0; c < 3; c++ {
			out.Patches[i].MeasuredRGB[c] /= scale
			if out.Patches[i].SigmaRGB[c] > 0 {
				out.Patches[i].SigmaRGB[c] /= scale
			}
		}
	}
	return out
}
