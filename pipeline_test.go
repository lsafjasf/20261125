package cutluma

import (
	"os"
	"path/filepath"
	"testing"
)

func runScenario(t *testing.T) (*Scenario, *Report, *RepairPlan) {
	t.Helper()
	sc := BuildScenario()
	rep, err := Analyze(sc.Job, DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	plan, err := BuildRepairPlan(sc.Job, rep, 257)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	return sc, rep, plan
}

func TestAllNineCutsMatchScriptedCause(t *testing.T) {
	sc, rep, _ := runScenario(t)
	if len(rep.Cuts) != 9 {
		t.Fatalf("cuts = %d, want 9", len(rep.Cuts))
	}
	for i, truth := range sc.Truth {
		got := rep.Cuts[i].Kind
		if got != truth.Kind {
			t.Errorf("cut %d: kind = %s, want %s (%s)", i, got, truth.Kind, truth.Note)
		}
	}
}

func TestSegmentParameterConsistency(t *testing.T) {
	_, rep, _ := runScenario(t)
	if len(rep.Segments) != 10 {
		t.Fatalf("segments = %d, want 10", len(rep.Segments))
	}
	for i, s := range rep.Segments {
		if !s.DeclaredAvailable || s.Declared == nil {
			t.Fatalf("seg %d missing declared params", i)
		}
	}
	if !rep.Segments[0].ParamsConsistent {
		t.Error("reference segment 0 must be consistent with target")
	}
	if rep.Segments[1].ParamsConsistent {
		t.Error("segment 1 (+0.22 EV) must be flagged inconsistent")
	}
	foundEV := false
	for _, f := range rep.Segments[1].InconsistentFields {
		if f == "exposureEV" {
			foundEV = true
		}
	}
	if !foundEV {
		t.Errorf("segment 1 fields = %v, want exposureEV", rep.Segments[1].InconsistentFields)
	}
}

func TestConversionSeamsAreRepaired(t *testing.T) {
	_, _, plan := runScenario(t)
	byCut := map[int]CutVerification{}
	for _, v := range plan.Verification {
		byCut[v.CutIndex] = v
	}
	for _, ci := range []int{0, 1, 3, 4, 5, 7} {
		v := byCut[ci]
		if v.AbsAfter > 0.006 {
			t.Errorf("cut %d residual gap %.5f > 0.006", ci, v.AbsAfter)
		}
		if v.ReductionPct < 90 {
			t.Errorf("cut %d reduction %.1f%% < 90%%", ci, v.ReductionPct)
		}
	}
}

func TestNaturalLightIsNotFlaggedOrFlattened(t *testing.T) {
	_, rep, plan := runScenario(t)
	byCut := map[int]CutVerification{}
	for _, v := range plan.Verification {
		byCut[v.CutIndex] = v
	}
	for _, ci := range []int{2, 6} {
		c := rep.Cuts[ci]
		if c.Kind != KindNatural {
			t.Errorf("cut %d classified %s, want natural_light_change", ci, c.Kind)
		}
		if c.ParamMismatch {
			t.Errorf("cut %d must not show param mismatch", ci)
		}
		v := byCut[ci]
		if v.AbsAfter > 0.008 {
			t.Errorf("natural cut %d corrected gap %.5f too large", ci, v.AbsAfter)
		}
		if v.SlopeKeptPct < 80 || v.SlopeKeptPct > 125 {
			t.Errorf("natural cut %d slope kept %.1f%% out of [80,125]", ci, v.SlopeKeptPct)
		}
	}
}

func TestPreexistingSourceStepIsPreserved(t *testing.T) {
	_, rep, plan := runScenario(t)
	c := rep.Cuts[8]
	if c.Kind != KindPreexisting {
		t.Fatalf("cut 8 = %s, want preexisting_step", c.Kind)
	}
	var v CutVerification
	for _, vv := range plan.Verification {
		if vv.CutIndex == 8 {
			v = vv
		}
	}
	if v.AbsAfter < 0.05 {
		t.Errorf("source step erased: after gap %.4f < 0.05", v.AbsAfter)
	}
	if v.ReductionPct > 15 {
		t.Errorf("source step reduction %.1f%% too high; should be preserved", v.ReductionPct)
	}
}

func TestAttributionWithoutSourceCurve(t *testing.T) {
	sc, _, _ := runScenario(t)
	for i := range sc.Job.Frames {
		sc.Job.Frames[i].Src = nil
	}
	rep2, err := Analyze(sc.Job, DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, ci := range []int{0, 1, 3, 4, 5, 7} {
		k := rep2.Cuts[ci].Kind
		if k != KindConversion && k != KindMixed {
			t.Errorf("no-source cut %d = %s, want conversion/mixed", ci, k)
		}
	}
	for _, ci := range []int{2, 6} {
		if rep2.Cuts[ci].Kind != KindNatural {
			t.Errorf("no-source natural cut %d = %s, want natural_light_change", ci, rep2.Cuts[ci].Kind)
		}
	}
	if rep2.Cuts[8].Kind == KindConversion || rep2.Cuts[8].Kind == KindMixed {
		t.Errorf("equal-param hard step must not be blamed on conversion, got %s", rep2.Cuts[8].Kind)
	}
}

func TestArtifactsAndJobRoundTrip(t *testing.T) {
	sc, rep, plan := runScenario(t)
	dir := t.TempDir()
	sum, err := WriteArtifacts(dir, sc.Job, rep, plan, sc.Truth)
	if err != nil {
		t.Fatal(err)
	}
	if !sum.AllTruthMatched {
		t.Error("summary must report all truth matched")
	}
	for _, name := range []string{
		"summary.json", "report.json", "repair_params.json", "input_job.json",
		"segment_report.csv", "cut_report.csv", "curves_cross_cut.csv",
		"curve_timeline.csv", "curves.svg", "summary.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing artifact %s", name)
		}
	}
	j2, err := LoadJobJSON(filepath.Join(dir, "input_job.json"))
	if err != nil {
		t.Fatal(err)
	}
	rep2, err := Analyze(j2, DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rep.Cuts {
		if rep2.Cuts[i].Kind != rep.Cuts[i].Kind {
			t.Errorf("roundtrip cut %d kind %s -> %s", i, rep.Cuts[i].Kind, rep2.Cuts[i].Kind)
		}
	}
}
