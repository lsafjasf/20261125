package display

import (
	"math"
	"testing"
)

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestPQRoundTrip(t *testing.T) {
	for _, nits := range []float64{0.005, 0.1, 10, 100, 1000, 4000} {
		got := PQDecode(PQEncode(nits))
		if !approx(got, nits, nits*1e-4+1e-6) {
			t.Fatalf("PQ round trip at %g nits gave %g", nits, got)
		}
	}
}

func TestSRGBRoundTrip(t *testing.T) {
	for _, lin := range []float64{0.0, 0.001, 0.18, 0.5, 1.0} {
		got := SRGBDecode(SRGBEncode(lin))
		if !approx(got, lin, 1e-6) {
			t.Fatalf("sRGB round trip at %g gave %g", lin, got)
		}
	}
}

func TestToneMapStaysInsideRangeAndMonotonic(t *testing.T) {
	src := srgbGamut(t)
	profiles := []Profile{
		mustBuild(t, TierHDRPQ, validDisplay("hdr", 1000, 0.05, 10, "P3-D65"), src),
		mustBuild(t, TierSDRDevice, validDisplay("sdr", 300, 0.3, 8, "sRGB"), src),
	}
	for _, p := range profiles {
		peakRel := p.PeakNits / ReferenceWhiteNits
		prev := -1.0
		for v := 0.0; v <= 10.0; v += 0.05 {
			m := ToneMap(v, p)
			if m < 0 || m > peakRel*(1+1e-12) {
				t.Fatalf("%s: ToneMap(%g)=%g outside [0,%g]", p.Tier, v, m, peakRel)
			}
			if m+1e-12 < prev {
				t.Fatalf("%s: ToneMap not monotonic at %g (%g < %g)", p.Tier, v, m, prev)
			}
			prev = m
		}
		black := ToneMap(0, p)
		if !approx(black, p.BlackNits/ReferenceWhiteNits, 1e-12) {
			t.Fatalf("%s: black floor %g != %g", p.Tier, black, p.BlackNits/ReferenceWhiteNits)
		}
	}
}

func TestIdentityGamutMatrix(t *testing.T) {
	g := srgbGamut(t)
	m, ok := GamutMapMatrix(g, g)
	if !ok {
		t.Fatal("identity gamut matrix failed")
	}
	for i := range m {
		want := 0.0
		if i%4 == 0 {
			want = 1.0
		}
		if !approx(m[i], want, 1e-9) {
			t.Fatalf("identity matrix mismatch at %d: %g", i, m[i])
		}
	}
}

func TestSameMaterialProducesDifferentOutputs(t *testing.T) {
	src := srgbGamut(t)
	devices := []Display{
		validDisplay("hdr-oled", 1000, 0.05, 10, "P3-D65"),
		validDisplay("sdr-lcd", 300, 0.3, 8, "sRGB"),
		validDisplay("cheap-laptop", 180, 1.2, 6, "sRGB"),
	}
	material := [3]float64{4.0, 2.0, 0.5} // HDR highlight pixel, linear sRGB relative
	seen := map[[3]int]int{}
	tierSeen := map[TierID]bool{}
	for _, d := range devices {
		dec := Resolve(d, src)
		if err := dec.Profile.Verify(); err != nil {
			t.Fatalf("%s: %v", d.ID, err)
		}
		_, codes := ConvertPixel(material, dec.Profile)
		seen[codes]++
		tierSeen[dec.Selected] = true
	}
	if len(seen) != len(devices) {
		t.Fatalf("expected %d distinct code triples, got %d: %v", len(devices), len(seen), seen)
	}
	if !tierSeen[TierHDRPQ] || !tierSeen[TierSDRDevice] {
		t.Fatalf("expected both HDR and SDR tiers in use, got %v", tierSeen)
	}
}

func mustBuild(t *testing.T, tier TierID, d Display, src Primaries) Profile {
	t.Helper()
	p, err := buildProfile(tier, d, src)
	if err != nil {
		t.Fatalf("buildProfile %s: %v", tier, err)
	}
	return p
}
