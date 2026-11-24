package pipeline

import (
	"math"
	"sort"

	"rec2020-to-rec709-shoulder/internal/colorconv"
)

type FrameMetrics struct {
	Frame                     int     `json:"frame"`
	Variant                   string  `json:"variant"`
	Pixels                    int     `json:"pixels"`
	HueEligiblePixels         int     `json:"hue_eligible_pixels"`
	HueMeanAbsDegrees         float64 `json:"hue_mean_abs_degrees"`
	HueP95AbsDegrees          float64 `json:"hue_p95_abs_degrees"`
	HueMaxAbsDegrees          float64 `json:"hue_max_abs_degrees"`
	HueOver5Pct               float64 `json:"hue_over_5deg_pct"`
	HueOver10Pct              float64 `json:"hue_over_10deg_pct"`
	DesaturatedPct            float64 `json:"desaturated_pct"`
	MeanDeltaE76              float64 `json:"mean_delta_e76"`
	SourceMeanLuma            float64 `json:"source_mean_luma"`
	OutputMeanLuma            float64 `json:"output_mean_luma"`
	OutputMeanLabL            float64 `json:"output_mean_lab_l"`
	OutputP95Luma             float64 `json:"output_p95_luma"`
	OutputClipPct             float64 `json:"output_clip_pct"`
	HighlightMaskPct          float64 `json:"highlight_mask_pct"`
	GlobalTextureRetention    float64 `json:"global_texture_retention"`
	HighlightTextureRetention float64 `json:"highlight_texture_retention"`
	SevereInteriorRetention   float64 `json:"severe_interior_texture_retention"`
	ClippedTextureRetention   float64 `json:"clipped_texture_retention"`
}

func CompareFrame(src2020, out709 *FloatImage, frame int, variant string) FrameMetrics {
	w, h := src2020.Width, src2020.Height
	n := w * h
	sourceLuma := make([]float64, n)
	outputLuma := make([]float64, n)
	workMax := make([]float64, n)

	m := FrameMetrics{Frame: frame, Variant: variant, Pixels: n}
	hueDeltas := make([]float64, 0, n/8)
	var hue5, hue10 int
	var desaturatedCandidates, desaturated int
	var deltaE sumFloat
	var outLumaSum, outLabLSum float64
	outLumas := make([]float64, 0, n)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			src := src2020.Pixel(x, y)
			raw709 := colorconv.ConvertRec2020To709(src)
			work := GamutCompress(raw709)
			out := out709.Pixel(x, y)
			sourceLuma[idx] = colorconv.Luminance709(work)
			outputLuma[idx] = colorconv.Luminance709(out)
			workMax[idx] = math.Max(work.R, math.Max(work.G, work.B))

			srcLab := colorconv.Rec2020ToLab(src)
			outLab := colorconv.RGBToLab(out)
			deltaE.add(colorconv.DeltaE00(srcLab, outLab))
			outLumaSum += outputLuma[idx]
			outLabLSum += outLab.L
			outLumas = append(outLumas, outputLuma[idx])
			if math.Max(out.R, math.Max(out.G, out.B)) >= 0.999 {
				m.OutputClipPct++
			}

			srcChroma := colorconv.Chroma(srcLab)
			outChroma := colorconv.Chroma(outLab)
			if srcChroma >= 18 {
				desaturatedCandidates++
				if outChroma < 8 {
					desaturated++
				}
			}
			if srcChroma >= 10 && outChroma >= 10 {
				d := math.Abs(colorconv.HueDelta(colorconv.HueDegrees(srcLab), colorconv.HueDegrees(outLab)))
				hueDeltas = append(hueDeltas, d)
				if d > 5 {
					hue5++
				}
				if d > 10 {
					hue10++
				}
			}
		}
	}

	m.HueEligiblePixels = len(hueDeltas)
	if len(hueDeltas) > 0 {
		sort.Float64s(hueDeltas)
		m.HueMeanAbsDegrees = average(hueDeltas)
		m.HueP95AbsDegrees = percentileSorted(hueDeltas, .95)
		m.HueMaxAbsDegrees = hueDeltas[len(hueDeltas)-1]
		m.HueOver5Pct = 100 * float64(hue5) / float64(len(hueDeltas))
		m.HueOver10Pct = 100 * float64(hue10) / float64(len(hueDeltas))
	}
	if desaturatedCandidates > 0 {
		m.DesaturatedPct = 100 * float64(desaturated) / float64(desaturatedCandidates)
	}
	m.MeanDeltaE76 = deltaE.mean(float64(n))
	m.SourceMeanLuma = average(sourceLuma)
	m.OutputMeanLuma = outLumaSum / float64(n)
	m.OutputMeanLabL = outLabLSum / float64(n)
	sort.Float64s(outLumas)
	m.OutputP95Luma = percentileSorted(outLumas, .95)
	m.OutputClipPct = 100 * m.OutputClipPct / float64(n)

	var srcGlobal, outGlobal, srcHi, outHi, srcClip, outClip float64
	var hiCount, clipCount int
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			idx := y*w + x
			sg := sobel(sourceLuma, w, idx)
			og := sobel(outputLuma, w, idx)
			srcGlobal += sg
			outGlobal += og
			if workMax[idx] > .75 {
				srcHi += sg
				outHi += og
				hiCount++
			}
			if math.Max(out709.Pix[idx*3], math.Max(out709.Pix[idx*3+1], out709.Pix[idx*3+2])) >= .995 {
				srcClip += sg
				outClip += og
				clipCount++
			}
		}
	}
	m.HighlightMaskPct = 100 * float64(hiCount) / float64((w-2)*(h-2))
	m.GlobalTextureRetention = ratio(outGlobal, srcGlobal)
	if hiCount > 0 {
		m.HighlightTextureRetention = ratio(outHi, srcHi)
	}

	var srcInterior, outInterior float64
	var interiorCount int
	for y := 2; y < h-2; y++ {
		for x := 2; x < w-2; x++ {
			idx := y*w + x
			if !severeInterior(workMax, w, idx) {
				continue
			}
			srcInterior += sobel(sourceLuma, w, idx)
			outInterior += sobel(outputLuma, w, idx)
			interiorCount++
		}
	}
	if interiorCount > 32 {
		m.SevereInteriorRetention = ratio(outInterior, srcInterior)
	}
	if clipCount > 20 {
		m.ClippedTextureRetention = ratio(outClip, srcClip)
	}
	return m
}

func severeInterior(maxRGB []float64, width, idx int) bool {
	if maxRGB[idx] <= 1.15 {
		return false
	}
	offsets := []int{-width - 1, -width, -width + 1, -1, 1, width - 1, width, width + 1}
	for _, off := range offsets {
		if maxRGB[idx+off] <= 1.05 {
			return false
		}
	}
	return true
}

type sumFloat struct{ sum float64 }

func (s *sumFloat) add(v float64) { s.sum += v }
func (s sumFloat) mean(n float64) float64 {
	if n == 0 {
		return 0
	}
	return s.sum / n
}

func average(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func ratio(a, b float64) float64 {
	if b < 1e-9 {
		return 1
	}
	return a / b
}

func sobel(luma []float64, width, idx int) float64 {
	gx := -luma[idx-width-1] + luma[idx-width+1] - 2*luma[idx-1] + 2*luma[idx+1] - luma[idx+width-1] + luma[idx+width+1]
	gy := -luma[idx-width-1] - 2*luma[idx-width] - luma[idx-width+1] + luma[idx+width-1] + 2*luma[idx+width] + luma[idx+width+1]
	return math.Hypot(gx, gy)
}
