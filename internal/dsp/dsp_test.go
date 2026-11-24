package dsp

import (
	"math"
	"testing"
)

func TestFFTSine(t *testing.T) {
	n := 64
	re := make([]float64, n)
	im := make([]float64, n)
	for i := range re {
		re[i] = math.Sin(2 * math.Pi * 4 * float64(i) / float64(n))
	}
	FFT(re, im, false)
	if math.Abs(im[4]-float64(n)/2) > 1e-9 {
		t.Fatalf("expected spectral peak at bin 4 with magnitude n/2, got %.4f", im[4])
	}
	IFFT(re, im)
	for i := 0; i < n; i++ {
		want := math.Sin(2 * math.Pi * 4 * float64(i) / float64(n))
		if math.Abs(re[i]-want) > 1e-9 || math.Abs(im[i]) > 1e-9 {
			t.Fatalf("roundtrip mismatch at %d: got %.6f want %.6f", i, re[i], want)
		}
	}
}

func TestNCCKnown(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	y := []float64{2, 4, 6, 8, 10, 12, 14, 16}
	if r := NCC(x, y, 0, 0, 8); math.Abs(r-1) > 1e-12 {
		t.Fatalf("perfect linear relation NCC = %.4f, want 1", r)
	}
	neg := []float64{8, 7, 6, 5, 4, 3, 2, 1}
	if r := NCC(x, neg, 0, 0, 8); math.Abs(r+1) > 1e-12 {
		t.Fatalf("inverse relation NCC = %.4f, want -1", r)
	}
	if r := NCC(x, x, 0, 0, 8); math.Abs(r-1) > 1e-12 {
		t.Fatalf("self NCC = %.4f, want 1", r)
	}
}

func TestSlidingNCCDetectsCopy(t *testing.T) {
	x := make([]float64, 4096)
	seed := uint64(12345)
	for i := range x {
		// Deterministic LCG white noise gives a unique correlation peak.
		seed = seed*6364136223846793005 + 1442695040888963407
		x[i] = float64(seed%10000)/10000 - 0.5
	}
	mean := 0.0
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	for i := range x {
		x[i] -= mean
	}
	lag := 2048 // >= window length so the two windows never overlap
	copy(x[lag:lag+1024], x[0:1024])
	// At the exact copied position the NCC must be (numerically) one.
	if r := NCC(x, x, 0, lag, 1024); r < 0.9999 {
		t.Fatalf("exact-copy NCC %.6f, want >=0.9999", r)
	}
	pos, r := SlidingNCC(x, lag, 1024, 256)
	if len(pos) == 0 || pos[0] != 0 || r[0] < 0.9999 {
		t.Fatalf("sliding map did not locate the copy at t=0: pos=%v r0=%v", pos, r)
	}
}

func TestCepstrumPeakPeriodic(t *testing.T) {
	n := 16384
	x := make([]float64, n)
	T := 300
	for k := 0; k*T < n; k++ {
		for j := 0; j < 60 && k*T+j < n; j++ {
			x[k*T+j] = math.Exp(-float64(j) / 12)
		}
	}
	scores, used := CepstralPeaks(x, 4096, 2048, 100, 2000, 8, 1e-9)
	if used == 0 {
		t.Fatal("no cepstral windows produced peaks")
	}
	if scores[T] == 0 {
		t.Fatalf("no cepstral peak at true quefrency %d; got %v", T, scores)
	}
}

func TestHannAndFrame(t *testing.T) {
	w := Hann(8)
	if w[0] != 0 || w[7] != 0 {
		t.Fatalf("Hann endpoints must be zero: %v", w)
	}
	if math.Abs(w[3]-w[4]) > 1e-12 || w[3] <= 0.9 {
		t.Fatalf("Hann central maxima wrong: %v", w)
	}
	x := make([]float64, 1000)
	frames := Frame(x, 256, 128, Hann(256))
	starts := FrameStarts(len(x), 256, 128)
	if len(frames) != len(starts) {
		t.Fatalf("frame/start count mismatch: %d vs %d", len(frames), len(starts))
	}
	if starts[len(starts)-1] >= len(x) {
		t.Fatal("last frame start out of bounds")
	}
}

func TestBandpassAttenuatesOutOfBand(t *testing.T) {
	n := 8192
	inBand := make([]float64, n)
	outBand := make([]float64, n)
	for i := 0; i < n; i++ {
		inBand[i] = math.Sin(2 * math.Pi * 200 * float64(i) / 8000)
		outBand[i] = math.Sin(2 * math.Pi * 6000 * float64(i) / 8000)
	}
	kept := RMS(Bandpass(inBand, 8000, 100, 1000), 0, n)
	suppressed := RMS(Bandpass(outBand, 8000, 100, 1000), 0, n)
	if kept < 0.6 {
		t.Fatalf("in-band tone over-attenuated: %.4f", kept)
	}
	if suppressed > 0.05 {
		t.Fatalf("out-of-band tone not suppressed: %.4f", suppressed)
	}
}
