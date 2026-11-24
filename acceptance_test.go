package colorcal_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"colorcal"
)

func relError(got, want float64) float64 {
	return math.Abs(got-want) / math.Max(math.Abs(want), 1e-12)
}

func patchByID(t *testing.T, report *colorcal.Report, id string) colorcal.PatchReport {
	t.Helper()
	for _, patch := range report.Patches {
		if patch.ID == id {
			return patch
		}
	}
	t.Fatalf("patch %q not found", id)
	return colorcal.PatchReport{}
}

func hasReason(patch colorcal.PatchReport, fragment string) bool {
	for _, reason := range patch.Reasons {
		if strings.Contains(reason, fragment) {
			return true
		}
	}
	return false
}

func TestGeneratedParametersRecoveredWithoutTruthInput(t *testing.T) {
	chart, truth := colorcal.NewSyntheticScenario(42)
	report, err := colorcal.Fit(chart)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Diagnostics.Converged {
		t.Fatalf("fit did not converge: %+v", report.Diagnostics)
	}

	if err := closeRel("exposure", report.Params.Exposure, truth.Exposure, 0.05); err != nil {
		t.Error(err)
	}
	for c, channel := range []string{"r", "g", "b"} {
		if err := closeRel("wb_"+channel, report.Params.WhiteBalance[c], truth.WhiteBalance[c], 0.06); err != nil {
			t.Error(err)
		}
		if err := closeRel("gamma_"+channel, report.Params.Gamma[c], truth.Gamma[c], 0.05); err != nil {
			t.Error(err)
		}
		if err := closeRel("gain_"+channel, report.Params.EffectiveGain[c], truth.EffectiveGain[c], 0.05); err != nil {
			t.Error(err)
		}
	}

	if len(report.Diagnostics.CovarianceLabels) == 0 {
		t.Fatal("expected covariance labels")
	}
	if len(report.Diagnostics.Couplings) == 0 {
		t.Fatal("expected parameter coupling diagnostics")
	}
	maxAbsCorrelation := 0.0
	for _, coupling := range report.Diagnostics.Couplings {
		maxAbsCorrelation = math.Max(maxAbsCorrelation, math.Abs(coupling.Correlation))
	}
	if maxAbsCorrelation < 0.5 {
		t.Fatalf("expected visible coupling, max correlation %.3f", maxAbsCorrelation)
	}
}

func closeRel(name string, got, want, tolerance float64) error {
	if err := relError(got, want); err > tolerance {
		return fmt.Errorf("%s=%.5f want %.5f rel_err=%.3f > %.3f", name, got, want, err, tolerance)
	}
	return nil
}

func TestHighlightsShadowsAndReflectionsAreExposed(t *testing.T) {
	chart, truth := colorcal.NewSyntheticScenario(42)
	report, err := colorcal.Fit(chart)
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range truth.ReflectionPatches {
		patch := patchByID(t, report, id)
		if patch.Status != "unreliable" || !hasReason(patch, "local_reflection_or_outlier") {
			t.Fatalf("reflection patch %s not exposed: status=%s reasons=%v", id, patch.Status, patch.Reasons)
		}
	}
	for _, id := range truth.HighlightPatches {
		patch := patchByID(t, report, id)
		if patch.Status != "unreliable" || !hasReason(patch, "highlight_clipped") {
			t.Fatalf("highlight patch %s not exposed: status=%s reasons=%v", id, patch.Status, patch.Reasons)
		}
	}

	black := patchByID(t, report, "black")
	if black.Status == "reliable" || !hasReason(black, "shadow_low_snr") {
		t.Fatalf("shadow patch black not exposed: status=%s reasons=%v", black.Status, black.Reasons)
	}
}

func TestJointFitImprovesSkinIncludingHoldout(t *testing.T) {
	chart, _ := colorcal.NewSyntheticScenario(42)
	report, err := colorcal.Fit(chart)
	if err != nil {
		t.Fatal(err)
	}
	if report.NaiveMeanAlign == nil || report.JointFit == nil {
		t.Fatal("expected both baseline and joint accuracy summaries")
	}
	if !(report.JointFit.SkinDeltaE < report.NaiveMeanAlign.SkinDeltaE*0.65) {
		t.Fatalf("joint skin dE %.3f not clearly better than mean-align %.3f",
			report.JointFit.SkinDeltaE, report.NaiveMeanAlign.SkinDeltaE)
	}
	if report.JointFit.SkinDeltaE > 2.0 {
		t.Fatalf("joint skin dE %.3f > 2.0", report.JointFit.SkinDeltaE)
	}

	training := chart
	training.Patches = append([]colorcal.Patch(nil), chart.Patches...)
	for i := range training.Patches {
		if strings.Contains(training.Patches[i].ID, "skin") {
			training.Patches[i].MaskRGB = [3]bool{true, true, true}
		}
	}
	trained, err := colorcal.Fit(training)
	if err != nil {
		t.Fatal(err)
	}

	var holdoutDE []float64
	var naiveDE []float64
	var sumRef, sumMeas colorcal.RGB
	for _, all := range chart.Patches {
		for c := 0; c < 3; c++ {
			sumRef[c] += all.ReferenceRGB[c]
			sumMeas[c] += all.MeasuredRGB[c]
		}
	}
	var naiveGain colorcal.RGB
	for c := 0; c < 3; c++ {
		naiveGain[c] = sumRef[c] / sumMeas[c]
	}
	for _, patch := range chart.Patches {
		if !strings.Contains(patch.ID, "skin") {
			continue
		}
		corrected := colorcal.CorrectColor(patch.MeasuredRGB, trained.Params)
		holdoutDE = append(holdoutDE, colorcal.DeltaE2000(corrected, patch.ReferenceRGB))
		naiveCorrected := colorcal.RGB{
			patch.MeasuredRGB[0] * naiveGain[0],
			patch.MeasuredRGB[1] * naiveGain[1],
			patch.MeasuredRGB[2] * naiveGain[2],
		}
		naiveDE = append(naiveDE, colorcal.DeltaE2000(naiveCorrected, patch.ReferenceRGB))
	}
	jointHoldout := meanTestFloat(holdoutDE)
	naiveSkin := meanTestFloat(naiveDE)
	if jointHoldout > 2.2 {
		t.Fatalf("held-out skin dE %.3f > 2.2", jointHoldout)
	}
	if jointHoldout >= naiveSkin*0.7 {
		t.Fatalf("held-out joint skin dE %.3f not better than naive %.3f", jointHoldout, naiveSkin)
	}
}

func meanTestFloat(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}
