package analyze

import (
	"math"
	"sort"

	"hdrmeta/pkg/pq"
)

// BT.2020 luma coefficients (non-constant luminance, R'G'B' -> Y').
const (
	coefR = 0.2627
	coefG = 0.6780
	coefB = 0.0593
)

// lumaNits converts one pixel's PQ code values to luminance in nits using
// the BT.2020 Y' weighting. Each channel is linearized by the PQ EOTF first,
// then combined in the linear-light domain.
func lumaNits(r, g, b uint32, depth int) float64 {
	return coefR*pq.CodeToNits(int(r), depth) +
		coefG*pq.CodeToNits(int(g), depth) +
		coefB*pq.CodeToNits(int(b), depth)
}

// pixelPeakNits returns the maximum per-channel luminance (hardest hit the
// display can receive), useful as a conservative secondary metric.
func pixelPeakNits(r, g, b uint32, depth int) float64 {
	m := pq.CodeToNits(int(r), depth)
	if v := pq.CodeToNits(int(g), depth); v > m {
		m = v
	}
	if v := pq.CodeToNits(int(b), depth); v > m {
		m = v
	}
	return m
}

// AnalyzeFrame computes per-frame luminance statistics.
//
// highlightNits is the threshold (nits) for connected-region extraction;
// clipNits is the display clip point used for clipped-pixel accounting.
func AnalyzeFrame(f *Frame, highlightNits, clipNits, minAreaRatio float64) FrameStats {
	n := f.PixelCount()
	lum := make([]float64, n)

	st := FrameStats{Name: f.Name}
	sum := 0.0
	for i := 0; i < n; i++ {
		y := lumaNits(f.R[i], f.G[i], f.B[i], f.BitDepth)
		lum[i] = y
		sum += y
		if y > st.MaxNits {
			st.MaxNits = y
			st.MaxCode = maxCode(f, i)
		}
		switch {
		case y > 4000:
			st.Over4000++
			fallthrough
		case y > 2000:
			st.Over2000++
			fallthrough
		case y > 1000:
			st.Over1000++
		}
		if y > clipNits {
			st.ClipPixels++
		}
	}
	st.MeanNits = sum / float64(n)
	st.ClipRatio = float64(st.ClipPixels) / float64(n)
	st.P99_99Nits = topFractionMean(lum, 0.0001)
	st.P99_9Nits = topFractionMean(lum, 0.001)
	st.Regions = findRegions(f, lum, highlightNits, minAreaRatio)
	return st
}

func maxCode(f *Frame, i int) int {
	m := int(f.R[i])
	if int(f.G[i]) > m {
		m = int(f.G[i])
	}
	if int(f.B[i]) > m {
		m = int(f.B[i])
	}
	return m
}

// topFractionMean returns the mean of the brightest fraction (e.g. 0.0001 =
// brightest 0.01%) of luminance values. At least one pixel is included.
func topFractionMean(lum []float64, fraction float64) float64 {
	k := int(math.Ceil(float64(len(lum)) * fraction))
	if k < 1 {
		k = 1
	}
	// Partial sort via full sort; frame sizes here are modest and this keeps
	// the code dependency-free and easy to review.
	sorted := make([]float64, len(lum))
	copy(sorted, lum)
	sort.Float64s(sorted)
	sum := 0.0
	start := len(sorted) - k
	for _, v := range sorted[start:] {
		sum += v
	}
	return sum / float64(k)
}
