package colorcal_test

import (
	"testing"

	"colorcal"
)

func TestFitWithoutPerChannelSigmas(t *testing.T) {
	chart, truth := colorcal.NewSyntheticScenario(7)
	for i := range chart.Patches {
		chart.Patches[i].SigmaRGB = colorcal.RGB{}
	}

	report, err := colorcal.Fit(chart)
	if err != nil {
		t.Fatal(err)
	}
	for c := 0; c < 3; c++ {
		if err := closeRel("gain", report.Params.EffectiveGain[c], truth.EffectiveGain[c], 0.08); err != nil {
			t.Error(err)
		}
		if err := closeRel("gamma", report.Params.Gamma[c], truth.Gamma[c], 0.08); err != nil {
			t.Error(err)
		}
	}
	if report.JointFit.SkinDeltaE > 2.5 {
		t.Fatalf("skin dE=%.3f", report.JointFit.SkinDeltaE)
	}
	for _, id := range truth.ReflectionPatches {
		patch := patchByID(t, report, id)
		if patch.Status != "unreliable" {
			t.Fatalf("reflection %s status=%s", id, patch.Status)
		}
	}
}
