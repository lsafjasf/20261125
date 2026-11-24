package cutluma

import (
	"math"
	"sort"
)

// clamp01 constrains v to the normalized luma range [0,1].
func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// clampF constrains v to [lo, hi].
func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func minF(a, b float64) float64 { return math.Min(a, b) }
func maxF(a, b float64) float64 { return math.Max(a, b) }

// sortedCopy returns a sorted copy of xs.
func sortedCopy(xs []float64) []float64 {
	out := append([]float64(nil), xs...)
	sort.Float64s(out)
	return out
}

// Quantile returns the linearly interpolated order-statistic at q in [0,1].
// xs does not need to be sorted; an empty slice yields 0.
func Quantile(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := sortedCopy(xs)
	return quantileSorted(s, q)
}

func quantileSorted(s []float64, q float64) float64 {
	n := len(s)
	if n == 1 {
		return s[0]
	}
	pos := clampF(q, 0, 1) * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return s[lo]
	}
	frac := pos - float64(lo)
	return s[lo]*(1-frac) + s[hi]*frac
}

// Median is the 0.5 quantile.
func Median(xs []float64) float64 { return Quantile(xs, 0.5) }

// MAD returns the median absolute deviation around the median.
func MAD(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	med := Median(xs)
	dev := make([]float64, len(xs))
	for i, x := range xs {
		dev[i] = math.Abs(x - med)
	}
	return Median(dev)
}

// RobustSigma estimates the standard deviation scale from MAD.
func RobustSigma(xs []float64) float64 {
	return 1.4826 * MAD(xs)
}

// TheilSenLine is a robust straight-line fit y = intercept + slope*x.
// The slope is the median of all pairwise slopes; the intercept is the
// median of y_i - slope*x_i.
type Line struct {
	Slope     float64 `json:"slope"`
	Intercept float64 `json:"intercept"`
}

func (l Line) At(x float64) float64 { return l.Intercept + l.Slope*x }

// TheilSen fits a robust line. With one point slope is zero.
func TheilSen(xs, ys []float64) Line {
	n := len(xs)
	if n == 0 || n != len(ys) {
		return Line{}
	}
	if n == 1 {
		return Line{Intercept: ys[0]}
	}
	slopes := make([]float64, 0, n*(n-1)/2)
	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			dx := xs[j] - xs[i]
			if math.Abs(dx) < 1e-12 {
				continue
			}
			slopes = append(slopes, (ys[j]-ys[i])/dx)
		}
	}
	if len(slopes) == 0 {
		return Line{Intercept: Median(ys)}
	}
	m := Median(slopes)
	offs := make([]float64, n)
	for i := range xs {
		offs[i] = ys[i] - m*xs[i]
	}
	return Line{Slope: m, Intercept: Median(offs)}
}

// meanFloat is a small arithmetic mean helper.
func meanFloat(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// medianOf is an alias used for local level estimates.
func medianOf(xs []float64) float64 { return Median(xs) }
