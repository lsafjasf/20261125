package coloradapt

import "testing"

func TestBenchmarkDatasetCoverage(t *testing.T) {
	samples := BuildSamples()
	if len(samples) != 144 {
		t.Fatalf("sample count = %d, want 144", len(samples))
	}

	groupCounts := map[string]int{}
	illuminantCounts := map[string]int{}
	for _, sample := range samples {
		groupCounts[sample.Group]++
		illuminantCounts[sample.Illuminant]++
	}

	requiredGroups := map[string]int{
		"neutral":              15,
		"skin":                 15,
		"spectral_chroma":      25,
		"ap0_gamut_edge":       21,
		"ap0_gamut_near_edge":  42,
		"encoding_cube_vertex": 8,
		"encoding_cube_edge":   12,
		"encoding_cube_face":   6,
	}
	for group, want := range requiredGroups {
		if groupCounts[group] != want {
			t.Fatalf("group %s count = %d, want %d", group, groupCounts[group], want)
		}
	}

	requiredIlluminants := []string{
		"A_2856K",
		"warm_3500K",
		"neutral_5000K",
		"daylight_6500K",
		"synthetic_equal_power",
		"ACES_D60",
	}
	for _, illuminant := range requiredIlluminants {
		if illuminantCounts[illuminant] == 0 {
			t.Fatalf("missing illuminant %s", illuminant)
		}
	}
}

func TestMethodSummaries(t *testing.T) {
	samples := BuildSamples()
	bradford := Summarize(Evaluate(MethodBradford, samples))
	vonKries := Summarize(Evaluate(MethodVonKries, samples))
	keepXYZ := Summarize(Evaluate(MethodKeepXYZ, samples))

	if bradford.WhitePointDeltaE != 0 {
		t.Fatalf("Bradford white ΔE = %.12f", bradford.WhitePointDeltaE)
	}
	if vonKries.WhitePointDeltaE != 0 {
		t.Fatalf("Von Kries white ΔE = %.12f", vonKries.WhitePointDeltaE)
	}
	if keepXYZ.WhitePointDeltaE <= 10 {
		t.Fatalf("keep XYZ white ΔE = %.6f, expected a D60/D50 mismatch", keepXYZ.WhitePointDeltaE)
	}
	if vonKries.BoundaryPerceptualMax <= 10 {
		t.Fatalf("Von Kries boundary ΔE = %.6f, expected a measurable boundary difference", vonKries.BoundaryPerceptualMax)
	}
	if bradford.RoundTripMax > 1e-10 || vonKries.RoundTripMax > 1e-10 || keepXYZ.RoundTripMax > 1e-10 {
		t.Fatalf("round trips exceed tolerance: Bradford=%.3e VonKries=%.3e KeepXYZ=%.3e",
			bradford.RoundTripMax, vonKries.RoundTripMax, keepXYZ.RoundTripMax)
	}
}

func TestDeltaE2000IdenticalColor(t *testing.T) {
	color := Lab{L: 54.3, A: 21.1, B: -17.8}
	if DeltaE2000(color, color) != 0 {
		t.Fatal("identical Lab colors have nonzero ΔE2000")
	}
}

func TestEmptyResultsDoNotPanic(t *testing.T) {
	if results := Evaluate(MethodBradford, nil); results != nil {
		t.Fatalf("empty evaluation = %v, want nil", results)
	}
	if summary := Summarize(nil); summary.Count != 0 {
		t.Fatalf("empty summary count = %d, want 0", summary.Count)
	}
	if group := SummarizeGroup(nil, "missing"); group.Count != 0 || group.P95 != 0 {
		t.Fatalf("empty group summary = %+v", group)
	}
}
