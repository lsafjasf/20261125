package colorcal

import (
	"fmt"
	"math"
	"sort"
)

type observation struct {
	patch    int
	channel  int
	ref      float64
	measured float64
	variance float64
	knownVar bool

	clipped    bool
	userMasked bool

	predicted float64
	zScore    float64
	weight    float64
}

// Options controls robust fitting and reliability thresholds.
type Options struct {
	ClipThreshold     float64
	ShadowSNR         float64
	OutlierZ          float64
	MinChannelWeight  float64
	MaxLMIterations   int
	RobustOuterRounds int
}

// DefaultOptions returns conservative normalized-[0,1] thresholds.
func DefaultOptions() Options {
	return Options{
		ClipThreshold:     0.985,
		ShadowSNR:         8.0,
		OutlierZ:          4.0,
		MinChannelWeight:  0.20,
		MaxLMIterations:   40,
		RobustOuterRounds: 6,
	}
}

// Fit estimates exposure, white balance and per-channel response from chart
// measurements only. Generator truth is intentionally not an argument.
func Fit(chart Chart) (*Report, error) {
	return FitWithOptions(chart, DefaultOptions())
}

// FitWithOptions is Fit with caller-controlled thresholds.
func FitWithOptions(chart Chart, opts Options) (*Report, error) {
	if len(chart.Patches) == 0 {
		return nil, fmt.Errorf("colorcal: chart has no patches")
	}
	chart = NormalizeChart(chart)
	chart = subtractKnownBlackLevel(chart)
	if opts.ClipThreshold <= 0 || opts.ClipThreshold > 1 {
		saturation := chart.SaturationLevel
		if saturation <= 0 || saturation > 1 {
			saturation = 1
		}
		opts.ClipThreshold = saturation - math.Max(0.012, 0.015*saturation)
	}
	if opts.ShadowSNR <= 0 {
		opts.ShadowSNR = 8
	}
	if opts.OutlierZ <= 0 {
		opts.OutlierZ = 4
	}
	if opts.MinChannelWeight <= 0 {
		opts.MinChannelWeight = 0.2
	}
	if opts.MaxLMIterations <= 0 {
		opts.MaxLMIterations = 40
	}
	if opts.RobustOuterRounds <= 0 {
		opts.RobustOuterRounds = 6
	}

	obs, index, err := buildObservations(chart, opts)
	if err != nil {
		return nil, err
	}

	theta := initialTheta(chart.Patches, obs)
	estimateUnknownScales(theta, obs)

	iterations := 0
	converged := false
	for round := 0; round < opts.RobustOuterRounds; round++ {
		kind := "tukey"
		if round < 2 {
			kind = "huber"
		}
		updateResidualsAndWeights(theta, obs, kind)
		theta, converged, iterations = lmMinimize(theta, obs, opts.MaxLMIterations)
		estimateUnknownScales(theta, obs)
	}
	updateResidualsAndWeights(theta, obs, "tukey")

	params := thetaToParams(theta)
	diag := buildDiagnostics(theta, obs, iterations, converged)
	patchReports := buildPatchReports(chart, obs, index, params, opts)

	report := &Report{
		Params:      params,
		Patches:     patchReports,
		Diagnostics: diag,
	}
	report.NaiveMeanAlign = evaluateMeanAlign(chart)
	report.JointFit = evaluateJointFit(chart, params, patchReports)
	return report, nil
}

func subtractKnownBlackLevel(chart Chart) Chart {
	black := chart.BlackLevel
	if black <= 0 || black >= 1 {
		return chart
	}
	clipAt := 0.985
	if chart.SaturationLevel > 0 && chart.SaturationLevel <= 1 {
		clipAt = chart.SaturationLevel - math.Max(0.012, 0.015*chart.SaturationLevel)
	}

	out := chart
	out.Patches = append([]Patch(nil), chart.Patches...)
	for i := range out.Patches {
		for c := 0; c < 3; c++ {
			if out.Patches[i].MeasuredRGB[c] >= clipAt {
				out.Patches[i].ClipRGB[c] = true
			}
			out.Patches[i].MeasuredRGB[c] -= black
		}
	}
	out.BlackLevel = 0
	return out
}

func buildObservations(chart Chart, opts Options) ([]observation, [3][]int, error) {
	n := len(chart.Patches)
	var index [3][]int
	for c := range index {
		index[c] = make([]int, n)
		for i := range index[c] {
			index[c][i] = -1
		}
	}

	var obs []observation
	for i, patch := range chart.Patches {
		for c := 0; c < 3; c++ {
			if patch.MaskRGB[c] || patch.ReferenceRGB[c] <= 1e-8 {
				continue
			}
			clipped := patch.ClipRGB[c] || patch.MeasuredRGB[c] >= opts.ClipThreshold
			if clipped {
				continue
			}

			variance := 1e-4
			known := false
			if patch.SigmaRGB[c] > 0 {
				variance = patch.SigmaRGB[c] * patch.SigmaRGB[c]
				variance = math.Max(variance, 1e-12)
				known = true
			}
			idx := len(obs)
			index[c][i] = idx
			obs = append(obs, observation{
				patch:    i,
				channel:  c,
				ref:      patch.ReferenceRGB[c],
				measured: patch.MeasuredRGB[c],
				variance: variance,
				knownVar: known,
				weight:   1,
			})
		}
	}

	channelCount := [3]int{}
	for _, ob := range obs {
		channelCount[ob.channel]++
	}
	for c, count := range channelCount {
		if count < 4 {
			return nil, index, fmt.Errorf("colorcal: channel %s has only %d usable observations; need at least 4", channelName(c), count)
		}
	}
	return obs, index, nil
}

func initialTheta(patches []Patch, obs []observation) []float64 {
	theta := make([]float64, thetaParameters)
	for c := 0; c < 3; c++ {
		type point struct{ x, y, weight float64 }
		var points []point
		var ratios []float64

		for _, ob := range obs {
			if ob.channel != c {
				continue
			}
			ratio := ob.measured / ob.ref
			if ratio > 1e-4 && ratio < 100 {
				ratios = append(ratios, ratio)
			}
			if ob.ref >= 0.025 && ob.measured >= 0.035 {
				if ob.knownVar && ob.measured < 3*math.Sqrt(ob.variance) {
					continue
				}
				points = append(points, point{
					x:      math.Log(ob.ref),
					y:      math.Log(math.Max(ob.measured, 1e-5)),
					weight: 1,
				})
			}
		}

		gamma := 1.0
		logGain := 0.0
		if len(points) >= 4 {
			sumW, sumX, sumY, sumXX, sumXY := 0.0, 0.0, 0.0, 0.0, 0.0
			for _, p := range points {
				sumW += p.weight
				sumX += p.weight * p.x
				sumY += p.weight * p.y
				sumXX += p.weight * p.x * p.x
				sumXY += p.weight * p.x * p.y
			}
			xBar := sumX / sumW
			yBar := sumY / sumW
			den := sumXX - sumX*sumX/sumW
			num := sumXY - sumX*sumY/sumW
			if math.Abs(den) > 1e-8 {
				gamma = num / den
			}
			gamma = math.Max(0.45, math.Min(2.2, gamma))
			intercept := yBar - gamma*xBar
			logGain = intercept / gamma
		} else if len(ratios) > 0 {
			sort.Float64s(ratios)
			median := ratios[len(ratios)/2]
			logGain = math.Log(math.Max(median, 1e-4))
		}

		logGain = math.Max(math.Log(0.05), math.Min(math.Log(20), logGain))
		theta[c] = logGain
		theta[c+3] = math.Log(gamma)
	}
	return theta
}

func estimateUnknownScales(theta []float64, obs []observation) {
	residuals := make([][]float64, 3)
	for i := range obs {
		ob := &obs[i]
		ob.predicted = predictTheta(ob.ref, theta, ob.channel)
		if !ob.knownVar {
			residuals[ob.channel] = append(residuals[ob.channel], math.Abs(ob.measured-ob.predicted))
		}
	}
	for c := 0; c < 3; c++ {
		if len(residuals[c]) < 3 {
			continue
		}
		sort.Float64s(residuals[c])
		median := residuals[c][len(residuals[c])/2]
		sigma := 1.4826 * median
		sigma = math.Max(sigma, 0.004)
		variance := sigma * sigma
		for i := range obs {
			if obs[i].channel == c && !obs[i].knownVar {
				obs[i].variance = variance
			}
		}
	}
}

func updateResidualsAndWeights(theta []float64, obs []observation, kind string) {
	for i := range obs {
		ob := &obs[i]
		ob.predicted = predictTheta(ob.ref, theta, ob.channel)
		residual := ob.measured - ob.predicted
		sigma := math.Sqrt(math.Max(ob.variance, 1e-12))
		ob.zScore = residual / sigma
		absZ := math.Abs(ob.zScore)

		switch kind {
		case "huber":
			const k = 2.5
			if absZ <= k {
				ob.weight = 1
			} else {
				ob.weight = k / absZ
			}
		default:
			const tuning = 4.685
			u := absZ / tuning
			if u >= 1 {
				ob.weight = 0
			} else {
				v := 1 - u*u
				ob.weight = v * v
			}
		}
	}
}
