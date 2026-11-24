package detect

import (
	"sort"

	"watermarkaudit/internal/dsp"
)

// lagHyp is a candidate repeat spacing with an accumulated evidence score.
type lagHyp struct {
	lag    int
	score  float64
	source string
}

// discoverLags fuses three independent lag sources per band and clusters
// lags within tolerance. No source is allowed to dominate silently:
// every retained lag records which evidence produced it.
func discoverLags(bandSignal []float64, cfg Config) []lagHyp {
	scores := map[int]float64{}
	src := map[int]string{}

	add := func(lag int, s float64, tag string) {
		if lag < cfg.MinLag || lag > cfg.MaxLag {
			return
		}
		scores[lag] += s
		if _, ok := src[lag]; !ok {
			src[lag] = tag
		} else {
			src[lag] += "+" + tag
		}
	}

	// 1) Windowed cepstral peaks (quefrency = repeat lag).
	cep, _ := dsp.CepstralPeaks(bandSignal, cfg.CepWinLen, cfg.Cephop,
		cfg.MinLag, cfg.MaxLag, 8, 1e-6)
	for lag, s := range cep {
		add(lag, s*1000, "cepstrum")
	}

	// 2) Frame-energy envelope autocorrelation peaks: repeated bursts of
	// band energy show up as periodic envelope maxima.
	env, _ := dsp.FrameEnergy(bandSignal, cfg.FrameLen, cfg.Hop)
	ac := autocorr(env)
	peaks := dsp.LocalPeaks(ac, cfg.MinLag/cfg.Hop, cfg.MaxLag/cfg.Hop, 4, 0.05)
	for _, p := range peaks {
		add(p.Index*cfg.Hop, p.Prominence, "envelope")
	}

	// 3) Onset-event intervals: localised energy rises and the gaps between
	// them catch irregularly spaced (non-periodic) watermark insertions.
	events := onsetEvents(env, 1.8, 3)
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			gap := (events[j] - events[i]) * cfg.Hop
			add(gap, 0.5, "onset")
		}
	}

	// 4) Explicit codec-block hypotheses: ringing repeats at transform-block
	// multiples, so test those lags directly instead of relying on cepstral
	// peak quantisation landing exactly on the block size.
	for _, block := range cfg.GridBlocks {
		for mult := 1; mult <= 4; mult++ {
			lag := block * mult
			if lag >= cfg.MinLag && lag <= cfg.MaxLag {
				add(lag, 0.25, "blockgrid")
			}
		}
	}

	var hyps []lagHyp
	for lag, s := range scores {
		hyps = append(hyps, lagHyp{lag: lag, score: s, source: src[lag]})
	}
	sort.Slice(hyps, func(i, j int) bool { return hyps[i].score > hyps[j].score })
	return clusterLags(hyps, 12, 10, cfg.GridBlocks)
}

// clusterLags merges hypotheses whose lags differ by <= tol samples,
// keeping at most maxOut and preserving the strongest member's lag.
func clusterLags(hyps []lagHyp, tol, maxOut int, blocks []int) []lagHyp {
	used := make([]bool, len(hyps))
	var out []lagHyp
	for i, h := range hyps {
		if used[i] {
			continue
		}
		used[i] = true
		member := 1
		for j := i + 1; j < len(hyps); j++ {
			if !used[j] && abs(hyps[j].lag-h.lag) <= tol {
				used[j] = true
				// Prefer an exact transform-block lag when the cluster
				// contains one (cepstral peaks quantise around it).
				for _, b := range blocks {
					if hyps[j].lag == b || (hyps[j].lag >= b && hyps[j].lag%b == 0) {
						h.lag = hyps[j].lag
					}
				}
				h.score += hyps[j].score
				h.source += "," + hyps[j].source
				member++
			}
		}
		_ = member
		out = append(out, h)
		if len(out) >= maxOut {
			break
		}
	}
	return out
}

func autocorr(x []float64) []float64 {
	n := len(x)
	mean, _ := MeanStd(x)
	y := make([]float64, n)
	var denom float64
	for i, v := range x {
		y[i] = v - mean
		denom += y[i] * y[i]
	}
	if denom == 0 {
		return make([]float64, n)
	}
	out := make([]float64, n)
	for lag := 0; lag < n; lag++ {
		s := 0.0
		for i := 0; i+lag < n; i++ {
			s += y[i] * y[i+lag]
		}
		out[lag] = s / denom
	}
	return out
}

// onsetEvents returns frame indices whose energy exceeds z*localSD above the
// moving average, separated by at least minGap frames.
func onsetEvents(env []float64, z, minGap float64) []int {
	w := 16
	var events []int
	last := -1000
	for i := w; i < len(env)-w; i++ {
		m, sd := MeanStd(env[i-w : i+w])
		if sd == 0 {
			continue
		}
		if env[i] > m+z*sd && i-last >= int(minGap) {
			events = append(events, i)
			last = i
		}
	}
	return events
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// OnsetEventsExport exposes onset detection for external diagnostics.
func OnsetEventsExport(env []float64, z, minGap float64) []int {
	return onsetEvents(env, z, minGap)
}
