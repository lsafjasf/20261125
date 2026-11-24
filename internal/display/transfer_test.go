package display

import (
	"math"
	"testing"
)

func TestSRGBRoundTrip(t *testing.T) {
	for _, v := range []float64{0, 0.001, 0.0031308, 0.18, 0.5, 0.9, 1} {
		if got := SRGBDecode(SRGBEncode(v)); math.Abs(got-v) > 1e-12 {
			t.Errorf("sRGB round trip %.9f -> %.9f", v, got)
		}
	}
}

func TestShoulderPreservesKneeAndOrder(t *testing.T) {
	s := DefaultShoulder()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{0, 0.09, 0.18, 0.36, 0.7} {
		if got := s.Apply(v); got != v {
			t.Errorf("below knee: Apply(%.5f) = %.9f", v, got)
		}
	}
	prev := s.Apply(0.7)
	for v := 0.71; v <= 128; v *= 1.01 {
		got := s.Apply(v)
		if got <= prev {
			t.Fatalf("shoulder is not strictly increasing at %.5f: %.9f <= %.9f", v, got, prev)
		}
		prev = got
	}
	if got := s.Apply(1e9); got >= s.Ceiling {
		t.Fatalf("shoulder reached its exclusive ceiling: %.9f", got)
	}
}
