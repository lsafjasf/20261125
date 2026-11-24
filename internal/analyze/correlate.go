package analyze

import (
	"math"
	"sort"
)

// Match finds the integer sample lag that best aligns b to a. Positive lag
// means b arrives later than a by that many samples.
func Match(a, b []float64, start, end, maxLag int, _ int) DelayPair {
	if start < 0 {
		start = 0
	}
	if end > len(a) {
		end = len(a)
	}
	if end > len(b) {
		end = len(b)
	}
	if end <= start || maxLag < 0 {
		return DelayPair{}
	}
	if maxLag >= end-start {
		maxLag = end - start - 1
	}
	corrs, counts := exactCorrelations(a, b, start, end, maxLag)
	bestLag, bestCorr := 0, 0.0
	bestScore := -1.0
	for lag := -maxLag; lag <= maxLag; lag++ {
		corr := corrs[lag]
		score := math.Abs(corr) * math.Sqrt(float64(counts[lag])/float64(end-start))
		if score > bestScore {
			bestLag, bestCorr, bestScore = lag, corr, score
		}
	}
	env := envelopeCorr(a, b, start, end, bestLag)
	return DelayPair{
		LagSamples: bestLag, Correlation: bestCorr, EnvelopeCorr: env,
	}
}

func exactCorrelations(a, b []float64, start, end, maxLag int) (map[int]float64, map[int]int) {
	length := end - start
	nfft := 1
	for nfft < length+maxLag+1 {
		nfft <<= 1
	}
	fa := make([]complex128, nfft)
	fb := make([]complex128, nfft)
	prefixA := make([]float64, length+1)
	prefixB := make([]float64, length+1)
	prefixAA := make([]float64, length+1)
	prefixBB := make([]float64, length+1)
	for i := 0; i < length; i++ {
		av, bv := a[start+i], b[start+i]
		fa[i] = complex(av, 0)
		fb[i] = complex(bv, 0)
		prefixA[i+1] = prefixA[i] + av
		prefixB[i+1] = prefixB[i] + bv
		prefixAA[i+1] = prefixAA[i] + av*av
		prefixBB[i+1] = prefixBB[i] + bv*bv
	}
	fft(fa)
	fft(fb)
	for i := range fa {
		fa[i] = cmplxConj(fa[i]) * fb[i]
	}
	fftInverse(fa)
	corrs := make(map[int]float64, 2*maxLag+1)
	counts := make(map[int]int, 2*maxLag+1)
	for lag := -maxLag; lag <= maxLag; lag++ {
		var loA, hiA, loB, hiB int
		if lag >= 0 {
			loA, hiA = 0, length-lag
			loB, hiB = lag, length
		} else {
			loA, hiA = -lag, length
			loB, hiB = 0, length+lag
		}
		n := hiA - loA
		if n < 32 || hiB <= loB {
			counts[lag] = 0
			continue
		}
		sumA := prefixA[hiA] - prefixA[loA]
		sumB := prefixB[hiB] - prefixB[loB]
		sumAA := prefixAA[hiA] - prefixAA[loA]
		sumBB := prefixBB[hiB] - prefixBB[loB]
		index := lag
		if index < 0 {
			index += nfft
		}
		sumAB := real(fa[index])
		denA := float64(n)*sumAA - sumA*sumA
		denB := float64(n)*sumBB - sumB*sumB
		if denA > 1e-20 && denB > 1e-20 {
			value := (float64(n)*sumAB - sumA*sumB) / math.Sqrt(denA*denB)
			corrs[lag] = math.Max(-1, math.Min(1, value))
		}
		counts[lag] = n
	}
	return corrs, counts
}

func cmplxConj(value complex128) complex128 {
	return complex(real(value), -imag(value))
}

func fftInverse(values []complex128) {
	for i := range values {
		values[i] = cmplxConj(values[i])
	}
	fft(values)
	scale := 1 / float64(len(values))
	for i := range values {
		values[i] = cmplxConj(values[i]) * complex(scale, 0)
	}
}

func envelopeCorr(a, b []float64, start, end, lag int) float64 {
	lo, hi := start, end
	if lag > 0 {
		hi -= lag
	} else if lag < 0 {
		lo -= lag
	}
	if lo < 0 {
		lo = 0
	}
	if hi > len(a) || hi+lag > len(b) || hi <= lo {
		return 0
	}
	const block = 8
	var xs, ys, xx, yy, xy, n float64
	for i := lo; i+block <= hi; i += block {
		j := i + lag
		if j < 0 || j+block > len(b) {
			continue
		}
		var ax, by float64
		for k := 0; k < block; k++ {
			ax += math.Abs(a[i+k])
			by += math.Abs(b[j+k])
		}
		n++
		xs += ax
		ys += by
		xx += ax * ax
		yy += by * by
		xy += ax * by
	}
	if n < 8 {
		return 0
	}
	denX := n*xx - xs*xs
	denY := n*yy - ys*ys
	if denX <= 1e-20 || denY <= 1e-20 {
		return 0
	}
	return (n*xy - xs*ys) / math.Sqrt(denX*denY)
}

func mirrorEvidence(pair DelayPair) bool {
	return pair.Correlation <= -0.75 && math.Abs(pair.Correlation) >= 0.85 && pair.EnvelopeCorr >= 0.70
}

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]int(nil), values...)
	sort.Ints(copyValues)
	return copyValues[len(copyValues)/2]
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	return copyValues[len(copyValues)/2]
}
