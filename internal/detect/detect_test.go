package detect

import (
	"math"
	"math/rand/v2"
	"testing"

	"watermarkaudit/internal/dsp"
	"watermarkaudit/internal/wavio"
)

func TestFisherRoundTrip(t *testing.T) {
	for _, r := range []float64{-0.9, -0.3, 0, 0.25, 0.77} {
		if got := InvFisherZ(FisherZ(r)); math.Abs(got-r) > 1e-12 {
			t.Fatalf("fisher roundtrip %.3f -> %.3f", r, got)
		}
	}
}

func TestMeanCICoversValue(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	vals := make([]float64, 200)
	for i := range vals {
		vals[i] = 0.7 + 0.05*rng.NormFloat64()
	}
	ci := MeanCIZ(vals, 1000, rng)
	if ci.Level != 0.95 || ci.Low >= ci.High {
		t.Fatalf("bad CI: %+v", ci)
	}
	if FisherZ(ci.Low) > FisherZ(0.7) || FisherZ(ci.High) < FisherZ(0.7) {
		t.Fatalf("CI %+v does not cover the population value 0.7", ci)
	}
}

func TestShuffledNullNearZero(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	x := make([]float64, 16384)
	for i := range x {
		x[i] = rng.NormFloat64()
	}
	null := ShuffledNull(x, 2000, 1024, 256, 300, rng)
	if math.Abs(null) > 0.15 {
		t.Fatalf("independent-window null NCC %.4f should be near zero", null)
	}
}

// TestEndToEndSynthetic runs the full pipeline on a controlled signal and
// checks the three labelled phenomena against their ground-truth geometry.
func TestEndToEndSynthetic(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 99))
	sr := 24000
	n := 10 * sr
	x := make([]float64, n)
	for i := range x {
		tm := float64(i) / float64(sr)
		x[i] = 0.3*math.Sin(2*math.Pi*700*tm) + 0.2*math.Sin(2*math.Pi*1500*tm)
	}
	// Watermark: identical 1024-sample band-limited fragment, 4 copies.
	wm := make([]float64, 1024)
	for i := range wm {
		wm[i] = rng.NormFloat64()
	}
	wm = dsp.Bandpass(wm, sr, 1200, 3200)
	truthStarts := []int{sr, 3 * sr, 5 * sr, 8 * sr}
	for _, p := range truthStarts {
		for k := range wm {
			x[p+k] += 0.8 * wm[k]
		}
	}
	// Echo: damped 3000-sample delay.
	for i := 3000; i < n; i++ {
		x[i] += 0.45 * x[i-3000]
	}
	w := &wavio.WAV{SampleRate: sr, Channels: 1, BitsPerSample: 16, Mono: x}
	cfg := DefaultConfig(sr)
	rep := Analyze(w, dsp.DefaultBands(), cfg)

	var hasWM, hasEcho, hasRing bool
	for _, f := range rep.Findings {
		switch f.Verdict {
		case VerdictWatermark:
			hasWM = true
			if f.Confidence.Low <= 0.45 {
				t.Fatalf("watermark CI low %.3f too weak", f.Confidence.Low)
			}
			if len(f.Occurrences) < 3 {
				t.Fatalf("watermark occurrences %d < 3", len(f.Occurrences))
			}
		case VerdictEcho:
			hasEcho = true
			if math.Abs(float64(f.Features.LagSamples-3000)) > 60 {
				t.Fatalf("echo lag %d not near 3000", f.Features.LagSamples)
			}
		case VerdictRinging:
			hasRing = true
		}
		if f.Counterexample.Kind == "" {
			t.Fatal("finding missing counterexample")
		}
		if f.Range.End <= f.Range.Start {
			t.Fatal("finding has empty sample range")
		}
	}
	if !hasWM {
		t.Fatal("watermark not detected")
	}
	if !hasEcho {
		t.Fatal("echo not detected")
	}
	if hasRing {
		t.Fatal("ringing claimed on a signal with no block artefacts")
	}
}
