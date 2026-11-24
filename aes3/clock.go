package aes3

import (
	"errors"
	"math"
	"sort"
)

const (
	defaultJitterTolerance = 0.35
	defaultMinScore        = 1.0
)

func (o Options) normalized() Options {
	if o.JitterTolerance <= 0 {
		o.JitterTolerance = defaultJitterTolerance
	}
	if o.MinSubframeScore <= 0 {
		o.MinSubframeScore = defaultMinScore
	}
	if o.MinSubframeScore > 1 {
		o.MinSubframeScore = 1
	}
	return o
}

func prepareEdges(in []Edge) []Edge {
	edges := make([]Edge, len(in))
	copy(edges, in)
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Time == edges[j].Time {
			return edges[i].Level < edges[j].Level
		}
		return edges[i].Time < edges[j].Time
	})
	return edges
}

func validTime(t float64) bool {
	return !math.IsNaN(t) && !math.IsInf(t, 0)
}

// recoverHalfUnit estimates h = UI/2 from BMC edge spacings.
//
// Locked BMC data produces spacings of roughly h and 2h; AES3 preambles also
// contain 3h. A robust histogram score avoids treating one injected glitch as
// the clock. The returned value is the median d/k of the winning class.
func recoverHalfUnit(edges []Edge, fixed float64, classTolerance float64) (float64, string, error) {
	if fixed > 0 {
		return fixed, "option", nil
	}
	if len(edges) < 4 {
		return 0, "", errors.New("aes3: at least four edges are required for clock recovery")
	}

	deltas := make([]float64, 0, len(edges)-1)
	for i := 1; i < len(edges); i++ {
		d := edges[i].Time - edges[i-1].Time
		if validTime(d) && d > 0 {
			deltas = append(deltas, d)
		}
	}
	if len(deltas) < 3 {
		return 0, "", errors.New("aes3: edge timestamps must contain positive intervals")
	}
	sort.Float64s(deltas)

	candidates := clockCandidates(deltas)
	bestH := 0.0
	bestScore := -1.0
	bestInliers := 0
	for _, h := range candidates {
		if h <= 0 {
			continue
		}
		score := 0.0
		inliers := 0
		for _, d := range deltas {
			k := int(math.Round(d / h))
			if k < 1 || k > 3 {
				continue
			}
			classError := math.Abs(d/float64(k)-h) / h
			if classError <= classTolerance {
				weight := 1.0 - classError/classTolerance
				score += weight
				inliers++
			}
		}
		// Tiny deterministic tie-break toward candidates explaining more.
		score += float64(inliers) * 1e-6
		if score > bestScore {
			bestScore = score
			bestH = h
			bestInliers = inliers
		}
	}

	if bestH <= 0 || bestInliers < 3 {
		return 0, "", errors.New("aes3: unable to recover a stable biphase clock")
	}

	estimates := make([]float64, 0, bestInliers)
	for _, d := range deltas {
		k := int(math.Round(d / bestH))
		if k < 1 || k > 3 {
			continue
		}
		h := d / float64(k)
		if math.Abs(h-bestH)/bestH <= classTolerance {
			estimates = append(estimates, h)
		}
	}
	sort.Float64s(estimates)
	refined := medianFloat(estimates)
	if refined <= 0 {
		refined = bestH
	}
	return refined, "edge-interval-histogram", nil
}

func clockCandidates(sortedDeltas []float64) []float64 {
	seen := map[float64]bool{}
	out := make([]float64, 0, 80)
	add := func(v float64) {
		if v <= 0 || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}

	limit := len(sortedDeltas)
	if limit > 96 {
		limit = 96
	}
	for _, d := range sortedDeltas[:limit] {
		add(d)
	}
	for _, q := range []float64{0.01, 0.05, 0.10, 0.25, 0.50} {
		add(quantileSorted(sortedDeltas, q))
	}
	sort.Float64s(out)
	return out
}

func quantileSorted(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	if q <= 0 {
		return xs[0]
	}
	if q >= 1 {
		return xs[len(xs)-1]
	}
	pos := q * float64(len(xs)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return xs[lo]
	}
	frac := pos - float64(lo)
	return xs[lo]*(1-frac) + xs[hi]*frac
}

func medianFloat(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	if len(xs)%2 == 1 {
		return xs[len(xs)/2]
	}
	return (xs[len(xs)/2-1] + xs[len(xs)/2]) / 2
}

type waveform struct {
	times  []float64
	levels []Level
}

func newWaveform(edges []Edge) *waveform {
	w := &waveform{times: make([]float64, len(edges)), levels: make([]Level, len(edges))}
	level := LevelLow
	for i, e := range edges {
		w.times[i] = e.Time
		if e.Level == LevelLow || e.Level == LevelHigh {
			level = e.Level
		} else {
			if level == LevelHigh {
				level = LevelLow
			} else {
				level = LevelHigh
			}
		}
		w.levels[i] = level
	}
	return w
}

// levelAt returns the driven level at t. Candidate sample times are always at
// or after their starting edge, so pre-first-edge polarity is irrelevant.
func (w *waveform) levelAt(t float64) Level {
	i := sort.Search(len(w.times), func(i int) bool { return w.times[i] > t }) - 1
	if i < 0 {
		return LevelLow
	}
	return w.levels[i]
}

func (w *waveform) nearestEdge(t float64) (int, float64) {
	i := sort.Search(len(w.times), func(i int) bool { return w.times[i] >= t })
	best := -1
	bestDist := math.Inf(1)
	if i < len(w.times) {
		best = i
		bestDist = math.Abs(w.times[i] - t)
	}
	if i > 0 {
		j := i - 1
		if d := math.Abs(w.times[j] - t); d < bestDist {
			best = j
			bestDist = d
		}
	}
	return best, bestDist
}

func intToBool(v int) bool { return v != 0 }

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
