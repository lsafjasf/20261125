package nightwb

const (
	clipHighFracThreshold = 0.05
	crushFracThreshold    = 0.10
	crushChannelMax       = 0.02
)

// AnalyzeExposure summarizes clipping/crushing statistics of the source frame.
func AnalyzeExposure(f *Frame) ExposureReport {
	n := len(f.RGB)
	var clipped, crushed, usable int
	for i := range f.RGB {
		isClip := f.ChannelClipped[i][0] || f.ChannelClipped[i][1] || f.ChannelClipped[i][2]
		isCrush := f.RGB[i][0] <= crushChannelMax &&
			f.RGB[i][1] <= crushChannelMax && f.RGB[i][2] <= crushChannelMax
		if isClip {
			clipped++
		}
		if isCrush {
			crushed++
		}
		if !isClip && !isCrush {
			usable++
		}
	}
	rep := ExposureReport{
		HighlightClippedFrac: float64(clipped) / float64(n),
		ShadowCrushedFrac:    float64(crushed) / float64(n),
		UsableFrac:           float64(usable) / float64(n),
		ExposureState:        "normal",
	}
	if rep.HighlightClippedFrac > clipHighFracThreshold {
		rep.ExposureState = "overexposed"
	} else if rep.ShadowCrushedFrac > crushFracThreshold {
		rep.ExposureState = "underexposed"
	}
	return rep
}

// AnalyzeRecoverability flags pixels whose true color cannot be guaranteed
// after adaptation: clipped channels, crushed shadows, adapted out-of-gamut
// values, mixed-illuminant contested pixels, and low-confidence regions.
func AnalyzeRecoverability(f *Frame, adapted *AdaptedFrame, est EstimateResult,
	ill Illuminant) ([][]uint16, RecoverabilityStats) {
	n := len(f.RGB)
	masks := make([][]uint16, 2)
	for i := range masks {
		masks[i] = make([]uint16, n)
	}
	stats := RecoverabilityStats{FlagCounts: map[uint16]int{}}

	var clippedN, crushedN, gamutN, mixedN, confN, anyN int
	for i := range f.RGB {
		var flag uint16
		if f.ChannelClipped[i][0] {
			flag |= FlagClippedRed
		}
		if f.ChannelClipped[i][1] {
			flag |= FlagClippedGreen
		}
		if f.ChannelClipped[i][2] {
			flag |= FlagClippedBlue
		}
		if f.RGB[i][0] <= crushChannelMax && f.RGB[i][1] <= crushChannelMax &&
			f.RGB[i][2] <= crushChannelMax {
			flag |= FlagCrushed
		}
		if adapted.OutOfGamut[i] {
			flag |= FlagOutOfGamut
		}
		if ill.MixedLight && est.MixedMembership[i] > 0.25 {
			flag |= FlagMixedLight
		}
		if ill.Confidence < 0.35 {
			flag |= FlagLowConfidence
		}
		if flag != 0 {
			anyN++
			stats.FlagCounts[flag]++
		}
		if flag&(FlagClippedRed|FlagClippedGreen|FlagClippedBlue) != 0 {
			clippedN++
		}
		if flag&FlagCrushed != 0 {
			crushedN++
		}
		if flag&FlagOutOfGamut != 0 {
			gamutN++
		}
		if flag&FlagMixedLight != 0 {
			mixedN++
		}
		if flag&FlagLowConfidence != 0 {
			confN++
		}
		// Mask 0 carries source-side defects; mask 1 the adaptation-side ones.
		masks[0][i] = flag & (FlagClippedRed | FlagClippedGreen | FlagClippedBlue | FlagCrushed)
		masks[1][i] = flag & (FlagOutOfGamut | FlagMixedLight | FlagLowConfidence)
	}
	fn := float64(n)
	stats.ClippedFrac = float64(clippedN) / fn
	stats.CrushedFrac = float64(crushedN) / fn
	stats.OutOfGamutFrac = float64(gamutN) / fn
	stats.MixedLightFrac = float64(mixedN) / fn
	stats.LowConfFrac = float64(confN) / fn
	stats.AnyFlagFrac = float64(anyN) / fn
	return masks, stats
}
