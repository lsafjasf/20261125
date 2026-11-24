package colorcal_test

import (
	"testing"

	"colorcal"
)

func TestMultipleSeedsRecoverParametersAndFlags(t *testing.T) {
	for seed := int64(1); seed <= 40; seed++ {
		chart, truth := colorcal.NewSyntheticScenario(seed)
		report, err := colorcal.Fit(chart)
		if err != nil {
			t.Fatal(seed, err)
		}
		maxParam := 0.0
		maxParam = maxRel(maxParam, report.Params.Exposure, truth.Exposure)
		for c := 0; c < 3; c++ {
			maxParam = maxRel(maxParam, report.Params.WhiteBalance[c], truth.WhiteBalance[c])
			maxParam = maxRel(maxParam, report.Params.Gamma[c], truth.Gamma[c])
			maxParam = maxRel(maxParam, report.Params.EffectiveGain[c], truth.EffectiveGain[c])
		}
		reflectionOK := true
		for _, id := range truth.ReflectionPatches {
			p := findPatch(report, id)
			if p.Status != "unreliable" {
				reflectionOK = false
			}
		}
		highlightOK := true
		for _, id := range truth.HighlightPatches {
			p := findPatch(report, id)
			if p.Status != "unreliable" {
				highlightOK = false
			}
		}
		t.Logf("seed=%2d maxParamErr=%.3f skin=%.2f/%.2f refl=%v high=%v cond=%.3g",
			seed, maxParam, report.JointFit.SkinDeltaE, report.NaiveMeanAlign.SkinDeltaE,
			reflectionOK, highlightOK, report.Diagnostics.MatrixCondition)
		if maxParam > 0.08 || report.JointFit.SkinDeltaE > 2.5 || !reflectionOK || !highlightOK {
			t.Errorf("seed %d weak result: param=%.3f skin=%.2f refl=%v high=%v",
				seed, maxParam, report.JointFit.SkinDeltaE, reflectionOK, highlightOK)
		}
	}
}

func maxRel(current, got, want float64) float64 {
	if d := relError(got, want); d > current {
		return d
	}
	return current
}

func findPatch(r *colorcal.Report, id string) colorcal.PatchReport {
	for _, p := range r.Patches {
		if p.ID == id {
			return p
		}
	}
	return colorcal.PatchReport{}
}
