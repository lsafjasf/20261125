package icc

import (
	"testing"

	"icccmp/icc/fixture"
)

// TestEndToEnd builds one v2 and one v4 profile sharing primaries/white
// point and confirms the full comparison report is internally consistent.
func TestEndToEnd(t *testing.T) {
	matrix := func(b *fixture.Builder) *fixture.Builder {
		return b.
			AddXYZ("wtpt", 0.9642, 1.0, 0.8249).
			AddXYZ("rXYZ", 0.43607, 0.22249, 0.01392).
			AddXYZ("gXYZ", 0.38515, 0.71687, 0.09708).
			AddXYZ("bXYZ", 0.14307, 0.06061, 0.71410)
	}
	v2 := Parse(matrix(fixture.New(2, 4)).
		AddGamma("rTRC", 2.2).
		AddGamma("gTRC", 2.2).
		AddGamma("bTRC", 2.2).Build(), "v2.icc")
	srgb := []float64{2.4, 1.0 / 1.055, 0.055 / 1.055, 1.0 / 12.92, 0.04045}
	v4 := Parse(matrix(fixture.New(4, 3)).
		AddPara("rTRC", 3, srgb).
		AddPara("gTRC", 3, srgb).
		AddPara("bTRC", 3, srgb).
		AddRaw("chad", make([]byte, 44)).Build(), "v4.icc")

	if v2.Version.Major != 2 || v4.Version.Major != 4 {
		t.Fatalf("versions = %s / %s", v2.Version, v4.Version)
	}
	rep := Compare(v2, v4)

	if rep.WhitePoint.DeltaE76 > 1e-9 {
		t.Fatalf("shared white point should match, deltaE=%v", rep.WhitePoint.DeltaE76)
	}
	if len(rep.Matrix) != 3 {
		t.Fatalf("matrix columns = %d, want 3", len(rep.Matrix))
	}
	for _, m := range rep.Matrix {
		if m.MaxAbsDiff > 1e-9 {
			t.Fatalf("channel %s matrix differs: %v", m.Channel, m.MaxAbsDiff)
		}
	}
	if len(rep.Curves) != 3 {
		t.Fatalf("curve rows = %d, want 3", len(rep.Curves))
	}
	for _, c := range rep.Curves {
		if !c.Comparable || c.Stats.Samples != SamplePoints {
			t.Fatalf("channel %s not comparable: %s", c.Channel, c.Reason)
		}
	}
	// Only the v4 profile carries an out-of-subset tag.
	if len(rep.Unsupported.Left) != 0 || len(rep.Unsupported.Right) != 1 {
		t.Fatalf("unsupported tags = %+v / %+v", rep.Unsupported.Left, rep.Unsupported.Right)
	}
}
