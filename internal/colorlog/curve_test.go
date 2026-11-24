package colorlog

import (
	"math"
	"testing"
)

func TestPublishedAnchors(t *testing.T) {
	tests := []struct {
		curve Curve
		mid   float64
		black float64
	}{
		{Curve{Kind: SLog3}, 420.0 / 1023.0, 95.0 / 1023.0},
		{Curve{Kind: LogC3}, 0.391, 0.092809},
		{Curve{Kind: ACEScct}, 0.4136, 0.0729055},
	}
	for _, tt := range tests {
		if got := tt.curve.Encode(0.18); math.Abs(got-tt.mid) > 0.0006 {
			t.Errorf("%s mid-gray = %.7f, want %.7f", tt.curve.Name(), got, tt.mid)
		}
		if got := tt.curve.Encode(0); math.Abs(got-tt.black) > 0.000001 {
			t.Errorf("%s black = %.7f, want %.7f", tt.curve.Name(), got, tt.black)
		}
	}
}

func TestDecodeInvertsEncode(t *testing.T) {
	for _, curve := range Profiles() {
		for _, scene := range []float64{0, 0.001, 0.01, 0.18, 0.5, 1, 8, 32} {
			if scene > curve.Decode(1) {
				continue
			}
			got := curve.Decode(curve.Encode(scene))
			want := math.Max(scene, 1e-9)
			got = math.Max(got, 1e-9)
			if math.Abs(got-want)/want > 1e-6 {
				t.Errorf("%s round trip %.8f -> %.8f", curve.Name(), scene, got)
			}
		}
	}
}

func TestMonotonicCurves(t *testing.T) {
	for _, curve := range Profiles() {
		prev := -1.0
		for code := 0.0; code <= 1.0; code += 1.0 / 4096 {
			scene := curve.Decode(code)
			if scene < prev {
				t.Fatalf("%s decode decreases at %.6f", curve.Name(), code)
			}
			prev = scene
		}
	}
}
