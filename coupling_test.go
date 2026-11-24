package colorcal_test

import (
	"math"
	"testing"

	"colorcal"
)

func TestNarrowTonalRangeExposesIdentifiabilityCoupling(t *testing.T) {
	trueGain := colorcal.RGB{1.45, 1.10, 0.88}
	trueGamma := colorcal.RGB{0.92, 1.02, 1.10}
	values := []float64{0.180, 0.188, 0.196, 0.204, 0.212, 0.220}

	chart := colorcal.Chart{SchemaVersion: "test", SaturationLevel: 1}
	for i, value := range values {
		patch := colorcal.Patch{
			ID:           "narrow-" + string(rune('a'+i)),
			ReferenceRGB: colorcal.RGB{value, value, value},
			SigmaRGB:     colorcal.RGB{0.002, 0.002, 0.002},
		}
		for c := 0; c < 3; c++ {
			patch.MeasuredRGB[c] = math.Pow(trueGain[c]*value, trueGamma[c])
		}
		chart.Patches = append(chart.Patches, patch)
	}

	report, err := colorcal.Fit(chart)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("condition=%.3g identifiable=%v couplings=%+v",
		report.Diagnostics.MatrixCondition, report.Diagnostics.Identifiable, report.Diagnostics.Couplings)
	for i, label := range report.Diagnostics.CovarianceLabels {
		t.Logf("COV %-14s diag=% .6g row=% .4g", label, report.Diagnostics.Covariance[i][i], report.Diagnostics.Correlation[i])
	}

	if report.Diagnostics.MatrixCondition < 1e3 {
		t.Fatalf("expected ill-conditioned fit, condition=%.3g", report.Diagnostics.MatrixCondition)
	}
	found := false
	for _, coupling := range report.Diagnostics.Couplings {
		if coupling.Parameters[0] == "log_gain_g" && coupling.Parameters[1] == "log_gamma_g" &&
			math.Abs(coupling.Correlation) > 0.9 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected critical gain/gamma coupling, got %+v", report.Diagnostics.Couplings)
	}
}
