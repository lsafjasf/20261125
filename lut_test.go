package lutdiff

import (
	"math"
	"testing"
)

func TestIdentityLUT(t *testing.T) {
	lut := IdentityLUT(17)
	points := []RGB{
		{0, 0, 0}, {1, 1, 1}, {0.5, 0.25, 0.75},
		{0.137, 0.519, 0.901}, {0.999, 0.001, 0.333},
	}
	for _, c := range points {
		got := lut.Apply(c)
		if DistRGB(got, c) > 1e-12 {
			t.Fatalf("identity LUT changed %v -> %v", c, got)
		}
	}
}

func TestBuildLUTHitsNodes(t *testing.T) {
	const n = 9
	lut := BuildLUT(n, TransformA)
	for r := 0; r < n; r++ {
		for g := 0; g < n; g++ {
			for b := 0; b < n; b++ {
				den := float64(n - 1)
				c := RGB{float64(r) / den, float64(g) / den, float64(b) / den}
				got := lut.Apply(c)
				want := TransformA(c).Clamp()
				if DistRGB(got, want) > 1e-12 {
					t.Fatalf("node evaluation mismatch at %v: got %v want %v", c, got, want)
				}
			}
		}
	}
}

func TestNeutralColorsCommute(t *testing.T) {
	p := NewPipeline(33)
	for _, s := range AnalyzeSamples(p) {
		if !hasGroup(s.Groups, GroupNeutral) {
			continue
		}
		if s.Dist > 1e-9 || s.CodeDelta > 1e-6 {
			t.Fatalf("neutral sample %s %s should commute, got dist=%.3e code=%.3e AB=%v BA=%v",
				s.ID, s.Name, s.Dist, s.CodeDelta, s.AB, s.BA)
		}
	}

	var interiorWorst, fullWorst float64
	for i := 0; i <= 256; i++ {
		v := float64(i) / 256
		c := RGB{v, v, v}
		d := MaxCodeDelta8(p.AB(c), p.BA(c))
		fullWorst = math.Max(fullWorst, d)
		if v >= 0.05 && v <= 0.95 {
			interiorWorst = math.Max(interiorWorst, d)
		}
	}
	if interiorWorst > 1e-6 {
		t.Fatalf("interior gray axis must commute exactly, worst code delta %.3e", interiorWorst)
	}
	if fullWorst > 3.0 {
		t.Fatalf("gray axis boundary-cell interpolation artifact too large: %.3f codes", fullWorst)
	}
}

func TestHighSaturationDoesNotCommute(t *testing.T) {
	p := NewPipeline(33)
	strong := 0
	for _, s := range AnalyzeSamples(p) {
		if !hasGroup(s.Groups, GroupSaturated) {
			continue
		}
		if s.CodeDelta >= 8 {
			strong++
		}
	}
	if strong < 3 {
		t.Fatalf("expected at least 3 high-saturation samples with >=8 code-value order delta, got %d", strong)
	}

	a := SweepCube(p, 21)
	st := a.Regions[RegionSaturatedBit]
	if st.MaxCodeDelta < 20 {
		t.Fatalf("saturated region max code delta too small: %.2f", st.MaxCodeDelta)
	}
	if st.MaxDeltaE < 5 {
		t.Fatalf("saturated region max DeltaE too small: %.2f", st.MaxDeltaE)
	}
}

func TestNearBoundaryOrderSensitive(t *testing.T) {
	p := NewPipeline(33)
	strong := 0
	for _, s := range AnalyzeSamples(p) {
		if !hasGroup(s.Groups, GroupBoundary) {
			continue
		}
		if s.CodeDelta >= 8 {
			strong++
		}
	}
	if strong < 2 {
		t.Fatalf("expected at least 2 boundary-near samples with >=8 code delta, got %d", strong)
	}

	st := SweepCube(p, 21).Regions[RegionBoundaryBit]
	if st.MaxCodeDelta < 20 {
		t.Fatalf("boundary region should contain strong order effects, max %.2f", st.MaxCodeDelta)
	}
}

func TestAverageCannotMaskOrderDifference(t *testing.T) {
	p := NewPipeline(33)

	for _, s := range AnalyzeSamples(p) {
		if s.CodeDelta < 8 {
			continue
		}
		avg := Average(s.AB, s.BA)

		if DistRGB(avg, s.AB) < 1e-12 || DistRGB(avg, s.BA) < 1e-12 {
			t.Fatalf("sample %s: average must differ from both orders", s.ID)
		}
		wantHalf := s.CodeDelta / 2
		if math.Abs(s.AvgRefCodeErr-wantHalf) > 1e-6 {
			t.Fatalf("sample %s: avg reference error %.4f, want half delta %.4f",
				s.ID, s.AvgRefCodeErr, wantHalf)
		}
		const strictToleranceCodes = 1.0
		if s.AvgRefCodeErr <= strictToleranceCodes {
			t.Fatalf("sample %s: averaged result unexpectedly within ±1 code of A->B", s.ID)
		}
	}

	a := SweepCube(p, 21)
	if a.All.AvgRefOver1 < 100 {
		t.Fatalf("averaging should violate ±1 code vs A->B on many points, got %d", a.All.AvgRefOver1)
	}
	if a.All.MaxAvgRefCode < 10 {
		t.Fatalf("worst averaging error vs A->B too small: %.2f codes", a.All.MaxAvgRefCode)
	}
}

func TestOutputsStayInGamut(t *testing.T) {
	p := NewPipeline(17)
	for r := 0; r < 17; r++ {
		for g := 0; g < 17; g++ {
			for b := 0; b < 17; b++ {
				c := RGB{float64(r) / 16, float64(g) / 16, float64(b) / 16}
				for name, out := range map[string]RGB{"AB": p.AB(c), "BA": p.BA(c)} {
					for k, v := range out {
						if v < 0 || v > 1 {
							t.Fatalf("%s(%v) channel %d out of gamut: %v", name, c, k, out)
						}
					}
				}
			}
		}
	}
}

func TestSampleClassificationMatchesGroups(t *testing.T) {
	for _, s := range DefaultSamples() {
		regions := Classify(s.Color)
		for _, group := range s.Groups {
			var bit Region
			switch group {
			case GroupNeutral:
				bit = RegionNeutralBit
			case GroupSaturated:
				bit = RegionSaturatedBit
			case GroupBoundary:
				bit = RegionBoundaryBit
			default:
				t.Fatalf("sample %s has unknown group %q", s.ID, group)
			}
			if regions&bit == 0 {
				t.Fatalf("sample %s %v labeled %q but Classify=%b", s.ID, s.Color, group, regions)
			}
		}
	}
}

func hasGroup(groups []string, want string) bool {
	for _, g := range groups {
		if g == want {
			return true
		}
	}
	return false
}
