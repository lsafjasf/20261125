package pipeline

import (
	"math"
	"sort"

	"rec2020-to-rec709-shoulder/internal/colorconv"
)

type FrameStats struct {
	Frame                 int       `json:"frame"`
	Width                 int       `json:"width"`
	Height                int       `json:"height"`
	Pixels                int       `json:"pixels"`
	MeanLuma              float64   `json:"mean_luma"`
	P95Max                float64   `json:"p95_max_rgb"`
	P995Max               float64   `json:"p995_max_rgb"`
	FractionOver1         float64   `json:"fraction_max_rgb_gt_1"`
	FractionNearHighlight float64   `json:"fraction_max_rgb_gt_0p75"`
	FractionGamutNegative float64   `json:"fraction_raw_709_negative"`
	HighlightPixels       float64   `json:"highlight_pixels"`
	GlobalGradientMean    float64   `json:"global_gradient_mean"`
	HighlightGradientMean float64   `json:"highlight_gradient_mean"`
	HighlightDetailRatio  float64   `json:"highlight_detail_ratio"`
	LumaHistogram         []float64 `json:"luma_histogram"`
}

const histBins = 24

func AnalyzeFrame(src *FloatImage, frame int) FrameStats {
	w, h := src.Width, src.Height
	n := w * h
	luma := make([]float64, n)
	maxRGB := make([]float64, n)
	histCounts := make([]int, histBins)
	values := make([]float64, 0, n)

	var sumLuma float64
	var over1, near, gamutNegative int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			c2020 := src.Pixel(x, y)
			raw := colorconv.ConvertRec2020To709(c2020)
			if math.Min(raw.R, math.Min(raw.G, raw.B)) < -1e-5 {
				gamutNegative++
			}
			c := GamutCompress(raw)
			l := colorconv.Luminance709(c)
			m := math.Max(c.R, math.Max(c.G, c.B))
			luma[idx] = l
			maxRGB[idx] = m
			values = append(values, m)
			sumLuma += l
			if m > 1 {
				over1++
			}
			if m > 0.75 {
				near++
			}
			bin := int(math.Min(float64(histBins-1), math.Max(0, l/2.0)*histBins))
			histCounts[bin]++
		}
	}

	sort.Float64s(values)
	stats := FrameStats{
		Frame:                 frame,
		Width:                 w,
		Height:                h,
		Pixels:                n,
		MeanLuma:              sumLuma / float64(n),
		P95Max:                percentileSorted(values, 0.95),
		P995Max:               percentileSorted(values, 0.995),
		FractionOver1:         float64(over1) / float64(n),
		FractionNearHighlight: float64(near) / float64(n),
		FractionGamutNegative: float64(gamutNegative) / float64(n),
		LumaHistogram:         make([]float64, histBins),
	}
	for i, count := range histCounts {
		stats.LumaHistogram[i] = float64(count) / float64(n)
	}

	var globalEnergy, highlightEnergy float64
	var highlightCount int
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			idx := y*w + x
			gx := -luma[idx-w-1] + luma[idx-w+1] +
				-2*luma[idx-1] + 2*luma[idx+1] +
				-luma[idx+w-1] + luma[idx+w+1]
			gy := -luma[idx-w-1] - 2*luma[idx-w] - luma[idx-w+1] +
				luma[idx+w-1] + 2*luma[idx+w] + luma[idx+w+1]
			energy := math.Hypot(gx, gy)
			globalEnergy += energy
			if maxRGB[idx] > 0.75 {
				highlightEnergy += energy
				highlightCount++
			}
		}
	}
	interior := float64((w - 2) * (h - 2))
	stats.GlobalGradientMean = globalEnergy / interior
	if highlightCount > 0 {
		stats.HighlightGradientMean = highlightEnergy / float64(highlightCount)
		stats.HighlightPixels = float64(highlightCount) / interior
		stats.HighlightDetailRatio = stats.HighlightGradientMean / math.Max(stats.GlobalGradientMean, 1e-6)
	}
	return stats
}

func percentileSorted(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func HistogramDistance(a, b []float64) float64 {
	n := int(math.Min(float64(len(a)), float64(len(b))))
	var total float64
	for i := 0; i < n; i++ {
		total += math.Abs(a[i] - b[i])
	}
	return total / 2
}
