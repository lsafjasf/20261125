package sdrquant

import (
	"math"
	"testing"
)

func rampImage(width, height int, low, high float64) *FloatImage {
	im, _ := NewFloatImage(width, height, 1)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			u := float64(x) / float64(width-1)
			im.Set(x, y, 0, float32(low+(high-low)*u))
		}
	}
	return im
}

func config(dither DitherMode, shaping ShapingMode) Config {
	cfg := DefaultConfig()
	cfg.Dither = dither
	cfg.Shaping = shaping
	cfg.DitherStrength = 1
	return cfg
}

func TestNoDitherProducesLongFlatBands(t *testing.T) {
	src := rampImage(512, 24, 0.005, 20.0/255)
	res, err := Quantize(src, config(DitherNone, ShapingNone))
	if err != nil {
		t.Fatal(err)
	}
	transitions := 0
	for y := 0; y < src.Height; y++ {
		for x := 1; x < src.Width; x++ {
			a := res.Quant[y*src.Width+x-1]
			b := res.Quant[y*src.Width+x]
			if a != b {
				transitions++
			}
		}
	}
	if transitions == 0 {
		t.Fatal("expected quantization transitions")
	}
	if transitions > src.Height*40 {
		t.Fatalf("unexpectedly fragmented output: %d transitions", transitions)
	}
}

func TestTPDFAndShapingAlterLevelsAndRaiseHighFrequencyEnergy(t *testing.T) {
	src := rampImage(256, 96, 0.004, 48.0/255)
	baseRes, err := Quantize(src, config(DitherNone, ShapingNone))
	if err != nil {
		t.Fatal(err)
	}
	baseDiag, err := Analyze(baseRes, DiagnosticOptions{SpectrumSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Quantize(src, config(DitherTPDF, ShapingFloydSteinberg))
	if err != nil {
		t.Fatal(err)
	}
	diag, err := Analyze(res, DiagnosticOptions{SpectrumSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	if diag.Metrics.Distinction.DitherAlteredPixels == 0 {
		t.Fatal("expected dither/shaping to alter output levels")
	}
	if diag.Metrics.Feedback.RMSLSB <= 0 {
		t.Fatal("expected nonzero diffused feedback")
	}
	if diag.Metrics.Spectrum.HighFrequencyRatio <= baseDiag.Metrics.Spectrum.HighFrequencyRatio+0.2 {
		t.Fatalf("expected high-frequency noise: base=%.3f dithered=%.3f",
			baseDiag.Metrics.Spectrum.HighFrequencyRatio,
			diag.Metrics.Spectrum.HighFrequencyRatio)
	}
	if diag.Metrics.Classification.DitherNoisePixels == 0 {
		t.Fatal("expected dither/shaping noise classification")
	}
}

func TestShapingRedistributesConstantTone(t *testing.T) {
	im, _ := NewFloatImage(64, 64, 1)
	const value = 0.5 + 0.2/255
	for i := range im.Pix {
		im.Pix[i] = value
	}
	plain, err := Quantize(im, config(DitherNone, ShapingNone))
	if err != nil {
		t.Fatal(err)
	}
	shaped, err := Quantize(im, config(DitherNone, ShapingFloydSteinberg))
	if err != nil {
		t.Fatal(err)
	}
	plainLevels := map[uint8]bool{}
	shapedLevels := map[uint8]bool{}
	for i := range plain.Quant {
		plainLevels[plain.Quant[i]] = true
		shapedLevels[shaped.Quant[i]] = true
	}
	if len(plainLevels) != 1 {
		t.Fatalf("plain quantizer should be flat, got levels %v", plainLevels)
	}
	if len(shapedLevels) < 2 {
		t.Fatalf("error diffusion should redistribute a constant tone, got %v", shapedLevels)
	}
	if diag, _ := Analyze(shaped, DiagnosticOptions{SpectrumSize: 64}); diag.Metrics.Feedback.RMSLSB <= 0 {
		t.Fatal("expected feedback RMS")
	}
}

func TestDitherAndShapingTogglesAreIndependent(t *testing.T) {
	src := rampImage(80, 40, 0.01, 30.0/255)
	ditherOnly, err := Quantize(src, config(DitherTPDF, ShapingNone))
	if err != nil {
		t.Fatal(err)
	}
	ditherDiag, err := Analyze(ditherOnly, DiagnosticOptions{SpectrumSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	if ditherDiag.Metrics.DitherSignal.RMSLSB == 0 {
		t.Fatal("dither was enabled but no dither signal was recorded")
	}
	if ditherDiag.Metrics.Feedback.RMSLSB != 0 {
		t.Fatal("shaping is off but feedback is nonzero")
	}

	shapingOnly, err := Quantize(src, config(DitherNone, ShapingFloydSteinberg))
	if err != nil {
		t.Fatal(err)
	}
	shapingDiag, err := Analyze(shapingOnly, DiagnosticOptions{SpectrumSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	if shapingDiag.Metrics.DitherSignal.RMSLSB != 0 {
		t.Fatal("dither is off but dither signal is nonzero")
	}
	if shapingDiag.Metrics.Feedback.RMSLSB == 0 {
		t.Fatal("shaping is on but feedback is zero")
	}
	if shapingDiag.Metrics.Distinction.DitherAlteredPixels == 0 {
		t.Fatal("shaping did not alter any output level")
	}
}

func TestQuantizeIsDeterministicForSameSeed(t *testing.T) {
	src := rampImage(80, 40, 0.01, 30.0/255)
	cfg := config(DitherTPDF, ShapingFloydSteinberg)
	a, err := Quantize(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Quantize(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a.Quant {
		if a.Quant[i] != b.Quant[i] {
			t.Fatalf("same seed differs at %d", i)
		}
	}
	cfg.Seed++
	c, err := Quantize(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	different := false
	for i := range a.Quant {
		different = different || a.Quant[i] != c.Quant[i]
	}
	if !different {
		t.Fatal("different seeds produced identical output")
	}
}

func TestQuantizeClipsAndSanitizes(t *testing.T) {
	im, _ := NewFloatImage(4, 1, 1)
	im.Pix = []float32{-1, float32(math.NaN()), 1.5, 0.5}
	res, err := Quantize(im, config(DitherTPDF, ShapingFloydSteinberg))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range res.Quant {
		if q > 255 {
			t.Fatal("quantized value out of range")
		}
	}
	if res.ClippedPixels != 3 {
		t.Fatalf("clipped pixels = %d, want 3", res.ClippedPixels)
	}
}

func TestSRGBTransferChangesDarkCode(t *testing.T) {
	im, _ := NewFloatImage(1, 1, 1)
	im.Pix[0] = 0.01
	plain, err := Quantize(im, config(DitherNone, ShapingNone))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config(DitherNone, ShapingNone)
	cfg.Transfer = TransferSRGB
	encoded, err := Quantize(im, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if encoded.Quant[0] <= plain.Quant[0]+10 {
		t.Fatalf("sRGB transfer did not lift dark code: plain=%d srgb=%d", plain.Quant[0], encoded.Quant[0])
	}
}
