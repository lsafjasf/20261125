package colorconv

import "testing"

func allSpecs(bits int) []Spec {
	return []Spec{
		{BT601, Limited, bits}, {BT709, Limited, bits},
		{BT601, Full, bits}, {BT709, Full, bits},
	}
}

// declared error budget, mirrored in cmd/compare/tolerance.go
func budget(bits int) (maxAbs int, mean, bias float64) {
	if bits == 10 {
		return 6, 1.5, 0.75
	}
	return 2, 0.5, 0.25
}

func check(t *testing.T, name string, s Spec, st Stats) {
	t.Helper()
	wantMax, wantMean, wantBias := budget(s.Bits)
	if !Within(st, wantMax, wantMean, wantBias) {
		t.Errorf("%s %s: max=%d mean=%.3f bias=%.3f, budget max=%d mean=%.2f bias=%.2f",
			name, s, st.MaxAbs, st.MeanAbs, st.Bias, wantMax, wantMean, wantBias)
	}
}

func TestQualifiedImplementations(t *testing.T) {
	impls := map[string]func(Spec) Encoder{
		"float64":  func(s Spec) Encoder { return FloatEncoder{Spec: s} },
		"fixedQ16": func(s Spec) Encoder { return FixedEncoder{Spec: s, Q: 16, Round: true} },
		"fixedQ12": func(s Spec) Encoder { return FixedEncoder{Spec: s, Q: 12, Round: true} },
	}
	for _, bits := range []int{8, 10} {
		samples := Samples(bits)
		for _, s := range allSpecs(bits) {
			ref := FloatEncoder{Spec: s}
			for name, mk := range impls {
				enc := mk(s)
				check(t, name, s, RoundTrip(enc, samples))
				fd := ForwardDrift(enc, ref, samples)
				if fd.MaxAbs > 1 {
					t.Errorf("%s %s forward drift max=%d, want <=1 code", name, s, fd.MaxAbs)
				}
			}
		}
	}
}

func TestRoundedInteger(t *testing.T) {
	// Q8 with symmetric rounding passes the hard LSB cap everywhere, but its
	// coarse coefficient table breaks the mean/bias budget for BT.709 at
	// 8-bit and for most 10-bit specs: only BT.601 8-bit fully qualifies.
	for _, s := range allSpecs(8) {
		st := RoundTrip(NewRoundedInteger(s), Samples(8))
		if s.Matrix == BT601 {
			check(t, "intQ8-round-8bit", s, st)
		} else {
			m, mn, b := budget(8)
			if Within(st, m, mn, b) {
				t.Errorf("intQ8-round %s unexpectedly met full 8-bit budget", s)
			}
		}
	}
	for _, s := range allSpecs(10) {
		st := RoundTrip(NewRoundedInteger(s), Samples(10))
		m, mn, b := budget(10)
		passes := Within(st, m, mn, b)
		if s.Matrix == BT709 && passes {
			t.Errorf("intQ8-round %s unexpectedly met full 10-bit budget", s)
		}
		if s.Matrix == BT601 && !passes {
			t.Errorf("intQ8-round %s should meet the 10-bit budget, got %+v", s, st)
		}
	}
}

func TestNaiveTruncationFails(t *testing.T) {
	// The floor-shift variant must violate the budget on every spec and show
	// a nonzero signed bias, concentrated on saturated pure colors.
	for _, bits := range []int{8, 10} {
		for _, s := range allSpecs(bits) {
			full := RoundTrip(NewNaiveInteger(s), Samples(bits))
			sat := RoundTrip(NewNaiveInteger(s), Saturated(bits))
			m, mn, b := budget(bits)
			if Within(full, m, mn, b) {
				t.Errorf("naive %s should violate the full budget, got max=%d mean=%.3f bias=%.3f",
					s, full.MaxAbs, full.MeanAbs, full.Bias)
			}
			if sat.Bias >= 0 {
				t.Errorf("naive %s saturated bias should be negative (floor drift), got %.3f",
					s, sat.Bias)
			}
		}
	}
}

func Within(st Stats, maxAbs int, mean, bias float64) bool {
	b := st.Bias
	if b < 0 {
		b = -b
	}
	return st.MaxAbs <= maxAbs && st.MeanAbs <= mean && b <= bias
}

func TestExtremaRoundTrip(t *testing.T) {
	// Explicit extreme-value guard for both depths and both ranges.
	for _, bits := range []int{8, 10} {
		max := (1 << bits) - 1
		corners := []RGB{
			{0, 0, 0}, {max, max, max},
			{max, 0, 0}, {0, max, 0}, {0, 0, max},
			{max, max, 0}, {max, 0, max}, {0, max, max},
		}
		for _, s := range allSpecs(bits) {
			enc := FixedEncoder{Spec: s, Q: 16, Round: true}
			for _, in := range corners {
				out := enc.Decode(enc.Encode(in))
				for _, d := range [3]int{out.R - in.R, out.G - in.G, out.B - in.B} {
					if d < -2 || d > 2 {
						t.Errorf("%s extrema %v -> %v error %d", s, in, out, d)
					}
				}
			}
		}
	}
}

func TestDefectControlsFail(t *testing.T) {
	s := Spec{BT709, Limited, 8}
	other := Spec{BT601, Limited, 8}
	samples := Samples(8)
	defects := map[string]Encoder{
		"matrix-mismatch": MatrixMismatchEncoder{EncodeSpec: s, DecodeSpec: other},
		"offset-dropped":  OffsetlessEncoder{Spec: s},
		"range-confusion": RangeConfusionEncoder{Spec: s, EncodeRange: Limited, DecodeRange: Full},
		"uv-swapped":      SwappedUVEncoder{Spec: s},
	}
	for name, enc := range defects {
		st := RoundTrip(enc, samples)
		if st.MaxAbs <= 2 {
			t.Errorf("defect %s should blow the budget, got max=%d", name, st.MaxAbs)
		}
	}
}
