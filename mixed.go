package nightwb

import "math"

// neutralCand is one candidate neutral sample.
type neutralCand struct {
	idx int
	lum float64
	cR  float64
	cB  float64
	w   float64
}

// detectMixed splits neutral candidates into two chromaticity modes using
// weighted 2D k-means over (luma, red-cyan chromaticity). Neutral surfaces
// under different illuminants share normalized chromaticity after the
// reflectance ratio cancels, so the illuminant split is carried by the luma
// axis; the chromaticity axis separates genuinely different tints.
func detectMixed(cands []neutralCand, membership []float64) ([]Cluster, bool) {
	if len(cands) < 2*neutralMinCount {
		return nil, false
	}
	minL, maxL := cands[0].lum, cands[0].lum
	minR, maxR := cands[0].cR, cands[0].cR
	var totalW float64
	for _, c := range cands {
		minL = math.Min(minL, c.lum)
		maxL = math.Max(maxL, c.lum)
		minR = math.Min(minR, c.cR)
		maxR = math.Max(maxR, c.cR)
		totalW += c.w
	}
	dL := maxL - minL
	dR := maxR - minR
	if dL < mixedLumaThresh && dR < mixedSplitThresh {
		return nil, false
	}

	// Normalize axes so k-means distances compare luma and chroma on one scale.
	sL := math.Max(dL, 1e-9)
	sR := math.Max(dR, 1e-9)
	kL0, kL1 := minL, maxL
	kR0, kR1 := minR, maxR
	var assign []int
	for iter := 0; iter < 25; iter++ {
		var newAssign = make([]int, len(cands))
		sumL := [2]float64{}
		sumR := [2]float64{}
		wsum := [2]float64{}
		for i, c := range cands {
			d0 := math.Hypot((c.lum-kL0)/sL, (c.cR-kR0)/sR)
			d1 := math.Hypot((c.lum-kL1)/sL, (c.cR-kR1)/sR)
			a := 0
			if d1 < d0 {
				a = 1
			}
			newAssign[i] = a
			sumL[a] += c.lum * c.w
			sumR[a] += c.cR * c.w
			wsum[a] += c.w
		}
		nkL0, nkL1 := kL0, kL1
		nkR0, nkR1 := kR0, kR1
		if wsum[0] > 0 {
			nkL0 = sumL[0] / wsum[0]
			nkR0 = sumR[0] / wsum[0]
		}
		if wsum[1] > 0 {
			nkL1 = sumL[1] / wsum[1]
			nkR1 = sumR[1] / wsum[1]
		}
		if math.Abs(nkL0-kL0) < 1e-5 && math.Abs(nkL1-kL1) < 1e-5 &&
			math.Abs(nkR0-kR0) < 1e-5 && math.Abs(nkR1-kR1) < 1e-5 {
			break
		}
		kL0, kL1, kR0, kR1 = nkL0, nkL1, nkR0, nkR1
		assign = newAssign
	}

	var wSum [2]float64
	var cRMean, cBMean [2]float64
	var rgbSum [2][3]float64
	var lumMean [2]float64
	for i, c := range cands {
		a := assign[i]
		wSum[a] += c.w
		cRMean[a] += c.cR * c.w
		cBMean[a] += c.cB * c.w
		lumMean[a] += c.lum * c.w
		rgb := chromaToRGB(c.cR, c.cB)
		for ch := 0; ch < 3; ch++ {
			rgbSum[a][ch] += rgb[ch] * c.w
		}
	}
	if wSum[0] == 0 || wSum[1] == 0 {
		return nil, false
	}
	minor := math.Min(wSum[0], wSum[1]) / totalW
	sepL := math.Abs(kL0-kL1) / sL
	sepR := math.Abs(kR0-kR1) / sR
	separation := math.Hypot(sepL, sepR)
	if minor < mixedMinWeight ||
		(math.Abs(lumMean[0]-lumMean[1]) < mixedLumaThresh &&
			math.Abs(kR0-kR1) < mixedSplitThresh) {
		return nil, false
	}

	clusters := make([]Cluster, 2)
	for a := 0; a < 2; a++ {
		meanR := cRMean[a] / wSum[a]
		meanB := cBMean[a] / wSum[a]
		rgb := normalizeMax([3]float64{
			rgbSum[a][0] / wSum[a],
			rgbSum[a][1] / wSum[a],
			rgbSum[a][2] / wSum[a],
		})
		xyz := SRGBToXYZ.mulVec(rgb)
		cct, _ := XYZToCCT(xyz[0], xyz[1], xyz[2])
		clusters[a] = Cluster{
			Weight:     wSum[a] / totalW,
			MeanRGB:    rgb,
			CCT:        cct,
			MeanChroma: [2]float64{meanR, meanB},
		}
	}

	// Per-pixel soft membership of the secondary mode: hard assignment with
	// distance-based edge softening, used only for the recoverability mask.
	for i, c := range cands {
		if assign[i] == 1 {
			d := math.Hypot((c.lum-kL1)/sL, (c.cR-kR1)/sR)
			membership[c.idx] = clamp01(1 - d/math.Max(separation, 1e-9))
		}
	}
	return clusters, true
}
