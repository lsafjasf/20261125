package colorcal_test

import (
	"testing"

	"colorcal"
)

func TestRawBitDepthChartIsNormalized(t *testing.T) {
	chart, truth := colorcal.NewSyntheticScenario(11)
	raw := chart
	raw.BitDepth = 10
	raw.SaturationLevel = 1023
	raw.Patches = append([]colorcal.Patch(nil), chart.Patches...)
	for i := range raw.Patches {
		for c := 0; c < 3; c++ {
			raw.Patches[i].MeasuredRGB[c] *= 1023
			raw.Patches[i].SigmaRGB[c] *= 1023
		}
	}

	report, err := colorcal.Fit(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := closeRel("exposure", report.Params.Exposure, truth.Exposure, 0.06); err != nil {
		t.Fatal(err)
	}
	if report.JointFit.SkinDeltaE > 2.2 {
		t.Fatalf("raw chart skin dE=%.3f", report.JointFit.SkinDeltaE)
	}
}

func TestKnownBlackLevelIsSubtracted(t *testing.T) {
	chart, truth := colorcal.NewSyntheticScenario(21)
	black := 0.025
	chart.BlackLevel = black
	for i := range chart.Patches {
		for c := 0; c < 3; c++ {
			shifted := chart.Patches[i].MeasuredRGB[c] + black
			if shifted >= 1 {
				shifted = 1
				chart.Patches[i].ClipRGB[c] = true
			}
			chart.Patches[i].MeasuredRGB[c] = shifted
		}
	}

	report, err := colorcal.Fit(chart)
	if err != nil {
		t.Fatal(err)
	}
	if err := closeRel("gamma_g", report.Params.Gamma[1], truth.Gamma[1], 0.07); err != nil {
		t.Fatal(err)
	}
	if report.JointFit.SkinDeltaE > 2.2 {
		t.Fatalf("black-level skin dE=%.3f", report.JointFit.SkinDeltaE)
	}
}
