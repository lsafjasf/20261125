package pipeline

import (
	"cutcheck/internal/cmx"
	"cutcheck/internal/compare"
	"cutcheck/internal/fcpxml"
	"cutcheck/internal/timeline"
	"path/filepath"
	"testing"
)

func loadTestList(t *testing.T) *timeline.EditList {
	t.Helper()
	el, err := timeline.Load(filepath.Join("..", "..", "editlist.json"))
	if err != nil {
		t.Fatal(err)
	}
	return el
}

func diagnosisFor(res *PhaseResult, format, layer string) compare.Diagnosis {
	for _, d := range res.Diagnoses {
		if d.Format == format && d.Layer == layer {
			return d
		}
	}
	return compare.Diagnosis{Pattern: "missing"}
}

// TestBrokenExportsAreDiagnosed reproduces the misconfigured exports and
// checks that the drift/offset are localized to the right layers.
func TestBrokenExportsAreDiagnosed(t *testing.T) {
	el := loadTestList(t)
	declared := DeclaredNonEquivalents(el)
	dir := t.TempDir()

	buggyCMX := cmx.CorrectConfig(el)
	buggyCMX.FPSNum, buggyCMX.FPSDen = 30, 1
	buggyFCP := fcpxml.ExportConfig{AudioPrimingSamples: 1024}
	cmxPath, fcpPath, _, err := ExportPair(el, dir, buggyCMX, buggyFCP)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ReopenAndCheck(el, cmxPath, fcpPath, declared)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failures) == 0 {
		t.Fatal("expected equivalence failures with broken export params")
	}
	if got := diagnosisFor(res, "cmx", "video"); got.Pattern != compare.PatternDrift {
		t.Errorf("cmx/video pattern = %s, want cumulative-drift", got.Pattern)
	}
	if got := diagnosisFor(res, "fcpxml", "audio"); got.Pattern != compare.PatternConstant {
		t.Errorf("fcpxml/audio pattern = %s, want constant-offset", got.Pattern)
	}
	if got := diagnosisFor(res, "fcpxml", "video"); got.Pattern != compare.PatternNone {
		t.Errorf("fcpxml/video pattern = %s, want match", got.Pattern)
	}
	// Every canonical event must be mapped in both formats.
	for _, row := range res.Mapping {
		if !row.Found["cmx"] || !row.Found["fcpxml"] {
			t.Errorf("event %s not mapped in both formats: %v", row.EventID, row.Found)
		}
	}
}

// TestFixedExportsAreEquivalent re-opens the corrected exports and
// verifies the equivalence proof passes.
func TestFixedExportsAreEquivalent(t *testing.T) {
	el := loadTestList(t)
	declared := DeclaredNonEquivalents(el)
	dir := t.TempDir()

	cmxPath, fcpPath, _, err := ExportPair(el, dir, cmx.CorrectConfig(el), fcpxml.CorrectConfig(el))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ReopenAndCheck(el, cmxPath, fcpPath, declared)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failures) > 0 {
		t.Fatalf("unexpected failures: %v", res.Failures)
	}
	for _, d := range res.Diagnoses {
		if d.Pattern != compare.PatternNone {
			t.Errorf("%s/%s pattern = %s, want match", d.Format, d.Layer, d.Pattern)
		}
	}
	// The retime event must be declared non-equivalent for CMX.
	found := false
	for _, d := range declared {
		if d.EventID == "V4" && d.Format == "cmx" {
			found = true
		}
	}
	if !found {
		t.Error("V4 retime not declared non-equivalent for cmx")
	}
}
