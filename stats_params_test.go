package cutluma

import (
	"math"
	"testing"
)

func TestQuantileAndMedian(t *testing.T) {
	xs := []float64{3, 1, 2, 5, 4}
	if got := Median(xs); got != 3 {
		t.Fatalf("median = %v, want 3", got)
	}
	if got := Quantile(xs, 0); got != 1 {
		t.Fatalf("q0 = %v, want 1", got)
	}
	if got := Quantile(xs, 1); got != 5 {
		t.Fatalf("q1 = %v, want 5", got)
	}
	if got := Quantile(nil, 0.5); got != 0 {
		t.Fatalf("empty quantile = %v, want 0", got)
	}
}

func TestTheilSenRecoversLine(t *testing.T) {
	xs := make([]float64, 0, 21)
	ys := make([]float64, 0, 21)
	for i := 0; i < 21; i++ {
		x := float64(i) - 10
		xs = append(xs, x)
		// Deterministic small wobble plus one outlier.
		y := 2 + 0.3*x + 0.01*math.Sin(x)
		if i == 17 {
			y += 0.8
		}
		ys = append(ys, y)
	}
	l := TheilSen(xs, ys)
	if math.Abs(l.Slope-0.3) > 0.01 {
		t.Fatalf("slope = %.4f, want ~0.3", l.Slope)
	}
	if math.Abs(l.Intercept-2) > 0.05 {
		t.Fatalf("intercept = %.4f, want ~2", l.Intercept)
	}
}

func TestConvertRoundTrip(t *testing.T) {
	for _, p := range segmentParamTable() {
		if err := p.Validate(); err != nil {
			t.Fatalf("invalid param %+v: %v", p, err)
		}
		for _, x := range []float64{0.08, 0.2, 0.4, 0.6, 0.8} {
			y := p.Apply(x)
			back := p.Inverse(y)
			if math.Abs(back-x) > 1e-9 {
				t.Fatalf("%+v roundtrip x=%.3f y=%.4f back=%.6f", p, x, y, back)
			}
		}
	}
}

func TestReconversionMap(t *testing.T) {
	a := DefaultParams()
	b := ConvertParams{Black: 0.02, White: 0.96, ExposureEV: 0.22, TransferPower: 1}
	lut, err := ReconversionMap(b, a, 257)
	if err != nil {
		t.Fatal(err)
	}
	// Mapping b-output through the LUT must reproduce a-output.
	for _, x := range []float64{0.1, 0.25, 0.4, 0.6, 0.8} {
		yb := b.Apply(x)
		got := ApplyLUT(lut, yb)
		want := a.Apply(x)
		if math.Abs(got-want) > 2e-4 {
			t.Fatalf("reconvert x=%.3f got %.5f want %.5f", x, got, want)
		}
	}
	id, err := ReconversionMap(a, a, 129)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 129; i++ {
		v := float64(i) / 128
		if math.Abs(ApplyLUT(id, v)-v) > 1e-9 {
			t.Fatalf("identity LUT at %.3f = %.5f", v, ApplyLUT(id, v))
		}
	}
}

func TestDiffParamsConsistency(t *testing.T) {
	a := DefaultParams()
	if !DiffParams(a, a).Consistent(DefaultTolerance()) {
		t.Fatal("identical params must be consistent")
	}
	big := ConvertParams{Black: 0.09, White: 0.9, ExposureEV: 0.5, TransferPower: 1.3}
	d := DiffParams(big, a)
	if d.Consistent(DefaultTolerance()) {
		t.Fatal("large drift must be inconsistent")
	}
	fields := inconsistentFields(d, DefaultTolerance())
	if len(fields) != 4 {
		t.Fatalf("expected 4 inconsistent fields, got %v", fields)
	}
}
