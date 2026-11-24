package detect

import (
	"math"
	"math/rand/v2"
	"sort"
)

// FisherZ transforms a correlation r to the approximately normal z scale.
func FisherZ(r float64) float64 {
	r = clamp(r, -0.999999, 0.999999)
	return 0.5 * math.Log((1+r)/(1-r))
}

// InvFisherZ inverts FisherZ.
func InvFisherZ(z float64) float64 {
	return (math.Exp(2*z) - 1) / (math.Exp(2*z) + 1)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// MeanCI returns a bootstrap percentile CI (default level 0.95). The
// resampling unit is the individual measurement (occurrence or alignment
// position), so clustered evidence is represented honestly.
func MeanCI(values []float64, iters int, rng *rand.Rand) CI {
	if len(values) == 0 {
		return CI{Level: 0.95}
	}
	if iters <= 0 {
		iters = 1000
	}
	means := make([]float64, iters)
	for i := 0; i < iters; i++ {
		sum := 0.0
		for range values {
			sum += values[rng.IntN(len(values))]
		}
		means[i] = sum / float64(len(values))
	}
	sort.Float64s(means)
	lo := means[int(0.025*float64(iters))]
	hi := means[int(0.975*float64(iters))-1]
	return CI{Level: 0.95, Low: lo, High: hi}
}

// MeanCIZ applies the same bootstrap but transforms correlations through
// Fisher z before averaging and back before reporting.
func MeanCIZ(corrs []float64, iters int, rng *rand.Rand) CI {
	if len(corrs) == 0 {
		return CI{Level: 0.95}
	}
	z := make([]float64, len(corrs))
	for i, r := range corrs {
		z[i] = FisherZ(r)
	}
	ci := MeanCI(z, iters, rng)
	return CI{Level: ci.Level, Low: InvFisherZ(ci.Low), High: InvFisherZ(ci.High)}
}

// MeanStd returns arithmetic mean and population standard deviation.
func MeanStd(x []float64) (mean, sd float64) {
	if len(x) == 0 {
		return 0, 0
	}
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	for _, v := range x {
		d := v - mean
		sd += d * d
	}
	sd = math.Sqrt(sd / float64(len(x)))
	return mean, sd
}

// ShuffledNull is the negative control for every claimed repeat. It
// measures NCC at the same lag but with the second window drawn from a
// random, distant, non-matching position, so genuine temporal structure is
// destroyed while marginal spectrum and level are preserved. Any claim
// whose evidence does not clearly exceed this null is rejected.
func ShuffledNull(x []float64, lag, wlen, hop, iters int, rng *rand.Rand) float64 {
	n := len(x)
	if lag+wlen > n {
		return 0
	}
	if n < 4*wlen {
		return 0
	}
	sum, count := 0.0, 0
	maxStart := n - lag - wlen
	for it := 0; it < iters; it++ {
		t := rng.IntN(maxStart + 1)
		// Second window: random distant position, excluding the true-lag
		// neighbourhood and the window itself (+/- 2 window lengths).
		s := rng.IntN(maxStart + 1)
		if abs(s-(t+lag)) < 2*wlen || s == t {
			continue
		}
		sum += corrRaw(x[t:t+wlen], x[s:s+wlen])
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func corrRaw(u, v []float64) float64 {
	if len(u) != len(v) || len(u) < 2 {
		return 0
	}
	var su, sv, suu, svv, suv float64
	for i := range u {
		su += u[i]
		sv += v[i]
		suu += u[i] * u[i]
		svv += v[i] * v[i]
		suv += u[i] * v[i]
	}
	n := float64(len(u))
	cov := suv - su*sv/n
	den := math.Sqrt((suu - su*su/n) * (svv - sv*sv/n))
	if den <= 1e-12 {
		return 0
	}
	return clamp(cov/den, -1, 1)
}
