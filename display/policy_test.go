package display

import "testing"

func srgbGamut(t *testing.T) Primaries {
	t.Helper()
	g, ok := StandardGamut("sRGB")
	if !ok {
		t.Fatal("sRGB gamut preset missing")
	}
	return g
}

func validDisplay(id string, peak, black float64, bits int, gamutName string) Display {
	g, _ := StandardGamut(gamutName)
	return Display{ID: id, PeakNits: peak, BlackNits: black, Gamut: g, Bits: bits}
}

func TestMatrixSelectsExpectedTier(t *testing.T) {
	src := srgbGamut(t)
	cases := []struct {
		name string
		d    Display
		want TierID
	}{
		{"hdr oled", validDisplay("hdr", 1000, 0.05, 10, "P3-D65"), TierHDRPQ},
		{"sdr laptop", validDisplay("sdr", 300, 0.3, 8, "sRGB"), TierSDRDevice},
		{"weak but valid panel", validDisplay("weak", 120, 1.0, 8, "sRGB"), TierSDRDevice},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec := Resolve(tc.d, src)
			if !dec.ConfigValid {
				t.Fatalf("expected valid config, got errors %v", dec.ValidationErrors)
			}
			if dec.Selected != tc.want {
				t.Fatalf("selected %s, want %s; attempts=%+v", dec.Selected, tc.want, dec.Attempts)
			}
			if err := dec.Profile.Verify(); err != nil {
				t.Fatalf("selected profile failed verify: %v", err)
			}
			if dec.Fallback {
				t.Fatalf("valid rows must not be flagged as fallback: %v", dec.FallbackReasons)
			}
		})
	}
}

func TestInvalidConfigFallsBackToSafeTier(t *testing.T) {
	src := srgbGamut(t)
	bad := []Display{
		{ID: "negative-peak", PeakNits: -50, BlackNits: 0.1, Gamut: src, Bits: 10},
		{ID: "incomplete-gamut", PeakNits: 800, BlackNits: 0.05, Bits: 10}, // zero-valued primaries
		{ID: "black-above-peak", PeakNits: 100, BlackNits: 120, Gamut: src, Bits: 8},
	}
	for _, d := range bad {
		t.Run(d.ID, func(t *testing.T) {
			dec := Resolve(d, src)
			if dec.ConfigValid {
				t.Fatal("expected invalid configuration")
			}
			if !dec.Fallback || dec.Selected != TierSDRSafe {
				t.Fatalf("expected fallback to %s, got %s (fallback=%v)", TierSDRSafe, dec.Selected, dec.Fallback)
			}
			if len(dec.FallbackReasons) == 0 {
				t.Fatal("fallback must record why it happened")
			}
			if err := dec.Profile.Verify(); err != nil {
				t.Fatalf("fallback profile must be verifiable: %v", err)
			}
			safe := SafeProfile()
			if dec.Profile != safe {
				t.Fatalf("fallback profile %+v != safe baseline %+v", dec.Profile, safe)
			}
		})
	}
}

func TestHDRBelowThresholdSkipsTierWithoutFallbackFlag(t *testing.T) {
	src := srgbGamut(t)
	d := validDisplay("dim-hdr", 180, 0.2, 10, "sRGB")
	dec := Resolve(d, src)
	if dec.Selected != TierSDRDevice {
		t.Fatalf("want %s, got %s", TierSDRDevice, dec.Selected)
	}
	if dec.Fallback {
		t.Fatalf("normal matrix resolution is not a fallback: %v", dec.FallbackReasons)
	}
	var sawSkip bool
	for _, a := range dec.Attempts {
		if a.Tier == TierHDRPQ && !a.Eligible && a.Reason != "" {
			sawSkip = true
		}
	}
	if !sawSkip {
		t.Fatalf("expected an ineligible HDR attempt with reason, got %+v", dec.Attempts)
	}
}

func TestSafeProfileSelfVerification(t *testing.T) {
	if err := SafeProfile().Verify(); err != nil {
		t.Fatalf("safe baseline must always verify: %v", err)
	}
}
