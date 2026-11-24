package dsp

import "math"

// Band describes a candidate frequency band in Hz.
type Band struct {
	Name string
	Lo   float64
	Hi   float64
}

// DefaultBands spans the speech-relevant range in overlapping log steps.
// These are the "candidate bands" inspected for watermark energy.
func DefaultBands() []Band {
	return []Band{
		{"B1", 80, 250},
		{"B2", 200, 600},
		{"B3", 500, 1400},
		{"B4", 1200, 3200},
		{"B5", 2800, 5600},
		{"B6", 5000, 9000},
	}
}

// Bandpass isolates one band using a brickwall frequency-domain mask on a
// single FFT of the whole signal. This is an analysis filter for evidence
// isolation, not a denoiser: no spectral subtraction, thresholding or
// smoothing is applied, and the original signal stays untouched.
func Bandpass(x []float64, sampleRate int, lo, hi float64) []float64 {
	n := NextPow2(len(x))
	re := make([]float64, n)
	im := make([]float64, n)
	copy(re, x)
	FFT(re, im, false)
	binHz := float64(sampleRate) / float64(n)
	for k := 0; k <= n/2; k++ {
		f := float64(k) * binHz
		keep := f >= lo && f <= hi
		if k == 0 || k == n/2 {
			keep = false
		}
		if !keep {
			re[k], im[k] = 0, 0
			if k != 0 && k != n/2 {
				re[n-k], im[n-k] = 0, 0
			}
		}
	}
	IFFT(re, im)
	return re[:len(x)]
}

// BandEnergy returns the per-frame mean-square energy of x after band
// isolation, evaluated at the given frame start offsets.
func BandEnergy(x []float64, sampleRate int, band Band, frameLen, hop int) ([]float64, []int) {
	y := Bandpass(x, sampleRate, band.Lo, band.Hi)
	starts := FrameStarts(len(y), frameLen, hop)
	e := make([]float64, len(starts))
	for f, s := range starts {
		end := s + frameLen
		if end > len(y) {
			end = len(y)
		}
		sum := 0.0
		for _, v := range y[s:end] {
			sum += v * v
		}
		e[f] = sum / float64(end-s)
	}
	return e, starts
}

// RMS returns the root-mean-square level of x[lo:hi].
func RMS(x []float64, lo, hi int) float64 {
	if hi > len(x) {
		hi = len(x)
	}
	if lo < 0 {
		lo = 0
	}
	if hi <= lo {
		return 0
	}
	sum := 0.0
	for _, v := range x[lo:hi] {
		sum += v * v
	}
	return math.Sqrt(sum / float64(hi-lo))
}

// FrameEnergy computes per-frame mean-square energy directly on the raw
// signal at the given frame geometry, without any filtering.
func FrameEnergy(x []float64, frameLen, hop int) ([]float64, []int) {
	starts := FrameStarts(len(x), frameLen, hop)
	e := make([]float64, len(starts))
	for f, s := range starts {
		end := s + frameLen
		if end > len(x) {
			end = len(x)
		}
		sum := 0.0
		for _, v := range x[s:end] {
			sum += v * v
		}
		e[f] = sum / float64(end-s)
	}
	return e, starts
}
