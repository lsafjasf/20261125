package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func approx(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %.9g, want %.9g ± %.3g", name, got, want, tolerance)
	}
}

func TestInverseOETFRoundTrip(t *testing.T) {
	for _, scene := range []float64{0, 1.0 / 24, 1.0 / 12, 0.25, 0.75, 1} {
		code := OETF(scene)
		got := InverseOETF(code)
		approx(t, "scene round-trip", got, scene, 1e-12)
	}
}

func TestSystemGamma(t *testing.T) {
	got, err := SystemGamma(1000, 0, GammaStandard)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "gamma 1000", got, 1.2, 1e-12)

	got, err = SystemGamma(2000, 0, GammaStandard)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "gamma 2000", got, 1.326433, 1e-5)

	got, err = SystemGamma(4000, 0, GammaExtended)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "gamma 4000 extended", got, 1.481185, 1e-5)

	got, err = SystemGamma(4000, 1.234, GammaAuto)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "manual gamma wins", got, 1.234, 1e-12)
}

func TestGrayEndpointIsDisplayPeak(t *testing.T) {
	for _, peak := range []float64{1000, 2000, 4000} {
		cfg := DefaultConfig(peak)
		cfg.GammaLaw = GammaStandard
		sample := cfg.DecodeGray(1)
		approx(t, "E'=1 raw Y", sample.RawLumaNits, peak, peak*1e-5)
		if sample.OverPeak {
			t.Fatalf("peak endpoint should not be marked over peak: %.9f", sample.RawLumaNits)
		}
	}
}

func TestBetaLiftsCodeZeroToBlackNits(t *testing.T) {
	cfg := DefaultConfig(1000)
	cfg.BlackNits = 0.01
	sample := cfg.DecodeGray(0)
	approx(t, "black luminance", sample.RawLumaNits, 0.01, 1e-8)
}

func TestWrongAlphaIsSmoothButAbsoluteValueWrong(t *testing.T) {
	reference := DefaultConfig(1000)
	wrong := DefaultConfig(4000)
	wrong.GammaLaw = GammaManual
	wrong.ManualGamma = 1.2
	wrong.AlphaNits = 1000

	for _, code := range []float64{0.25, 0.5, 0.75, 1} {
		a := reference.DecodeGray(code)
		b := wrong.DecodeGray(code)
		approx(t, "normalized wrong/reference", b.RawLumaNits/b.AlphaNits,
			a.RawLumaNits/a.AlphaNits, 1e-12)
		approx(t, "absolute wrong/reference", b.RawLumaNits, a.RawLumaNits, 1e-6)
	}

	end := wrong.DecodeGray(1)
	approx(t, "wrong endpoint", end.RawLumaNits, 1000, 1e-4)
	if math.Abs(end.RawLumaNits-4000) < 1 {
		t.Fatal("wrong alpha example unexpectedly reached physical peak")
	}
}

func TestParsePeaks(t *testing.T) {
	peaks, err := ParsePeaks("1000, 2000 ,4000")
	if err != nil {
		t.Fatal(err)
	}
	if len(peaks) != 3 || peaks[2] != 4000 {
		t.Fatalf("unexpected peaks: %#v", peaks)
	}
	if _, err := ParsePeaks("1000,nope"); err == nil {
		t.Fatal("expected invalid peak error")
	}
}

func TestChartsProduceWellFormedSVGFiles(t *testing.T) {
	dir := t.TempDir()
	configs := []Config{DefaultConfig(1000), DefaultConfig(2000), DefaultConfig(4000)}
	wrong := DefaultConfig(4000)
	wrong.GammaLaw = GammaManual
	wrong.ManualGamma = 1.2
	wrong.AlphaNits = 1000

	files := map[string]func(string) error{
		"gray.svg":    func(p string) error { return WriteGrayscaleChart(p, configs, wrong) },
		"color.svg":   func(p string) error { return WriteColorChart(p, configs) },
		"compare.svg": func(p string) error { return WritePeakComparisonChart(p, configs) },
	}
	for name, write := range files {
		path := filepath.Join(dir, name)
		if err := write(path); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.HasPrefix(text, `<?xml`) || !strings.HasSuffix(strings.TrimSpace(text), "</svg>") {
			t.Fatalf("%s is not a complete SVG", name)
		}
	}
}

func TestNitsToPreviewSRGBWhite(t *testing.T) {
	white := NitsToPreviewSRGB(RGB{1000, 1000, 1000}, 1000)
	approx(t, "white r", white.R, 1, 1e-12)
	approx(t, "white g", white.G, 1, 1e-12)
	approx(t, "white b", white.B, 1, 1e-12)
}
