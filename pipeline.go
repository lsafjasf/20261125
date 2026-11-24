package nightwb

// Pipeline holds the outputs of a full analysis run.
type Pipeline struct {
	Frame        *Frame
	Estimate     EstimateResult
	Exposure     ExposureReport
	Conservative *AdaptedFrame
	Aggressive   *AdaptedFrame
	Masks        [2][]uint16 // source defects, adaptation defects (aggressive)
	Stats        RecoverabilityStats
	Report       Report
}

// Run executes estimation, both adaptation schemes, and recoverability
// analysis. Recoverability is evaluated against the aggressive pass because
// it is the least forgiving; conservative-specific gamut stats are also
// recorded in the report.
func Run(f *Frame) *Pipeline {
	est := EstimateIlluminant(f)
	ill := est.Illuminant
	exp := AnalyzeExposure(f)

	cons := ApplyAdaptation(f, ill, ConservativeScheme())
	agg := ApplyAdaptation(f, ill, AggressiveScheme())

	masks, stats := AnalyzeRecoverability(f, agg, est, ill)

	rep := Report{
		Illuminant:  ill,
		Exposure:    exp,
		Recoverable: stats,
		Schemes: []SchemeReport{
			schemeStats("conservative", cons, f, ConservativeScheme().Strength),
			schemeStats("aggressive", agg, f, AggressiveScheme().Strength),
		},
	}
	rep.UnrecoverableNote = buildNote(ill, exp, stats)

	return &Pipeline{
		Frame:        f,
		Estimate:     est,
		Exposure:     exp,
		Conservative: cons,
		Aggressive:   agg,
		Masks:        [2][]uint16{masks[0], masks[1]},
		Stats:        stats,
		Report:       rep,
	}
}

func schemeStats(name string, af *AdaptedFrame, f *Frame, strength float64) SchemeReport {
	var gamut int
	var neutralSum [3]float64
	var neutralN float64
	for i, px := range f.RGB {
		if af.OutOfGamut[i] {
			gamut++
		}
		cR, cB := chroma(px)
		if abs(cR) < 0.03 && abs(cB) < 0.03 {
			a := af.RGB[i]
			neutralSum[0] += a[0]
			neutralSum[1] += a[1]
			neutralSum[2] += a[2]
			neutralN++
		}
	}
	mean := [3]float64{}
	if neutralN > 0 {
		mean = [3]float64{
			neutralSum[0] / neutralN,
			neutralSum[1] / neutralN,
			neutralSum[2] / neutralN,
		}
	}
	return SchemeReport{
		Name:               name,
		Strength:           strength,
		OutOfGamutFrac:     float64(gamut) / float64(len(f.RGB)),
		MeanAdaptedNeutral: mean,
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func buildNote(ill Illuminant, exp ExposureReport, s RecoverabilityStats) string {
	note := "Regions flagged in the recoverability mask cannot have their true " +
		"color guaranteed: clipped channels carry no hue information, crushed " +
		"shadows carry no signal, and out-of-gamut adapted pixels are clamped."
	if ill.MixedLight {
		note += " Mixed lighting means a single global illuminant leaves the " +
			"secondary light source's cast uncorrected (spatial/local adaptation " +
			"would be required)."
	}
	if exp.ExposureState != "normal" {
		note += " Exposure anomaly (" + exp.ExposureState +
			") further limits recoverable dynamic range."
	}
	if ill.Confidence < 0.35 {
		note += " Low estimator confidence: correction direction is uncertain, " +
			"prefer the conservative scheme."
	}
	return note
}
