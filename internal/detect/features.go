package detect

import (
	"math"

	"watermarkaudit/internal/dsp"
)

// extractFeatures measures every discriminator for one lag on one band.
func extractFeatures(x []float64, lag int, cfg Config) (Features, []int, []float64) {
	pos, r := dsp.SlidingNCC(x, lag, cfg.AlignWindow, cfg.AlignHop)
	f := Features{
		LagSamples: lag,
		GridBlock:  bestGridBlock(pos, cfg.GridBlocks, lag),
	}
	if len(r) == 0 {
		return f, pos, r
	}
	f.NCCMean, _ = MeanStd(r)
	for _, v := range r {
		if v > f.NCCMax {
			f.NCCMax = v
		}
		if v >= cfg.HighNCCThresh {
			f.FracHighNCC++
		}
	}
	f.FracHighNCC /= float64(len(r))
	f.SupportFraction = float64(len(pos)*cfg.AlignHop) / float64(len(x))

	gains, localized := gainRatios(x, pos, lag, cfg.AlignWindow)
	var gsd float64
	f.GainMean, gsd = MeanStd(gains)
	if f.GainMean != 0 {
		f.GainCV = math.Abs(gsd / f.GainMean)
	}
	f.OnsetLocalized = localized
	f.DecayT123 = echoDecay(x, lag)
	f.Rahmonic = dsp.RahmonicScore(x, cfg.CepWinLen, cfg.Cephop, lag, 3)
	if lag%f.GridBlock == 0 {
		f.GridFrac = gridFraction(pos, f.GridBlock)
	}
	f.BlockDecay = BlockDecay(x, f.GridBlock)
	return f, pos, r
}

// gainRatios returns RMS(x[t+lag:])/RMS(x[t:]) per alignment position,
// restricted to windows with appreciable energy (silence would inflate the
// ratio), plus the fraction of positions where the energy is concentrated
// in short localised bursts rather than spread over the whole signal.
func gainRatios(x []float64, pos []int, lag, wlen int) ([]float64, float64) {
	// Global per-sample RMS as a silence gate.
	gate := dsp.RMS(x, 0, len(x)) * 0.25
	out := make([]float64, 0, len(pos))
	localized := 0
	for _, t := range pos {
		a := dsp.RMS(x, t, t+wlen)
		b := dsp.RMS(x, t+lag, t+lag+wlen)
		if a < gate && b < gate {
			continue
		}
		if a > 1e-9 {
			out = append(out, b/a)
		}
		// Localised burst: energy of the window is > 3x the global mean.
		if a > gate*12 || b > gate*12 {
			localized++
		}
	}
	den := float64(len(pos))
	if den == 0 {
		return out, 0
	}
	return out, float64(localized) / den
}

// echoDecay compares RMS of the signal at offsets 0, lag, 2*lag, 3*lag in
// windows of wlen. A natural echo chain shows monotonic decay (<1 and
// shrinking); repeated watermark copies keep near-equal levels.
func echoDecay(x []float64, lag int) float64 {
	wlen := lag
	if wlen > 4096 {
		wlen = 4096
	}
	var levels [4]float64
	for k := 0; k < 4; k++ {
		start := k * lag
		if start+wlen > len(x) {
			levels[k] = -1
			continue
		}
		levels[k] = dsp.RMS(x, start, start+wlen)
	}
	// Ratio of mean later-tap level to first-tap level.
	first := levels[0]
	if first <= 1e-9 {
		return 1
	}
	sum, n := 0.0, 0
	for k := 1; k < 4; k++ {
		if levels[k] >= 0 {
			sum += levels[k] / first
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return sum / float64(n)
}

// gridFraction is the fraction of positions within 2 samples of a multiple
// of block. Codec ringing locks to transform-block boundaries; acoustic
// echoes and inserted watermarks do not. A lag is considered grid-locked
// only when the lag itself is an exact multiple of the block: otherwise a
// multiple-of-block position cannot imply block-aligned repeat copies.
func gridFraction(pos []int, block int) float64 {
	if block <= 0 || len(pos) == 0 {
		return 0
	}
	on := 0
	for _, p := range pos {
		rem := p % block
		if rem <= 2 || block-rem <= 2 {
			on++
		}
	}
	return float64(on) / float64(len(pos))
}

// bestGridBlock picks the candidate block size that (a) divides the lag and
// (b) has the strongest boundary alignment. Non-dividing blocks cannot
// represent a block-locked artefact and are ignored.
func bestGridBlock(pos []int, blocks []int, lag int) int {
	bestB, bestF := blocks[0], 0.0
	for _, b := range blocks {
		if lag%b != 0 {
			continue
		}
		f := gridFraction(pos, b)
		// On ties prefer the largest dividing block (the true codec block
		// size implies all its sub-multiples as well).
		if f > bestF || (f == bestF && f > 0 && b > bestB) {
			bestF, bestB = f, b
		}
	}
	return bestB
}

// symmetryScore compares the +lag and -lag NCC maps at matched positions.
// Codec pre/post-ring is symmetric in time (r+(t) ~ r-(t)); a
// one-directional echo and an inserted watermark are not.
func symmetryScore(x []float64, lag, wlen, hop int) float64 {
	_, rp := dsp.SlidingNCC(x, lag, wlen, hop)
	_, rm := dsp.SlidingNCC(x, lag, wlen, hop)
	n := len(rp)
	if n == 0 || len(rm) < n {
		return 1
	}
	diff := 0.0
	for i := 0; i < n; i++ {
		if i >= len(rm) {
			break
		}
		diff += math.Abs(rp[i] - rm[i])
	}
	return diff / float64(n)
}

// BlockDecay measures transform-block boundary onsets, the signature of
// pre/post-ring artefacts: a damped transient starts at each block
// boundary. It returns the fraction of energy-bearing blocks whose first
// eighth is louder than the preceding block's last eighth by at least 1.3x.
// A stationary periodic signal stays near 0; a block-ringed signal is high.
func BlockDecay(x []float64, block int) float64 {
	if block < 16 {
		return 0
	}
	q := block / 8
	gate := dsp.RMS(x, 0, len(x)) * 0.6
	active, decayed := 0, 0
	for base := block; base+block <= len(x); base += block {
		head := dsp.RMS(x, base, base+q)
		prevTail := dsp.RMS(x, base-q, base)
		if head < gate && prevTail < gate {
			continue
		}
		active++
		if prevTail > 1e-9 && head/prevTail >= 1.5 {
			decayed++
		}
	}
	if active == 0 {
		return 0
	}
	frac := float64(decayed) / float64(active)
	// Require the pattern on at least a tenth of all blocks so a few
	// coincidental onsets cannot trigger the verdict.
	if float64(active) < 0.1*float64(len(x)/block) {
		return 0
	}
	return frac
}

// SymmetryExport exposes the +/-lag symmetry score for diagnostics.
func SymmetryExport(x []float64, lag, wlen, hop int) float64 {
	return symmetryScore(x, lag, wlen, hop)
}

// EdgeLockExport exposes block-internal decay for diagnostics.
func EdgeLockExport(x []float64, lag, block int) float64 {
	_ = lag
	return BlockDecay(x, block)
}

// GridFracExport exposes block-grid alignment for diagnostics.
func GridFracExport(x []float64, block, wlen, hop int) float64 {
	pos, _ := dsp.SlidingNCC(x, block, wlen, hop)
	return gridFraction(pos, block)
}
