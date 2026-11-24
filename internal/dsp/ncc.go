package dsp

import "math"

// NCC returns the normalised cross-correlation (Pearson r in [-1,1])
// between x[a:a+len] and y[b:b+len]. Returns 0 when either window has
// near-zero variance.
func NCC(x, y []float64, a, b, length int) float64 {
	if a < 0 || b < 0 || a+length > len(x) || b+length > len(y) || length < 2 {
		return 0
	}
	var sx, sy, sxx, syy, sxy float64
	for k := 0; k < length; k++ {
		u, v := x[a+k], y[b+k]
		sx += u
		sy += v
		sxx += u * u
		syy += v * v
		sxy += u * v
	}
	n := float64(length)
	cov := sxy - sx*sy/n
	vx := sxx - sx*sx/n
	vy := syy - sy*sy/n
	den := math.Sqrt(vx * vy)
	if den <= 1e-12 {
		return 0
	}
	r := cov / den
	// Clamp tiny numerical overshoots.
	if r > 1 {
		r = 1
	} else if r < -1 {
		r = -1
	}
	return r
}

// SlidingNCC computes, for one fixed lag, the NCC between the segments
// starting at t and t+lag, over windows of length wlen, stepping by hop.
// Prefix sums make every evaluation O(wlen-independent); the cost is
// O(number of positions) with constant work each.
func SlidingNCC(x []float64, lag, wlen, hop int) (positions []int, r []float64) {
	n := len(x)
	if lag <= 0 || wlen < 2 || lag+wlen > n {
		return nil, nil
	}
	px := prefixSums(x)
	for t := 0; t+lag+wlen <= n; t += hop {
		positions = append(positions, t)
		r = append(r, nccPrefix(px, x, t, t+lag, wlen))
	}
	return positions, r
}

type prefix struct {
	s, ss []float64
}

func prefixSums(x []float64) prefix {
	p := prefix{s: make([]float64, len(x)+1), ss: make([]float64, len(x)+1)}
	for i, v := range x {
		p.s[i+1] = p.s[i] + v
		p.ss[i+1] = p.ss[i] + v*v
	}
	return p
}

// nccPrefix computes NCC of x[a:a+n] vs x[b:b+n] from prefix sums plus a
// direct cross-product pass (O(n) per evaluation).
func nccPrefix(p prefix, x []float64, a, b, n int) float64 {
	sx := p.s[a+n] - p.s[a]
	sy := p.s[b+n] - p.s[b]
	sxx := p.ss[a+n] - p.ss[a]
	syy := p.ss[b+n] - p.ss[b]
	sxy := 0.0
	for k := 0; k < n; k++ {
		sxy += x[a+k] * x[b+k]
	}
	fn := float64(n)
	cov := sxy - sx*sy/fn
	vx := sxx - sx*sx/fn
	vy := syy - sy*sy/fn
	den := math.Sqrt(vx * vy)
	if den <= 1e-12 {
		return 0
	}
	r := cov / den
	if r > 1 {
		return 1
	}
	if r < -1 {
		return -1
	}
	return r
}

// BestOffset searches offsets in [center-tol, center+tol] and returns the
// offset maximising NCC of the two length-wlen windows, plus the r value.
func BestOffset(x []float64, a, b, wlen, tol int) (int, float64) {
	bestOff, bestR := 0, -2.0
	for d := -tol; d <= tol; d++ {
		bb := b + d
		r := NCC(x, x, a, bb, wlen)
		if r > bestR {
			bestR, bestOff = r, d
		}
	}
	return bestOff, bestR
}
