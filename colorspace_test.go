package main

import (
	"math"
	"testing"
)

func approxEqual(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestResolveSpaceAliases(t *testing.T) {
	cases := map[string]string{
		"sRGB":          "sRGB",
		"srgb":          "sRGB",
		"Linear sRGB":   "Linear sRGB",
		"linear":        "Linear sRGB",
		"BT.709":        "BT.709",
		"rec709":        "BT.709",
		"Rec.709":       "BT.709",
		"display-p3":    "Display P3",
		"Linear BT.709": "Linear BT.709",
	}
	for in, wantLabel := range cases {
		s, err := ResolveSpace(in)
		if err != nil {
			t.Fatalf("ResolveSpace(%q): %v", in, err)
		}
		if s.Label != wantLabel {
			t.Fatalf("ResolveSpace(%q)=%q, want %q", in, s.Label, wantLabel)
		}
	}
	if _, err := ResolveSpace("ACES2065-1"); err == nil {
		t.Fatal("unknown space should error")
	}
}

func TestSRGBTransferRoundTrip(t *testing.T) {
	for _, v := range []float64{0, 0.02, 0.25, 0.5, 0.9, 1} {
		lin := srgbDecode(v)
		back := srgbEncode(lin)
		if !approxEqual(back, v, 1e-9) {
			t.Fatalf("sRGB round trip %v -> %v -> %v", v, lin, back)
		}
	}
	// 0.5 sRGB code value maps to roughly 0.214 linear.
	if got := srgbDecode(0.5); !approxEqual(got, 0.21404, 1e-4) {
		t.Fatalf("srgbDecode(0.5)=%v", got)
	}
}

func TestPrimariesMatrixMatchesStandardSRGB(t *testing.T) {
	red := spaces["srgb"].toXYZ.mul([3]float64{1, 0, 0})
	want := [3]float64{0.41239, 0.21264, 0.01933}
	for i := 0; i < 3; i++ {
		if !approxEqual(red[i], want[i], 5e-4) {
			t.Fatalf("sRGB red XYZ[%d]=%.5f want %.5f (full matrix %v)", i, red[i], want[i], spaces["srgb"].toXYZ)
		}
	}
	white := spaces["srgb"].toXYZ.mul([3]float64{1, 1, 1})
	if !approxEqual(white[0], 0.95047, 1e-3) || !approxEqual(white[1], 1.0, 1e-6) {
		t.Fatalf("D65 white XYZ unexpected: %v", white)
	}
}

func TestConvertRGBRoundTripAndIdentity(t *testing.T) {
	in := RGB{0.2, 0.55, 0.9}
	out, err := ConvertRGB("sRGB", "Linear sRGB", in)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ConvertRGB("Linear sRGB", "sRGB", out)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if !approxEqual(back[i], in[i], 1e-9) {
			t.Fatalf("channel %d round trip %v -> %v", i, in, back)
		}
	}
	// Same-primary spaces: sRGB -> Linear BT.709 gives identical linear values.
	lin, err := ConvertRGB("sRGB", "Linear BT.709", in)
	if err != nil {
		t.Fatal(err)
	}
	want := RGB{srgbDecode(in[0]), srgbDecode(in[1]), srgbDecode(in[2])}
	for i := 0; i < 3; i++ {
		if !approxEqual(lin[i], want[i], 1e-9) {
			t.Fatalf("709 linear[%d]=%v want %v", i, lin[i], want[i])
		}
	}
}

func TestP3ConversionChangesSaturation(t *testing.T) {
	// Converting encoded sRGB red into encoded Display P3 should pull the
	// green/blue channels up (the same XYZ red is a less saturated P3 code).
	p3red, err := ConvertRGB("sRGB", "Display P3", RGB{1, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if p3red[1] <= 0.05 || p3red[2] <= 0.05 {
		t.Fatalf("expected desaturated P3 red code, got %v", p3red)
	}
	back, err := ConvertRGB("Display P3", "sRGB", p3red)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(back[0], 1, 1e-6) || back[1] > 1e-6 || back[2] > 1e-6 {
		t.Fatalf("P3->sRGB round trip mismatch: %v", back)
	}
}
