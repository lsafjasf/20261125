package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"colorcal"
)

func runVerify(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	truthPath := fs.String("truth", "", "acceptance-only generated truth JSON")
	reportPath := fs.String("report", "", "fit report JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *truthPath == "" || *reportPath == "" {
		return errors.New("verify requires -truth and -report")
	}

	truth, err := loadTruth(*truthPath)
	if err != nil {
		return err
	}
	report, err := loadReport(*reportPath)
	if err != nil {
		return err
	}
	if err := verifyReport(truth, report); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "verify OK: parameters match generated truth within tolerance\n")
	fmt.Fprintf(stdout, "skin DeltaE2000 %.2f (mean align) -> %.2f (joint)\n",
		report.NaiveMeanAlign.SkinDeltaE, report.JointFit.SkinDeltaE)
	return nil
}

func loadTruth(path string) (colorcal.Truth, error) {
	var truth colorcal.Truth
	data, err := os.ReadFile(path)
	if err != nil {
		return truth, err
	}
	if err := json.Unmarshal(data, &truth); err != nil {
		return truth, fmt.Errorf("parse truth %s: %w", path, err)
	}
	return truth, nil
}

func loadReport(path string) (colorcal.Report, error) {
	var report colorcal.Report
	data, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return report, fmt.Errorf("parse report %s: %w", path, err)
	}
	return report, nil
}

func verifyReport(truth colorcal.Truth, report colorcal.Report) error {
	checks := []struct {
		name      string
		got, want float64
		tolerance float64
	}{
		{"exposure", report.Params.Exposure, truth.Exposure, 0.06},
	}
	for c, name := range []string{"r", "g", "b"} {
		checks = append(checks,
			struct {
				name      string
				got, want float64
				tolerance float64
			}{"wb_" + name, report.Params.WhiteBalance[c], truth.WhiteBalance[c], 0.07},
			struct {
				name      string
				got, want float64
				tolerance float64
			}{"gamma_" + name, report.Params.Gamma[c], truth.Gamma[c], 0.06},
			struct {
				name      string
				got, want float64
				tolerance float64
			}{"gain_" + name, report.Params.EffectiveGain[c], truth.EffectiveGain[c], 0.06},
		)
	}
	for _, check := range checks {
		if err := closeRelative(check.name, check.got, check.want, check.tolerance); err != nil {
			return err
		}
	}

	patches := map[string]colorcal.PatchReport{}
	for _, patch := range report.Patches {
		patches[patch.ID] = patch
	}
	for _, id := range truth.ReflectionPatches {
		patch, ok := patches[id]
		if !ok {
			return fmt.Errorf("missing reflection patch %q", id)
		}
		if patch.Status != "unreliable" || !hasReportReason(patch, "local_reflection_or_outlier") {
			return fmt.Errorf("reflection patch %q not exposed: %s %v", id, patch.Status, patch.Reasons)
		}
	}
	for _, id := range truth.HighlightPatches {
		patch, ok := patches[id]
		if !ok {
			return fmt.Errorf("missing highlight patch %q", id)
		}
		if patch.Status != "unreliable" || !hasReportReason(patch, "highlight_clipped") {
			return fmt.Errorf("highlight patch %q not exposed: %s %v", id, patch.Status, patch.Reasons)
		}
	}

	black, ok := patches["black"]
	if !ok {
		return errors.New("missing black patch")
	}
	if black.Status == "reliable" || !hasReportReason(black, "shadow_low_snr") {
		return fmt.Errorf("shadow patch not exposed: %s %v", black.Status, black.Reasons)
	}
	if len(report.Diagnostics.Couplings) == 0 {
		return errors.New("coupling diagnostics are empty")
	}
	gaugeNote := false
	for _, note := range report.Diagnostics.Notes {
		if strings.Contains(note, "geomean(WB)=1") {
			gaugeNote = true
			break
		}
	}
	if !gaugeNote {
		return errors.New("exposure/WB gauge note is missing")
	}
	if report.JointFit == nil || report.NaiveMeanAlign == nil {
		return errors.New("accuracy summaries are missing")
	}
	if report.JointFit.SkinDeltaE >= 2.0 {
		return fmt.Errorf("joint skin DeltaE=%.3f >= 2", report.JointFit.SkinDeltaE)
	}
	if report.JointFit.SkinDeltaE >= 0.65*report.NaiveMeanAlign.SkinDeltaE {
		return fmt.Errorf("joint skin DeltaE %.3f not 35%% better than naive %.3f",
			report.JointFit.SkinDeltaE, report.NaiveMeanAlign.SkinDeltaE)
	}
	return nil
}

func closeRelative(name string, got, want, tolerance float64) error {
	err := math.Abs(got-want) / math.Max(math.Abs(want), 1e-12)
	if err > tolerance {
		return fmt.Errorf("%s=%.5f want %.5f rel_err=%.3f > %.3f", name, got, want, err, tolerance)
	}
	return nil
}

func hasReportReason(patch colorcal.PatchReport, fragment string) bool {
	for _, reason := range patch.Reasons {
		if strings.Contains(reason, fragment) {
			return true
		}
	}
	return false
}
