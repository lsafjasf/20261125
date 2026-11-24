package nightwb

import "math"

// Frame is a decoded image in linear-light sRGB plus per-pixel clipping flags.
type Frame struct {
	Width, Height int
	RGB           [][3]float64 // linear, length Width*Height
	// ChannelClipped marks channels that were at the encoding maximum in the
	// source; clipping destroys color information and excludes the sample.
	ChannelClipped [][3]bool
	// ChannelCrushed channels are at the encoding minimum.
	ChannelCrushed [][3]bool
}

// EstimateResult bundles the illuminant with spatial evidence used later for
// the recoverability mask.
type EstimateResult struct {
	Illuminant      Illuminant
	MixedMembership []float64 // per pixel, 0..1 assignment to the secondary cluster
	NeutralFrac     float64
}

const (
	neutralChromaTol   = 0.08 // |cR|,|cB| for full-weight neutral samples
	candidateChromaTol = 0.22 // wider pool admitted for clustering/fallback
	neutralLumaMin     = 0.05
	neutralLumaMax     = 0.92
	neutralMinCount    = 64
	mixedSplitThresh   = 0.045 // cluster separation in chromaticity space
	mixedLumaThresh    = 0.12  // cluster separation in linear luma
	mixedMinWeight     = 0.12
)

// EstimateIlluminant estimates the scene illuminant from neutral samples.
//
// Candidate neutral pixels (low chroma, usable luma, no clipping) are
// aggregated by weighted median chromaticity, which is robust to the
// saturated neon/monochromatic reflectors common in night footage. When the
// candidate chromaticity distribution is bimodal (sodium + neon / tungsten +
// daylight mix) both modes are reported and the dominant mode is used.
// Gray-world and white-patch cross-checks bound the estimate; strong
// disagreement lowers confidence and triggers a regularized fusion. With too
// few candidates (monochromatic frame) it falls back to gray-world explicitly.
func EstimateIlluminant(f *Frame) EstimateResult {
	n := len(f.RGB)
	res := EstimateResult{MixedMembership: make([]float64, n)}

	cands := make([]neutralCand, 0, n/8)

	gw := [3]float64{} // gray world over usable pixels
	gwCount := 0
	wp := [3]float64{} // white patch: per-channel maxima

	for i, px := range f.RGB {
		lum := 0.2126*px[0] + 0.7152*px[1] + 0.0722*px[2]
		if lum > neutralLumaMin && lum < neutralLumaMax &&
			!(f.ChannelClipped[i][0] || f.ChannelClipped[i][1] || f.ChannelClipped[i][2]) {
			gw[0] += px[0]
			gw[1] += px[1]
			gw[2] += px[2]
			gwCount++
			for c := 0; c < 3; c++ {
				if px[c] > wp[c] {
					wp[c] = px[c]
				}
			}
		} else if lum >= neutralLumaMax ||
			(f.ChannelClipped[i][0] || f.ChannelClipped[i][1] || f.ChannelClipped[i][2]) {
			for c := 0; c < 3; c++ {
				if px[c] > wp[c] {
					wp[c] = px[c]
				}
			}
		}
		if lum <= neutralLumaMin || lum >= neutralLumaMax {
			continue
		}
		if f.ChannelClipped[i][0] || f.ChannelClipped[i][1] || f.ChannelClipped[i][2] {
			continue
		}
		cR, cB := chroma(px)
		if math.Abs(cR) <= candidateChromaTol && math.Abs(cB) <= candidateChromaTol {
			// Near-neutral samples dominate the weighted median; wider-pool
			// samples keep small weight but enable bimodal detection.
			chromaScore := math.Max(math.Abs(cR), math.Abs(cB))
			w := math.Max(0.05, 1-chromaScore/neutralChromaTol) *
				math.Min(1, lum/0.35) * math.Min(1, (1-lum)/0.35)
			if w > 0 {
				cands = append(cands, neutralCand{idx: i, lum: lum, cR: cR, cB: cB, w: w})
			}
		}
	}
	res.NeutralFrac = float64(len(cands)) / float64(n)

	ill := Illuminant{}
	if gwCount > 0 {
		gw[0] /= float64(gwCount)
		gw[1] /= float64(gwCount)
		gw[2] /= float64(gwCount)
	}

	if len(cands) < neutralMinCount {
		return monochromaticFallback(res, ill, gw, len(cands))
	}

	cRv := make([]float64, len(cands))
	cBv := make([]float64, len(cands))
	wv := make([]float64, len(cands))
	for i, c := range cands {
		cRv[i], cBv[i], wv[i] = c.cR, c.cB, c.w
	}
	medR := weightedMedian(cRv, wv)
	medB := weightedMedian(cBv, wv)

	clusters, mixed := detectMixed(cands, res.MixedMembership)
	ill.MixedLight = mixed
	ill.Clusters = clusters

	var chosen [2]float64
	if mixed {
		if clusters[0].Weight >= clusters[1].Weight {
			chosen = clusters[0].MeanChroma
		} else {
			chosen = clusters[1].MeanChroma
		}
		ill.Warnings = append(ill.Warnings,
			"mixed illuminants: a single global adaptation cannot neutralize both modes")
	} else {
		chosen = [2]float64{medR, medB}
	}

	ill.RGB = normalizeMax(chromaToRGB(chosen[0], chosen[1]))
	ill.SampleCount = len(cands)
	ill.Method = "neutral-sample-median"

	gwNorm := normalizeMax(gw)
	wpNorm := normalizeMax(wp)
	conf := estimateConfidence(ill.RGB, gwNorm, wpNorm, len(cands), n)
	if conf < 0.35 {
		ill.Warnings = append(ill.Warnings,
			"neutral samples disagree with gray-world/white-patch; low confidence, fusing with gray world")
		for c := 0; c < 3; c++ {
			ill.RGB[c] = 0.5*ill.RGB[c] + 0.5*gwNorm[c]
		}
		ill.RGB = normalizeMax(ill.RGB)
		ill.Method = "neutral+grayworld-fusion"
	}
	ill.Confidence = conf

	xyz := SRGBToXYZ.mulVec(ill.RGB)
	ill.XYZ = xyz
	ill.CCT, ill.Tint = XYZToCCT(xyz[0], xyz[1], xyz[2])

	res.Illuminant = ill
	return res
}

func chroma(px [3]float64) (cR, cB float64) {
	sum := px[0] + px[1] + px[2]
	if sum <= 1e-12 {
		return 0, 0
	}
	return px[0]/sum - 1.0/3.0, px[2]/sum - 1.0/3.0
}

// chromaToRGB reconstructs an RGB ratio from (cR,cB), fixing G so the
// channels sum to 1.
func chromaToRGB(cR, cB float64) [3]float64 {
	r := cR + 1.0/3.0
	b := cB + 1.0/3.0
	g := 1 - r - b
	if g < 1e-6 {
		g = 1e-6
	}
	return [3]float64{r, g, b}
}

func normalizeMax(v [3]float64) [3]float64 {
	m := math.Max(v[0], math.Max(v[1], v[2]))
	if m <= 1e-12 {
		return [3]float64{1, 1, 1}
	}
	return [3]float64{v[0] / m, v[1] / m, v[2] / m}
}

func estimateConfidence(est, gw, wp [3]float64, count, total int) float64 {
	sampleScore := math.Min(1, (float64(count)/float64(total))/0.08)
	dgw := rgbDistance(est, gw)
	dwp := rgbDistance(est, wp)
	agree := 1 - math.Min(1, math.Min(dgw, dwp)/0.35)
	return clamp01(0.45*sampleScore + 0.55*agree)
}

func rgbDistance(a, b [3]float64) float64 {
	var d float64
	for c := 0; c < 3; c++ {
		d += (a[c] - b[c]) * (a[c] - b[c])
	}
	return math.Sqrt(d / 3)
}

func monochromaticFallback(res EstimateResult, ill Illuminant,
	gw [3]float64, candCount int) EstimateResult {
	ill.Method = "grayworld-fallback"
	ill.SampleCount = candCount
	ill.Confidence = 0.15
	ill.Warnings = append(ill.Warnings,
		"insufficient neutral samples (monochromatic reflectance or uniform tint); gray world used, result unreliable")
	ill.RGB = normalizeMax(gw)
	xyz := SRGBToXYZ.mulVec(ill.RGB)
	ill.XYZ = xyz
	ill.CCT, ill.Tint = XYZToCCT(xyz[0], xyz[1], xyz[2])
	res.Illuminant = ill
	return res
}
