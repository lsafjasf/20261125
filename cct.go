package nightwb

import (
	"math"
	"sort"
)

// XYZToCCT returns correlated color temperature (K) via McCamy's formula and
// the tint (signed distance normal to the Planckian locus in xy). Returns
// 0,0 when the chromaticity is outside the formula's meaningful range.
func XYZToCCT(x, y, z float64) (cct, tint float64) {
	sum := x + y + z
	if sum <= 1e-12 {
		return 0, 0
	}
	cx := x / sum
	cy := y / sum
	if cy <= 1e-9 {
		return 0, 0
	}
	n := (cx - 0.3320) / (cy - 0.1858)
	t := -449.0*n*n*n + 3525.0*n*n - 6823.3*n + 5520.33
	if t < 1000 || t > 40000 || math.IsNaN(t) {
		return 0, 0
	}
	// Distance of the point from the Planckian locus tangent direction; sign
	// separates greenish (positive) from purplish (negative) chromaticities.
	tint = (cy - locusY(cx)) * 1000
	return t, tint
}

// locusY approximates the Planckian locus y as a function of x over the
// practical CCT range (parabolic fit, Wyszecki & Stiles style).
func locusY(x float64) float64 {
	return -3.0*math.Pow(x, 2) + 2.87*x - 0.275
}

// weightedMedian returns the weighted median of vals. Weights must be >= 0.
func weightedMedian(vals, weights []float64) float64 {
	idx := make([]int, len(vals))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return vals[idx[a]] < vals[idx[b]] })
	var total float64
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		return 0
	}
	var acc float64
	for _, i := range idx {
		acc += weights[i]
		if acc >= total*0.5 {
			return vals[i]
		}
	}
	return vals[idx[len(idx)-1]]
}
