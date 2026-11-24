package dsp

import "math"

// Cepstrum computes the real (power) cepstrum of x:
// c[q] = IFFT(log(|FFT(x)|^2 + eps))[q], q in samples (quefrency).
// The returned slice has length n (power of two).
func Cepstrum(x []float64) (ceps []float64, n int) {
	n = NextPow2(len(x))
	re := make([]float64, n)
	im := make([]float64, n)
	copy(re, x)
	FFT(re, im, false)
	eps := 1e-12
	for k := 0; k < n; k++ {
		power := re[k]*re[k] + im[k]*im[k]
		re[k] = math.Log(power + eps)
		im[k] = 0
	}
	IFFT(re, im)
	return re, n
}

// Peak is a local maximum in a one-dimensional series.
type Peak struct {
	Index      int
	Value      float64
	Prominence float64
}

// LocalPeaks returns peaks within [lo,hi] whose value exceeds the local
// moving baseline by minProminence. The baseline is the minimum value in a
// window of radius context around the peak.
func LocalPeaks(x []float64, lo, hi, context int, minProminence float64) []Peak {
	if lo < 1 {
		lo = 1
	}
	if hi >= len(x) {
		hi = len(x) - 1
	}
	var out []Peak
	for i := lo; i < hi; i++ {
		if x[i] <= x[i-1] || x[i] < x[i+1] {
			continue
		}
		a, b := i-context, i+context
		if a < lo {
			a = lo
		}
		if b > hi {
			b = hi
		}
		base := x[i]
		for j := a; j <= b; j++ {
			if x[j] < base {
				base = x[j]
			}
		}
		prom := x[i] - base
		if prom >= minProminence {
			out = append(out, Peak{Index: i, Value: x[i], Prominence: prom})
		}
	}
	return out
}

// CepstralPeaks runs windowed cepstra, aggregates quefrency peaks across
// windows and returns a score (summed prominence) per quefrency lag.
// minLag/maxLag bound the plausible watermark/echo spacing in samples.
func CepstralPeaks(x []float64, winLen, hop, minLag, maxLag, context int, minProminence float64) (map[int]float64, int) {
	w := Hann(winLen)
	frames := Frame(x, winLen, hop, w)
	starts := FrameStarts(len(x), winLen, hop)
	scores := map[int]float64{}
	used := 0
	for fi, fr := range frames {
		ceps, n := Cepstrum(fr)
		_ = n
		hi := maxLag
		if hi >= len(ceps)/2 {
			hi = len(ceps)/2 - 1
		}
		peaks := LocalPeaks(ceps, minLag, hi, context, minProminence)
		if len(peaks) == 0 {
			continue
		}
		used++
		for _, p := range peaks {
			scores[p.Index] += p.Prominence
		}
		_ = starts[fi]
	}
	return scores, used
}

// RahmonicScore measures how strongly lag is accompanied by integer
// multiples (2*lag, 3*lag, ...) in the windowed cepstra. Echo and periodic
// watermark spacing both produce rahmonics; isolated codec ringing does not.
// Returns mean over windows of the ratio (sum of harmonic-peak heights) /
// (fundamental peak height), in [0, ~harmonics].
func RahmonicScore(x []float64, winLen, hop, lag, harmonics int) float64 {
	w := Hann(winLen)
	frames := Frame(x, winLen, hop, w)
	total, count := 0.0, 0
	for _, fr := range frames {
		ceps, n := Cepstrum(fr)
		if lag >= len(ceps)/2 {
			continue
		}
		base := ceps[lag]
		hsum := 0.0
		ok := true
		for h := 2; h <= harmonics; h++ {
			q := lag * h
			if q >= len(ceps)/2 || q >= n/2 {
				ok = false
				break
			}
			hsum += ceps[q]
		}
		if !ok || base <= 0 {
			continue
		}
		total += hsum / base
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}
