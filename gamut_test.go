package main

import (
	"math"
	"testing"
)

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestLabLChRoundTrip(t *testing.T) {
	cases := []Lab{
		{L: 50, A: 23.4, B: -41.2},
		{L: 12, A: -5, B: 70},
		{L: 90, A: 0.01, B: -0.02},
	}
	for _, l := range cases {
		back := lchToLab(labToLCh(l))
		if !approx(back.L, l.L, 1e-9) || !approx(back.A, l.A, 1e-9) || !approx(back.B, l.B, 1e-9) {
			t.Fatalf("LCh round trip mismatch: %+v -> %+v", l, back)
		}
	}
}

func TestSRGBXYZRoundTrip(t *testing.T) {
	g := newSRGBGamut()
	for _, rgb := range []LinRGB{{0.1, 0.7, 0.4}, {0.9, 0.05, 0.33}, {1, 1, 1}} {
		back := g.xyzToLinear(g.linearToXYZ(rgb))
		if !approx(back.R, rgb.R, 1e-10) || !approx(back.G, rgb.G, 1e-10) || !approx(back.B, rgb.B, 1e-10) {
			t.Fatalf("XYZ round trip mismatch: %+v -> %+v", rgb, back)
		}
	}
}

func TestNarrowGamutKeepsPaperWhite(t *testing.T) {
	g := newNarrowGamut("NARROW-D65", 0.4)
	white := g.xyzToLinear(XYZ{X: whiteX, Y: whiteY, Z: whiteZ})
	for _, v := range []float64{white.R, white.G, white.B} {
		if !approx(v, 1.0, 1e-9) {
			t.Fatalf("D65 no longer maps to (1,1,1): %+v", white)
		}
	}
	if !g.containsLCh(LCh{L: 100, C: 0, H: 0}) {
		t.Fatal("paper white must stay in-gamut")
	}
}

func TestMaxChromaBoundary(t *testing.T) {
	g := newSRGBGamut()
	// A point just inside the reported boundary must be in-gamut and a
	// point just outside must not.
	for _, h := range []float64{0, 40, 120, 200, 300} {
		bc := g.maxChroma(50, h)
		if bc <= 0 {
			t.Fatalf("non-positive boundary at hue %v", h)
		}
		if !g.containsLCh(LCh{L: 50, C: bc * 0.999, H: h}) {
			t.Fatalf("point inside boundary reported out at hue %v", h)
		}
		if g.containsLCh(LCh{L: 50, C: bc*1.01 + 0.01, H: h}) {
			t.Fatalf("point outside boundary reported in at hue %v", h)
		}
	}
}

func TestSoftCompressionPreservesHueAndFits(t *testing.T) {
	dst := newNarrowGamut("NARROW-D65", 0.4)
	redEnd := xyzToLab(srgbRGBToXYZ(LinRGB{R: 1, G: 0, B: 0}))
	for i := 0; i <= 100; i++ {
		q := float64(i) / 100
		orig := lerpLab(Lab{L: 100}, redEnd, q)
		mapped := dst.softCompressLab(orig)
		if labToLCh(orig).C > 0.05 && !approx(labToLCh(mapped).H, labToLCh(orig).H, 1e-9) {
			t.Fatalf("soft compression changed hue at q=%v", q)
		}
		if !dst.containsLCh(labToLCh(mapped)) {
			t.Fatalf("soft result out of gamut at q=%v", q)
		}
	}
}

func TestCompressChromaIsContinuousAtKnee(t *testing.T) {
	boundary := 80.0
	k := softKnee * boundary
	under := compressChroma(k-1e-9, boundary)
	over := compressChroma(k+1e-9, boundary)
	if !approx(under, over, 1e-6) {
		t.Fatalf("compression discontinuous at knee: %v vs %v", under, over)
	}
	if compressChroma(300, boundary) > boundary {
		t.Fatal("compression must never exceed boundary chroma")
	}
}

func TestCIEDE2000ZeroForIdentical(t *testing.T) {
	c := Lab{L: 55, A: 10, B: -20}
	if ciede2000(c, c) != 0 {
		t.Fatal("identical colors must have zero difference")
	}
	if ciede2000(c, Lab{L: 56, A: 10, B: -20}) <= 0 {
		t.Fatal("different colors must have positive difference")
	}
}

func TestHueDistance(t *testing.T) {
	cases := []struct{ a, b, want float64 }{
		{10, 20, 10},
		{350, 10, 20},
		{0, 180, 180},
		{40, 40, 0},
	}
	for _, tc := range cases {
		if got := hueDistance(tc.a, tc.b); !approx(got, tc.want, 1e-9) {
			t.Fatalf("hueDistance(%v,%v)=%v want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
