package dsp

import "math"

// Hann returns a symmetric Hann window of length n.
// w[k] = 0.5 * (1 - cos(2*pi*k/(n-1)))
func Hann(n int) []float64 {
	if n <= 1 {
		return []float64{1}
	}
	w := make([]float64, n)
	denom := float64(n - 1)
	for k := range w {
		w[k] = 0.5 * (1 - math.Cos(2*math.Pi*float64(k)/denom))
	}
	return w
}

// Apply multiplies x by w in place; lengths must match.
func Apply(x, w []float64) []float64 {
	if len(x) != len(w) {
		panic("Apply: window length mismatch")
	}
	for i := range x {
		x[i] *= w[i]
	}
	return x
}

// Frame splits a signal into overlapping frames of frameLen with hop.
// The final partial frame is zero padded. All windows are applied here;
// no other preprocessing is performed on the raw samples.
func Frame(x []float64, frameLen, hop int, w []float64) [][]float64 {
	if hop <= 0 || frameLen <= 0 || len(w) != frameLen {
		panic("Frame: bad geometry")
	}
	nframes := 1 + (len(x)-1+frameLen)/hop
	if len(x) == 0 {
		return nil
	}
	out := make([][]float64, nframes)
	for f := 0; f < nframes; f++ {
		start := f * hop
		if start >= len(x) {
			out = out[:f]
			break
		}
		frame := make([]float64, frameLen)
		copied := copy(frame, x[start:])
		_ = copied
		for i := range frame {
			frame[i] *= w[i]
		}
		out[f] = frame
	}
	return out
}

// FrameStarts returns the sample offset of each frame produced by Frame.
func FrameStarts(nsamples, frameLen, hop int) []int {
	if nsamples == 0 || hop <= 0 {
		return nil
	}
	nframes := 1 + (nsamples-1)/hop
	starts := make([]int, nframes)
	for f := range starts {
		starts[f] = f * hop
	}
	return starts
}
