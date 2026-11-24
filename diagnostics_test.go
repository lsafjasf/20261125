package sdrquant

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func steppedRamp(t *testing.T) *FloatImage {
	t.Helper()
	im, err := GenerateDarkGradient(SyntheticConfig{
		Width: 180, Height: 90,
		Min: 0.005, Max: 0.055,
		Texture: 0,
		Step:    0.004, StepAt: 0.70,
	})
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func TestClassificationSeparatesSourceEdgesFromSyntheticContours(t *testing.T) {
	src := steppedRamp(t)
	res, err := Quantize(src, config(DitherNone, ShapingNone))
	if err != nil {
		t.Fatal(err)
	}
	diag, err := Analyze(res, DiagnosticOptions{SpectrumSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	m := diag.Metrics
	if m.Distinction.SourceEdges == 0 || m.Classification.SourceEdgePixels == 0 {
		t.Fatalf("real source step was not detected: %+v", m.Classification)
	}
	if m.Distinction.BaselineSyntheticEdges == 0 || m.Classification.QuantContourPixels == 0 {
		t.Fatalf("quantization-only contours were not detected: %+v", m.Classification)
	}
	if m.Distinction.DitherAlteredPixels != 0 {
		t.Fatalf("no-dither pipeline altered %d pixels", m.Distinction.DitherAlteredPixels)
	}
	hasSource, hasContour := false, false
	for _, class := range diag.Classes {
		hasSource = hasSource || class == ClassSourceEdge
		hasContour = hasContour || class == ClassQuantContour
	}
	if !hasSource || !hasContour {
		t.Fatalf("classification map missing categories: source=%v contour=%v", hasSource, hasContour)
	}
}

func TestDitherNoiseIsNotReportedAsSourceBand(t *testing.T) {
	src := steppedRamp(t)
	res, err := Quantize(src, config(DitherTPDF, ShapingFloydSteinberg))
	if err != nil {
		t.Fatal(err)
	}
	diag, err := Analyze(res, DiagnosticOptions{SpectrumSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	m := diag.Metrics
	if m.Classification.DitherNoisePixels == 0 {
		t.Fatal("dither/shaping noise not classified")
	}
	if m.Distinction.DitherAlteredPixels == 0 {
		t.Fatal("output was not altered by dither/shaping")
	}
	if m.Distinction.BaselineSyntheticEdges == 0 {
		t.Fatal("baseline contour count should remain available for comparison")
	}
	if m.Distinction.SourceEdges == 0 {
		t.Fatal("genuine source edge should survive classification")
	}
}

func TestSpectrumAndMapsHaveExpectedGeometry(t *testing.T) {
	src := rampImage(100, 70, 0.01, 40.0/255)
	res, err := Quantize(src, config(DitherTPDF, ShapingFloydSteinberg))
	if err != nil {
		t.Fatal(err)
	}
	diag, err := Analyze(res, DiagnosticOptions{SpectrumSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	if diag.SpectrumSize != 64 || len(diag.SpectrumDB) != 64*64 {
		t.Fatalf("spectrum size=%d grid=%d", diag.SpectrumSize, len(diag.SpectrumDB))
	}
	if diag.ResidualMap.Width != 100 || diag.ResidualMap.Height != 70 {
		t.Fatal("residual map has wrong size")
	}
	if diag.DiffusionMap.Width != 100 || diag.DiffusionMap.Height != 70 {
		t.Fatal("diffusion map has wrong size")
	}
	if diag.Metrics.Spectrum.HighFrequencyRatio <= 0.5 {
		t.Fatalf("expected shaped high-frequency noise, ratio=%.3f", diag.Metrics.Spectrum.HighFrequencyRatio)
	}
}

func TestWriteDiagnosticsCreatesAllOutputs(t *testing.T) {
	src := steppedRamp(t)
	res, err := Quantize(src, config(DitherTPDF, ShapingFloydSteinberg))
	if err != nil {
		t.Fatal(err)
	}
	diag, err := Analyze(res, DiagnosticOptions{SpectrumSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := WriteDiagnostics(dir, diag); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"residual.png", "spectrum.png", "diffusion.png",
		"classification.png", "metrics.json", "spectrum.csv",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	metricsFile, err := os.Open(filepath.Join(dir, "metrics.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer metricsFile.Close()
	var decoded Metrics
	if err := json.NewDecoder(metricsFile).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Width != src.Width || decoded.Mode.Dither != DitherTPDF {
		t.Fatalf("unexpected metrics: %+v", decoded)
	}
	mapFile, err := os.Open(filepath.Join(dir, "residual.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer mapFile.Close()
	img, err := png.Decode(mapFile)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != src.Width || img.Bounds().Dy() != src.Height {
		t.Fatal("residual PNG dimensions changed")
	}
}

func TestFFTImpulseHasFlatSpectrum(t *testing.T) {
	a := make([]complex128, 16)
	a[0] = 1
	fft1(a)
	for i, value := range a {
		if real(value) < 0.999 || real(value) > 1.001 {
			t.Fatalf("bin %d = %v", i, value)
		}
	}
}
