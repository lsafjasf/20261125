package sdrquant

import "math"

func nextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// fft1 is an in-place radix-2 Cooley-Tukey FFT. Input length must be a power
// of two. The inverse is not needed by the diagnostic path.
func fft1(a []complex128) {
	n := len(a)
	if n <= 1 {
		return
	}
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		wLength := complex(math.Cos(angle), math.Sin(angle))
		half := length >> 1
		for start := 0; start < n; start += length {
			w := complex(1, 0)
			for k := 0; k < half; k++ {
				u := a[start+k]
				v := a[start+k+half] * w
				a[start+k] = u + v
				a[start+k+half] = u - v
				w *= wLength
			}
		}
	}
}

func fft2(a []complex128, size int) {
	if size <= 1 || len(a) != size*size {
		return
	}
	for y := 0; y < size; y++ {
		fft1(a[y*size : (y+1)*size])
	}
	column := make([]complex128, size)
	for x := 0; x < size; x++ {
		for y := 0; y < size; y++ {
			column[y] = a[y*size+x]
		}
		fft1(column)
		for y := 0; y < size; y++ {
			a[y*size+x] = column[y]
		}
	}
}

func shiftedIndex(x, y, size int) int {
	return ((y+size/2)%size)*size + ((x + size/2) % size)
}
