package sdrquant

import (
	"fmt"
	"math"
	"sort"
)

// PixelClass is the per-pixel interpretation shown in classification.png.
type PixelClass uint8

const (
	ClassSmooth PixelClass = iota
	ClassSourceTexture
	ClassSourceEdge
	ClassQuantContour
	ClassDitherNoise
	ClassClipped
)

func (c PixelClass) String() string {
	switch c {
	case ClassSmooth:
		return "smooth"
	case ClassSourceTexture:
		return "source-texture"
	case ClassSourceEdge:
		return "source-edge"
	case ClassQuantContour:
		return "quantization-contour"
	case ClassDitherNoise:
		return "dither-shaping-noise"
	case ClassClipped:
		return "clipped"
	default:
		return "unknown"
	}
}

// ModeMetrics records the exact quantizer configuration used.
type ModeMetrics struct {
	Dither         DitherMode   `json:"dither"`
	DitherStrength float64      `json:"dither_strength_lsb"`
	Shaping        ShapingMode  `json:"shaping"`
	Serpentine     bool         `json:"serpentine"`
	Seed           int64        `json:"seed"`
	Transfer       TransferMode `json:"transfer"`
}

// SourceMetrics describes the high-bit-depth input before integer rounding.
type SourceMetrics struct {
	Min           float64 `json:"min"`
	Max           float64 `json:"max"`
	Mean          float64 `json:"mean"`
	ClippedPixels int     `json:"clipped_pixels"`
}

// QuantMetrics describes the 8-bit SDR result.
type QuantMetrics struct {
	Min          uint8   `json:"min"`
	Max          uint8   `json:"max"`
	Mean         float64 `json:"mean"`
	UniqueLevels int     `json:"unique_levels"`
}

// ResidualMetrics measures input minus output in output LSB units.
type ResidualMetrics struct {
	MeanAbsLSB float64 `json:"mean_abs_lsb"`
	RMSLSB     float64 `json:"rms_lsb"`
	MaxAbsLSB  float64 `json:"max_abs_lsb"`
}

// FeedbackMetrics measures error received from previous pixels. It is zero
// when shaping is disabled.
type FeedbackMetrics struct {
	MeanAbsLSB float64 `json:"mean_abs_lsb"`
	RMSLSB     float64 `json:"rms_lsb"`
	MaxAbsLSB  float64 `json:"max_abs_lsb"`
}

// ClassificationMetrics counts the mutually exclusive pixels in the map.
type ClassificationMetrics struct {
	SmoothPixels        int `json:"smooth_pixels"`
	SourceTexturePixels int `json:"source_texture_pixels"`
	SourceEdgePixels    int `json:"source_edge_pixels"`
	QuantContourPixels  int `json:"quant_contour_pixels"`
	DitherNoisePixels   int `json:"dither_shaping_noise_pixels"`
	ClippedPixels       int `json:"clipped_pixels"`
}

// DistinctionMetrics separates bands that only exist after quantization from
// output changes introduced by dithering/noise shaping.
type DistinctionMetrics struct {
	SourceEdges                   int     `json:"source_edges"`
	BaselineSyntheticEdges        int     `json:"baseline_synthetic_edges"`
	ActualQuantEdges              int     `json:"actual_quant_edges"`
	DitherAlteredPixels           int     `json:"dither_altered_pixels"`
	DitherAlteredFraction         float64 `json:"dither_altered_fraction"`
	SourceEdgeFraction            float64 `json:"source_edge_fraction"`
	BaselineSyntheticEdgeFraction float64 `json:"baseline_synthetic_edge_fraction"`
}

// SpectrumMetrics describe the spatial spectrum of the low-bit residual.
type SpectrumMetrics struct {
	Size               int     `json:"size"`
	LowFrequencyRatio  float64 `json:"low_frequency_ratio"`
	MidFrequencyRatio  float64 `json:"mid_frequency_ratio"`
	HighFrequencyRatio float64 `json:"high_frequency_ratio"`
	Centroid           float64 `json:"centroid_cycles_per_pixel"`
	PeakRadius         float64 `json:"peak_radius_cycles_per_pixel"`
	PeakPowerDB        float64 `json:"peak_power_db"`
}

// Metrics is written to diagnostics/metrics.json.
type Metrics struct {
	Width          int                   `json:"width"`
	Height         int                   `json:"height"`
	Channels       int                   `json:"channels"`
	Bits           int                   `json:"bits"`
	QuantStep      float64               `json:"quant_step"`
	Mode           ModeMetrics           `json:"mode"`
	Source         SourceMetrics         `json:"source"`
	Quantization   QuantMetrics          `json:"quantization"`
	Residual       ResidualMetrics       `json:"residual"`
	DitherSignal   ResidualMetrics       `json:"dither_signal"`
	Feedback       FeedbackMetrics       `json:"feedback"`
	Classification ClassificationMetrics `json:"classification"`
	Distinction    DistinctionMetrics    `json:"distinction"`
	Spectrum       SpectrumMetrics       `json:"spectrum"`
}

// DiagnosticOptions controls the FFT workspace. SpectrumSize is rounded up to
// a power of two and bounded to [64,2048].
type DiagnosticOptions struct {
	SpectrumSize int
}

// Diagnostics holds renderable maps plus their exact classification labels.
// SpectrumDB is the shifted log-power grid, in row-major order.
type Diagnostics struct {
	Metrics           Metrics
	ResidualMap       *FloatImage
	DiffusionMap      *FloatImage
	SpectrumMap       *FloatImage
	ClassificationMap *FloatImage
	Classes           []PixelClass
	SpectrumDB        []float64
	SpectrumSize      int
}

// Analyze computes all requested diagnostic representations without applying
// any blur to the SDR result. Clamped display values are used for maps; raw
// clipping is reported separately.
func Analyze(res *Result, opts DiagnosticOptions) (*Diagnostics, error) {
	if res == nil || res.Encoded == nil {
		return nil, fmt.Errorf("nil quantized result")
	}
	if res.Width <= 0 || res.Height <= 0 || (res.Channels != 1 && res.Channels != 3) {
		return nil, fmt.Errorf("invalid result dimensions")
	}
	expected64 := int64(res.Width) * int64(res.Height) * int64(res.Channels)
	if expected64 <= 0 || expected64 > int64(^uint(0)>>1) {
		return nil, fmt.Errorf("result dimensions overflow")
	}
	expected := int(expected64)
	if len(res.Quant) != expected || len(res.Residual) != expected ||
		len(res.Feedback) != expected || len(res.Encoded.Pix) != expected {
		return nil, fmt.Errorf("result buffers do not match dimensions")
	}
	n := res.Width * res.Height
	if n == 0 {
		return nil, fmt.Errorf("empty result")
	}
	maxLevel := float64((uint64(1) << res.Bits) - 1)
	step := 1 / maxLevel
	srcLuma, _, residualLuma, feedbackLuma := lumaPlanes(res, maxLevel)
	baseline := baselineLevels(res, maxLevel)

	classes := make([]PixelClass, n)
	classCounts := make(map[PixelClass]int)
	sourceEdges, syntheticEdges, actualEdges, alteredPixels := 0, 0, 0, 0
	quantSamples := []uint8{}
	levelSet := make(map[uint8]struct{})
	sourceMin, sourceMax, sourceSum := math.Inf(1), math.Inf(-1), 0.0

	for _, v := range res.Encoded.Pix {
		fv := float64(v)
		sourceMin = math.Min(sourceMin, fv)
		sourceMax = math.Max(sourceMax, fv)
		sourceSum += fv
	}
	for y := 0; y < res.Height; y++ {
		for x := 0; x < res.Width; x++ {
			i := y*res.Width + x
			base := i * res.Channels
			clipped := false
			altered := false
			for c := 0; c < res.Channels; c++ {
				if res.Encoded.Pix[base+c] < 0 || res.Encoded.Pix[base+c] > 1 ||
					math.IsNaN(float64(res.Encoded.Pix[base+c])) {
					clipped = true
				}
				if res.Quant[base+c] != baseline[base+c] {
					altered = true
				}
				quantSamples = append(quantSamples, res.Quant[base+c])
				levelSet[res.Quant[base+c]] = struct{}{}
			}
			if clipped {
				classes[i] = ClassClipped
				classCounts[ClassClipped]++
				continue
			}
			srcAdj := maxAdjacentDiff(srcLuma, res.Width, res.Height, x, y)
			srcRange := localRange(srcLuma, res.Width, res.Height, x, y, 2)
			if srcAdj >= 0.5*step {
				sourceEdges++
			}
			if baselineHasEdge(baseline, res.Width, res.Height, res.Channels, x, y) {
				if srcAdj < 0.5*step {
					syntheticEdges++
				}
			}
			if actualHasEdge(res, x, y) {
				actualEdges++
			}
			if altered {
				alteredPixels++
			}

			switch {
			case srcAdj >= 0.5*step:
				classes[i] = ClassSourceEdge
			case altered:
				classes[i] = ClassDitherNoise
			case baselineHasEdge(baseline, res.Width, res.Height, res.Channels, x, y):
				classes[i] = ClassQuantContour
			case srcRange >= 0.5*step:
				classes[i] = ClassSourceTexture
			default:
				classes[i] = ClassSmooth
			}
			classCounts[classes[i]]++
		}
	}

	residualStats := stats(res.Residual)
	feedbackStats := stats(res.Feedback)
	ditherStats := stats(res.Dither)
	spectrumMap, spectrumDB, spectrumMetrics, err := spectrumDiagnostics(
		residualLuma, res.Width, res.Height, opts.SpectrumSize)
	if err != nil {
		return nil, err
	}

	quantMin, quantMax := uint8(255), uint8(0)
	quantSum := 0.0
	for _, q := range quantSamples {
		if q < quantMin {
			quantMin = q
		}
		if q > quantMax {
			quantMax = q
		}
		quantSum += float64(q)
	}
	if quantMax == 0 && quantMin == 255 {
		quantMin = 0
	}
	metrics := Metrics{
		Width: res.Width, Height: res.Height, Channels: res.Channels,
		Bits: res.Bits, QuantStep: step,
		Mode: ModeMetrics{
			Dither: res.DitherMode, DitherStrength: res.DitherStrength,
			Shaping: res.ShapingMode, Serpentine: res.Serpentine,
			Seed: res.Seed, Transfer: res.TransferMode,
		},
		Source: SourceMetrics{
			Min: sourceMin, Max: sourceMax,
			Mean:          sourceSum / float64(len(res.Encoded.Pix)),
			ClippedPixels: res.ClippedPixels,
		},
		Quantization: QuantMetrics{
			Min: quantMin, Max: quantMax,
			Mean:         quantSum / float64(len(quantSamples)),
			UniqueLevels: len(levelSet),
		},
		Residual:     residualStats,
		DitherSignal: ditherStats,
		Feedback: FeedbackMetrics{
			MeanAbsLSB: feedbackStats.MeanAbsLSB,
			RMSLSB:     feedbackStats.RMSLSB,
			MaxAbsLSB:  feedbackStats.MaxAbsLSB,
		},
		Classification: ClassificationMetrics{
			SmoothPixels:        classCounts[ClassSmooth],
			SourceTexturePixels: classCounts[ClassSourceTexture],
			SourceEdgePixels:    classCounts[ClassSourceEdge],
			QuantContourPixels:  classCounts[ClassQuantContour],
			DitherNoisePixels:   classCounts[ClassDitherNoise],
			ClippedPixels:       classCounts[ClassClipped],
		},
		Distinction: DistinctionMetrics{
			SourceEdges:                   sourceEdges,
			BaselineSyntheticEdges:        syntheticEdges,
			ActualQuantEdges:              actualEdges,
			DitherAlteredPixels:           alteredPixels,
			DitherAlteredFraction:         float64(alteredPixels) / float64(n),
			SourceEdgeFraction:            float64(sourceEdges) / float64(n),
			BaselineSyntheticEdgeFraction: float64(syntheticEdges) / float64(n),
		},
		Spectrum: spectrumMetrics,
	}
	return &Diagnostics{
		Metrics:           metrics,
		ResidualMap:       signedMap(residualLuma, res.Width, res.Height, 0.5),
		DiffusionMap:      signedMap(feedbackLuma, res.Width, res.Height, 0.25),
		SpectrumMap:       spectrumMap,
		ClassificationMap: classificationMap(classes, res.Width, res.Height),
		Classes:           classes,
		SpectrumDB:        spectrumDB,
		SpectrumSize:      spectrumMetrics.Size,
	}, nil
}

func stats(values []float32) ResidualMetrics {
	sum, sumSq, maxAbs := 0.0, 0.0, 0.0
	for _, value := range values {
		v := math.Abs(float64(value))
		sum += v
		sumSq += v * v
		if v > maxAbs {
			maxAbs = v
		}
	}
	if len(values) == 0 {
		return ResidualMetrics{}
	}
	return ResidualMetrics{
		MeanAbsLSB: sum / float64(len(values)),
		RMSLSB:     math.Sqrt(sumSq / float64(len(values))),
		MaxAbsLSB:  maxAbs,
	}
}

func lumaPlanes(res *Result, maxLevel float64) (src, out, residual, feedback []float64) {
	n := res.Width * res.Height
	src = make([]float64, n)
	out = make([]float64, n)
	residual = make([]float64, n)
	feedback = make([]float64, n)
	weights := []float64{1}
	if res.Channels == 3 {
		weights = []float64{0.2126, 0.7152, 0.0722}
	}
	for i := 0; i < n; i++ {
		base := i * res.Channels
		for c := 0; c < res.Channels; c++ {
			w := weights[c]
			src[i] += w * Clamp01(float64(res.Encoded.Pix[base+c]))
			out[i] += w * float64(res.Quant[base+c]) / maxLevel
			residual[i] += w * float64(res.Residual[base+c])
			feedback[i] += w * float64(res.Feedback[base+c])
		}
	}
	return src, out, residual, feedback
}

func baselineLevels(res *Result, maxLevel float64) []uint8 {
	out := make([]uint8, len(res.Encoded.Pix))
	for i, value := range res.Encoded.Pix {
		q := math.Floor(Clamp01(float64(value))*maxLevel + 0.5)
		out[i] = uint8(math.Max(0, math.Min(maxLevel, q)))
	}
	return out
}

func maxAdjacentDiff(values []float64, width, height, x, y int) float64 {
	center := values[y*width+x]
	maxDiff := 0.0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			nx, ny := x+dx, y+dy
			if nx < 0 || nx >= width || ny < 0 || ny >= height {
				continue
			}
			maxDiff = math.Max(maxDiff, math.Abs(center-values[ny*width+nx]))
		}
	}
	return maxDiff
}

func localRange(values []float64, width, height, x, y, radius int) float64 {
	minV, maxV := math.Inf(1), math.Inf(-1)
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			nx, ny := x+dx, y+dy
			if nx < 0 || nx >= width || ny < 0 || ny >= height {
				continue
			}
			v := values[ny*width+nx]
			minV = math.Min(minV, v)
			maxV = math.Max(maxV, v)
		}
	}
	return maxV - minV
}

func baselineHasEdge(levels []uint8, width, height, channels, x, y int) bool {
	base := (y*width + x) * channels
	for _, delta := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		nx, ny := x+delta[0], y+delta[1]
		if nx < 0 || nx >= width || ny < 0 || ny >= height {
			continue
		}
		other := (ny*width + nx) * channels
		for c := 0; c < channels; c++ {
			if levels[base+c] != levels[other+c] {
				return true
			}
		}
	}
	return false
}

func actualHasEdge(res *Result, x, y int) bool {
	base := (y*res.Width + x) * res.Channels
	for _, delta := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		nx, ny := x+delta[0], y+delta[1]
		if nx < 0 || nx >= res.Width || ny < 0 || ny >= res.Height {
			continue
		}
		other := (ny*res.Width + nx) * res.Channels
		for c := 0; c < res.Channels; c++ {
			if res.Quant[base+c] != res.Quant[other+c] {
				return true
			}
		}
	}
	return false
}

func percentileAbs(values []float64, fraction float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	for i, v := range values {
		sorted[i] = math.Abs(v)
	}
	sort.Float64s(sorted)
	idx := int(math.Round(fraction * float64(len(sorted)-1)))
	return sorted[idx]
}
