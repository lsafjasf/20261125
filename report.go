package colorcal

import (
	"math"
	"sort"
	"strings"
)

// CorrectColor applies the inverse response and effective gain to one measured
// color. Clipped channels remain mathematically invertible but are flagged by
// Fit as untrustworthy.
func CorrectColor(measured RGB, params Params) RGB {
	return correctRGB(measured, params)
}

func buildPatchReports(chart Chart, obs []observation, index [3][]int, params Params, opts Options) []PatchReport {
	reports := make([]PatchReport, 0, len(chart.Patches))

	for i, patch := range chart.Patches {
		report := PatchReport{
			ID:           patch.ID,
			Name:         patch.Name,
			Group:        patch.Group,
			Status:       "reliable",
			Weight:       1,
			CorrectedRGB: correctRGB(patch.MeasuredRGB, params),
		}

		severe := false
		warning := false
		minWeight := 1.0
		applicableChannels := 0
		lowSignalChannels := 0

		for c := 0; c < 3; c++ {
			cd := ChannelDiagnostic{Channel: c, RobustWeight: 1}

			switch {
			case patch.MaskRGB[c]:
				cd.Used = false
				cd.Reason = "user_masked"
				report.Reasons = appendUnique(report.Reasons, channelName(c)+":user_masked")
				warning = true
				minWeight = 0
			case patch.ReferenceRGB[c] <= 1e-8:
				cd.Used = false
				cd.Reason = "reference_channel_zero"
				report.Reasons = appendUnique(report.Reasons, channelName(c)+":reference_channel_zero")
				warning = true
			default:
				obsIndex := index[c][i]
				if obsIndex < 0 {
					clipped := patch.ClipRGB[c] || patch.MeasuredRGB[c] >= opts.ClipThreshold
					cd.Measured = patch.MeasuredRGB[c]
					cd.Clipped = clipped
					cd.Used = false
					cd.RobustWeight = 0
					minWeight = 0
					if clipped {
						cd.Reason = "highlight_clipped"
						report.Reasons = appendUnique(report.Reasons, channelName(c)+":highlight_clipped")
						severe = true
					} else {
						cd.Reason = "excluded"
						warning = true
					}
					break
				}

				ob := obs[obsIndex]
				applicableChannels++
				cd.Used = ob.weight > 1e-6
				cd.RobustWeight = ob.weight
				cd.StandardResidual = ob.zScore
				cd.Predicted = ob.predicted
				cd.Measured = ob.measured

				lowSignal := ob.predicted < opts.ShadowSNR*math.Sqrt(ob.variance)
				cd.LowSignal = lowSignal
				if lowSignal {
					lowSignalChannels++
				}
				minWeight = math.Min(minWeight, ob.weight)

				switch {
				case ob.weight < opts.MinChannelWeight:
					cd.Reason = "local_reflection_or_outlier"
					report.Reasons = appendUnique(report.Reasons, channelName(c)+":local_reflection_or_outlier")
					severe = true
				case lowSignal:
					cd.Reason = "shadow_low_snr"
					report.Reasons = appendUnique(report.Reasons, channelName(c)+":shadow_low_snr")
					warning = true
				case ob.weight < 0.75 || math.Abs(ob.zScore) > 2.5:
					cd.Reason = "moderate_residual"
					report.Reasons = appendUnique(report.Reasons, channelName(c)+":moderate_residual")
					warning = true
				}
				if lowSignal {
					warning = true
				}
			}

			report.Channels = append(report.Channels, cd)
		}

		report.Weight = minWeight
		if applicableChannels == 3 && lowSignalChannels >= 2 {
			severe = true
			report.Reasons = appendUnique(report.Reasons, "multiple_channels_shadow_low_snr")
		}
		switch {
		case severe:
			report.Status = "unreliable"
		case warning:
			report.Status = "downweighted"
		default:
			report.Status = "reliable"
		}
		report.ReferenceDeltaE = DeltaE2000(report.CorrectedRGB, patch.ReferenceRGB)
		reports = append(reports, report)
	}
	return reports
}

func appendUnique(items []string, item string) []string {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func isSkinPatch(p Patch) bool {
	text := strings.ToLower(p.Group + " " + p.Name + " " + p.ID)
	return strings.Contains(text, "skin") || strings.Contains(text, "肤色")
}

func evaluateMeanAlign(chart Chart) *ColorAccuracy {
	var sumRef, sumMeas RGB
	for _, p := range chart.Patches {
		for c := 0; c < 3; c++ {
			sumRef[c] += p.ReferenceRGB[c]
			sumMeas[c] += math.Max(p.MeasuredRGB[c], 0)
		}
	}
	var gain RGB
	for c := 0; c < 3; c++ {
		if sumMeas[c] > 1e-12 {
			gain[c] = sumRef[c] / sumMeas[c]
		} else {
			gain[c] = 1
		}
	}

	corrected := make([]RGB, len(chart.Patches))
	for i, p := range chart.Patches {
		for c := 0; c < 3; c++ {
			corrected[i][c] = p.MeasuredRGB[c] * gain[c]
		}
	}
	return summarizeAccuracy(chart, corrected, "all-patch channel-mean ratio")
}

func evaluateJointFit(chart Chart, params Params, reports []PatchReport) *ColorAccuracy {
	corrected := make([]RGB, len(reports))
	for i, report := range reports {
		corrected[i] = report.CorrectedRGB
	}
	return summarizeAccuracy(chart, corrected, "joint exposure/WB/response robust fit")
}

func summarizeAccuracy(chart Chart, corrected []RGB, method string) *ColorAccuracy {
	result := &ColorAccuracy{Method: method}
	var allDE, skinDE []float64

	for i, patch := range chart.Patches {
		if i >= len(corrected) {
			break
		}
		de := DeltaE2000(corrected[i], patch.ReferenceRGB)
		allDE = append(allDE, de)
		result.PatchIDs = append(result.PatchIDs, patch.ID)
		if isSkinPatch(patch) {
			skinDE = append(skinDE, de)
		}
	}

	result.MeanDeltaE = meanFloat(allDE)
	result.P95DeltaE = percentileFloat(allDE, 0.95)
	result.MaxDeltaE = maxFloat(allDE)
	result.SkinDeltaE = meanFloat(skinDE)
	return result
}

func meanFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func percentileFloat(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(p*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func maxFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		m = math.Max(m, v)
	}
	return m
}
