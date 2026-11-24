package pq

import (
	"math"
	"testing"
)

func approx(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol
}

func TestEOTFAnchors(t *testing.T) {
	cases := []struct {
		n    float64
		want float64
	}{
		{0.0, 0.0},
		{0.01, 0.0023},
		{0.5, 92.2457},   // widely cited PQ midpoint
		{0.75, 983.3779}, // widely cited ~983 nits
		{1.0, 10000.0},
	}
	for _, c := range cases {
		if got := EOTF(c.n); !approx(got, c.want, 0.01) {
			t.Errorf("EOTF(%v) = %v, want %v", c.n, got, c.want)
		}
	}
}

func TestOETFAnchors(t *testing.T) {
	cases := []struct {
		nits float64
		want float64
	}{
		{0, 0},
		{100, 0.5081},
		{203, 0.5807},
		{1000, 0.7518},
		{10000, 1.0},
	}
	for _, c := range cases {
		if got := OETF(c.nits); !approx(got, c.want, 0.0005) {
			t.Errorf("OETF(%v) = %v, want %v", c.nits, got, c.want)
		}
	}
}

func TestCodeConversions(t *testing.T) {
	if got := NitsToCode(1000, 10); got != 769 {
		t.Errorf("NitsToCode(1000,10) = %d, want 769", got)
	}
	if got := NitsToCode(203, 10); got != 594 {
		t.Errorf("NitsToCode(203,10) = %d, want 594", got)
	}
	if got := NitsToCode(100, 10); got != 520 {
		t.Errorf("NitsToCode(100,10) = %d, want 520", got)
	}
	if got := CodeToNits(1023, 10); got != 10000 {
		t.Errorf("CodeToNits(1023,10) = %v, want 10000", got)
	}
	if got := CodeToNits(0, 10); got != 0 {
		t.Errorf("CodeToNits(0,10) = %v, want 0", got)
	}
}

func TestRoundTripExact(t *testing.T) {
	for depth := 8; depth <= 16; depth++ {
		maxCode := MaxCode(depth)
		// Full scan for shallow depths; sampled scan for 16-bit.
		step := 1
		if depth > 12 {
			step = 257
		}
		for code := 0; code <= maxCode; code += step {
			back := NitsToCode(CodeToNits(code, depth), depth)
			if d := back - code; d < -1 || d > 1 {
				t.Fatalf("depth %d code %d round trip -> %d", depth, code, back)
			}
		}
	}
}

func TestClamping(t *testing.T) {
	if got := EOTF(2); got != 10000 {
		t.Errorf("EOTF above 1 not clamped: %v", got)
	}
	if got := OETF(20000); got != 1 {
		t.Errorf("OETF above peak not clamped: %v", got)
	}
	if got := NitsToCode(-5, 10); got != 0 {
		t.Errorf("negative nits not clamped to 0: %d", got)
	}
}
