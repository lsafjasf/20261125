package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"

	"watermarkaudit/internal/dsp"
)

func fftMask(v []float64, lo, hi float64) []float64 {
	n := dsp.NextPow2(len(v))
	re := make([]float64, n)
	im := make([]float64, n)
	copy(re, v)
	dsp.FFT(re, im, false)
	binHz := float64(sr) / float64(n)
	for k := 1; k < n/2; k++ {
		f := float64(k) * binHz
		if f < lo || f > hi {
			re[k], im[k] = 0, 0
			re[n-k], im[n-k] = 0, 0
		}
	}
	dsp.IFFT(re, im)
	return re[:len(v)]
}

// codecRinging mimics transform-codec artefacts: every block is FFT coded
// with only the lowest fraction of bins retained, and a short damped tone
// rings from each block boundary. Both artefacts are strictly block-locked.
func codecRinging(x []float64, block int, rng *rand.Rand) []float64 {
	out := make([]float64, len(x))
	for base := 0; base < len(x); base += block {
		end := base + block
		if end > len(x) {
			end = len(x)
		}
		seg := make([]float64, block)
		copy(seg, x[base:end])
		w := dsp.Hann(block)
		for i := range seg {
			seg[i] *= w[i]
		}
		re := make([]float64, block)
		im := make([]float64, block)
		copy(re, seg)
		dsp.FFT(re, im, false)
		keep := block / 8 // coarse quantisation -> spectral truncation
		for k := keep; k < block-keep; k++ {
			re[k], im[k] = 0, 0
		}
		dsp.IFFT(re, im)
		for i := base; i < end; i++ {
			out[i] += re[i-base] * 0.35
		}
		// Damped boundary tone: 3.7 kHz, one block of decay. This is the
		// pre/post-ring transient a transform codec leaves on each block.
		phase := rng.Float64() * 2 * math.Pi
		for i := 0; i < block && base+i < len(x); i++ {
			d := math.Exp(-9 * float64(i) / float64(block))
			out[base+i] += 0.28 * d * math.Sin(2*math.Pi*3700*float64(i)/sr+phase)
		}
	}
	// Mix artefacts at moderate strength with the untouched original.
	for i := range x {
		out[i] = 0.55*x[i] + out[i]
	}
	return out
}

func dirOf(p string) string {
	d := filepath.Dir(p)
	if d == "." {
		return "."
	}
	return d
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "wmsynth:", err)
	os.Exit(1)
}
