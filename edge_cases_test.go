package cutluma

import (
	"math"
	"testing"
)

// makeMiniJob builds a two-segment one-cut Job whose median curve follows med.
// Other quantile columns are fixed offsets around the median.
func makeMiniJob(n, cut int, med func(t int) float64, p0, p1 *ConvertParams) *Job {
	offsets := []float64{-0.14, -0.10, -0.05, -0.025, 0, 0.025, 0.05, 0.10, 0.14}
	frames := make([]FrameRow, n)
	for t := 0; t < n; t++ {
		m := med(t)
		row := FrameRow{Frame: t, Out: make([]float64, len(QuantileLevels))}
		for i, off := range offsets {
			row.Out[i] = clamp01(m + off)
		}
		frames[t] = row
	}
	return &Job{
		Name: "mini", FrameCount: n, QuantileLevels: append([]float64(nil), QuantileLevels...),
		Cuts: []int{cut}, Frames: frames,
		Segments: []SegmentJob{
			{Index: 0, Start: 0, End: cut, Params: p0},
			{Index: 1, Start: cut, End: n, Params: p1},
		},
	}
}

func TestSteepSmoothLightChangeIsNatural(t *testing.T) {
	const n, cut = 200, 100
	// 0.14 amplitude smooth ramp over 20 frames centered exactly on the cut.
	med := func(t int) float64 {
		return 0.42 + 0.14*ramp(float64(t), float64(cut-10), float64(cut+10))
	}
	p := DefaultParams()
	job := makeMiniJob(n, cut, med, &p, &p)
	rep, err := Analyze(job, DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := rep.Cuts[0]
	if c.Kind != KindNatural {
		t.Fatalf("steep smooth ramp classified %s (gap=%.4f z=%.1f trend=%.4f drift=%+.4f), want natural_light_change",
			c.Kind, c.FitGap, c.ZScore, c.TrendChange, c.PreCutDrift)
	}
	if c.OutlierZ >= rep.Config.ZOutlier {
		t.Fatalf("smooth ramp boundary increment looks like outlier: z=%.1f", c.OutlierZ)
	}
	if c.ParamMismatch {
		t.Error("identical params must not mismatch")
	}
}

func TestFlatSceneIsConsistent(t *testing.T) {
	const n, cut = 200, 100
	p := DefaultParams()
	job := makeMiniJob(n, cut, func(int) float64 { return 0.42 }, &p, &p)
	rep, err := Analyze(job, DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Cuts[0].Kind != KindConsistent {
		t.Fatalf("flat cut = %s, want consistent", rep.Cuts[0].Kind)
	}
}

func TestAbruptStepWithEqualParamsIsNotBlamedOnConversion(t *testing.T) {
	const n, cut = 200, 100
	p := DefaultParams()
	med := func(t int) float64 {
		if t >= cut {
			return 0.53
		}
		return 0.42
	}
	job := makeMiniJob(n, cut, med, &p, &p)
	rep, err := Analyze(job, DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := rep.Cuts[0]
	if c.Kind == KindConversion || c.Kind == KindMixed {
		t.Fatalf("equal-param unexplained step classified %s; must not be conversion", c.Kind)
	}
	if c.Kind != KindAmbiguous && c.Kind != KindPreexisting {
		t.Fatalf("hard equal-param step = %s, want ambiguous/preexisting", c.Kind)
	}
}

func TestEmpiricalQuantileLUTMonotone(t *testing.T) {
	sc := BuildScenario()
	rep, err := Analyze(sc.Job, DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := rep.Segments[0].Quantiles
	lut := quantileLUT(rep.Segments[3].Quantiles, ref, 257)
	if len(lut) != 257 {
		t.Fatalf("lut len %d", len(lut))
	}
	for i := 1; i < len(lut); i++ {
		if lut[i]+1e-9 < lut[i-1] {
			t.Fatalf("LUT not monotone at %d: %.5f < %.5f", i, lut[i], lut[i-1])
		}
		if lut[i] < 0 || lut[i] > 1 {
			t.Fatalf("LUT out of range at %d: %.4f", i, lut[i])
		}
	}
}

func TestApplyTrimsRamp(t *testing.T) {
	p := DefaultParams()
	job := makeMiniJob(40, 20, func(t int) float64 {
		if t < 20 {
			return 0.40
		}
		return 0.50
	}, &p, &p)
	curve := quantileColumn(job, job.QIndex(0.50), false)
	plan := &RepairPlan{Trims: []SeamTrim{{
		CutIndex: 0, Frame: 20, LeftOffset: 0.05, RightOffset: -0.05, RampFrames: 4,
	}}}
	out := ApplyTrims(curve, job, plan)
	if math.Abs(out[19]-0.45) > 1e-9 {
		t.Errorf("nearest left frame = %.4f, want 0.45", out[19])
	}
	if math.Abs(out[20]-0.45) > 1e-9 {
		t.Errorf("nearest right frame = %.4f, want 0.45", out[20])
	}
	if math.Abs(out[16]-0.4125) > 1e-9 {
		t.Errorf("far ramp frame = %.4f, want 0.4125", out[16])
	}
	if math.Abs(out[15]-0.40) > 1e-9 {
		t.Errorf("frame outside ramp changed: %.4f", out[15])
	}
}

func TestRepairPlanWithoutDeclaredParams(t *testing.T) {
	sc := BuildScenario()
	for i := range sc.Job.Segments {
		sc.Job.Segments[i].Params = nil
	}
	rep, err := Analyze(sc.Job, DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRepairPlan(sc.Job, rep, 129)
	if err != nil {
		t.Fatal(err)
	}
	for _, sr := range plan.Segments {
		if sr.Segment != 0 && sr.Mode != "quantile_lut" {
			t.Errorf("seg %d mode = %s, want quantile_lut", sr.Segment, sr.Mode)
		}
		for i := 1; i < len(sr.LUT); i++ {
			if sr.LUT[i]+1e-9 < sr.LUT[i-1] {
				t.Fatalf("seg %d LUT non-monotone at %d", sr.Segment, i)
			}
		}
	}
}
