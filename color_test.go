package colorcal

import "testing"

func TestCIEDE2000KnownPair(t *testing.T) {
	// Sharma/Wu/Dalal CIEDE2000 verification pair.
	first := [3]float64{50, 2.6772, -79.7751}
	second := [3]float64{50, 0, -82.7485}
	got := labDeltaE2000(first, second)
	want := 2.0425
	if diff := got - want; diff > 0.01 || diff < -0.01 {
		t.Fatalf("DeltaE2000=%.4f, want %.4f", got, want)
	}
}

func TestSRGBRoundTripMonotonic(t *testing.T) {
	for _, v := range []float64{0, 0.02, 0.2, 0.7, 1} {
		linear := SRGBToLinear(v)
		if linear < 0 || linear > 1 {
			t.Fatalf("linear value out of range: %f", linear)
		}
	}
}
