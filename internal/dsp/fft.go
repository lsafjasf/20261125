package dsp

import "math"
import "math/bits"

// FFT is an in-place iterative radix-2 Cooley-Tukey transform.
// len(re) must be a power of two. inverse=false computes the forward
// transform; inverse=true computes the unnormalised inverse (call IFFT
// instead, which applies the 1/N scaling).
func FFT(re, im []float64, inverse bool) {
	n := len(re)
	if n == 0 || n&(n-1) != 0 || len(im) != n {
		panic("FFT: length must be a non-zero power of two")
	}
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		half := length >> 1
		ang := 2 * math.Pi / float64(length)
		if inverse {
			ang = -ang
		}
		wr, wi := math.Cos(ang), math.Sin(ang)
		for start := 0; start < n; start += length {
			cr, ci := 1.0, 0.0
			for k := 0; k < half; k++ {
				i0, i1 := start+k, start+k+half
				tr := cr*re[i1] - ci*im[i1]
				ti := cr*im[i1] + ci*re[i1]
				re[i1] = re[i0] - tr
				im[i1] = im[i0] - ti
				re[i0] += tr
				im[i0] += ti
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
	if inverse {
		inv := 1.0 / float64(n)
		for i := range re {
			re[i] *= inv
			im[i] *= inv
		}
	}
}

// IFFT computes the scaled inverse transform in place.
func IFFT(re, im []float64) { FFT(re, im, true) }

// NextPow2 returns the smallest power of two >= n.
func NextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	return 1 << bits.Len(uint(n-1))
}

// Spectrum returns magnitude spectrum |X(k)| for k=0..n/2.
func Spectrum(x []float64) (freq, mag, re, im []float64) {
	n := NextPow2(len(x))
	re = make([]float64, n)
	im = make([]float64, n)
	copy(re, x)
	FFT(re, im, false)
	nbins := n/2 + 1
	mag = make([]float64, nbins)
	freq = make([]float64, nbins)
	for k := 0; k < nbins; k++ {
		mag[k] = math.Hypot(re[k], im[k])
		freq[k] = float64(k)
	}
	return freq, mag, re, im
}
