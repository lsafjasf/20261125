package detect

import (
	"sort"

	"watermarkaudit/internal/dsp"
)

type anchorT struct {
	pos int
	len int
	r   float64
}

type edgePair struct{ a, b int }

// locateRepeats finds families of mutually similar short fragments. The
// design deliberately avoids over-fragmentation:
//
//  1. Onset events use a high z-score gate and wide refractory gap.
//  2. Candidate fragment lengths are tested longest-first.
//  3. A family is accepted only when members form a clique against one
//     anchor at the same length (no chaining of unrelated coincidences).
//  4. Overlapping detections at shorter lengths are suppressed (NMS).
func locateRepeats(bandSignal []float64, cfg Config) [][]Occurrence {
	env, _ := dsp.FrameEnergy(bandSignal, cfg.FrameLen, cfg.Hop)
	events := onsetEvents(env, 2.6, float64(cfg.AlignWindow/cfg.Hop))
	if len(events) < cfg.MinClusterSize {
		return nil
	}
	positions := make([]int, len(events))
	for i, ev := range events {
		positions[i] = ev * cfg.Hop
	}

	var accepted [][]Occurrence
	lens := append([]int(nil), cfg.WatermarkLens...)
	sort.Sort(sort.Reverse(sort.IntSlice(lens)))
	for _, wlen := range lens {
		family := cliqueFamily(bandSignal, positions, wlen, cfg)
		if family == nil {
			continue
		}
		if overlapsAny(family, accepted) {
			continue
		}
		accepted = append(accepted, family)
	}
	sort.Slice(accepted, func(i, j int) bool {
		return accepted[i][0].Range.Start < accepted[j][0].Range.Start
	})
	return accepted
}

// cliqueFamily builds an NCC graph at one fixed length and keeps only
// components whose members all correlate with the strongest anchor above
// PairNCCThresh. This rejects transitive coincidence chains.
func cliqueFamily(xs []float64, positions []int, wlen int, cfg Config) []Occurrence {
	n := len(positions)
	// Longer windows average over more surrounding content and naturally
	// lower the fragment NCC, so the clique gate scales with window length.
	pairThresh := cfg.PairNCCThresh - 0.05*float64(wlen/cfg.WatermarkLens[0]-1)
	if pairThresh < 0.5 {
		pairThresh = 0.5
	}
	bestR := make([][]float64, n)
	for i := range bestR {
		bestR[i] = make([]float64, n)
	}
	adj := make([][]int, n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if positions[j]-positions[i] < wlen/2 {
				continue
			}
			off, r := localAlign(xs, positions[i], positions[j], wlen, wlen/2)
			_ = off
			bestR[i][j], bestR[j][i] = r, r
			if r >= pairThresh {
				adj[i] = append(adj[i], j)
				adj[j] = append(adj[j], i)
			}
		}
	}
	seen := make([]bool, n)
	var best []Occurrence
	for start := 0; start < n; start++ {
		if seen[start] || len(adj[start]) == 0 {
			continue
		}
		comp := bfsComponent(start, adj, seen)
		anchor := strongestAnchor(comp, bestR)
		members := []int{anchor}
		var occ []Occurrence
		for _, idx := range comp {
			if idx == anchor {
				continue
			}
			off, r := localAlign(xs, positions[anchor], positions[idx], wlen, wlen/2)
			if r < pairThresh {
				continue // clique gate: must match the anchor directly
			}
			matchedStart := positions[idx] + off
			members = append(members, idx)
			occ = append(occ, Occurrence{
				Range: Range{Start: matchedStart, End: matchedStart + wlen},
				NCC:   r,
				GainVsAnchor: dsp.RMS(xs, matchedStart, matchedStart+wlen) /
					safeRMS(xs, positions[anchor], positions[anchor]+wlen),
				OffsetJitter: off,
			})
		}
		if len(members) < cfg.MinClusterSize {
			continue
		}
		anchorOcc := Occurrence{
			Range:        Range{Start: positions[anchor], End: positions[anchor] + wlen},
			NCC:          1,
			GainVsAnchor: 1,
		}
		family := append([]Occurrence{anchorOcc}, occ...)
		sort.Slice(family, func(i, j int) bool {
			return family[i].Range.Start < family[j].Range.Start
		})
		if len(family) > len(best) {
			best = family
		}
	}
	return best
}

// localAlign scans b over [b-tol, b+tol] and returns the offset and NCC of
// the best-aligned window against the fixed window at a. Onset markers only
// approximate a fragment start, so this local search recovers the true
// alignment without any global warping or denoising.
func localAlign(xs []float64, a, b, wlen, tol int) (int, float64) {
	bestOff, bestR := 0, -2.0
	lo := b - tol
	if lo < 0 {
		lo = 0
	}
	for bb := lo; bb+wlen <= len(xs) && bb <= b+tol; bb++ {
		if a+wlen > len(xs) {
			break
		}
		r := dsp.NCC(xs, xs, a, bb, wlen)
		if r > bestR {
			bestR, bestOff = r, bb-b
		}
	}
	return bestOff, bestR
}

func bfsComponent(start int, adj [][]int, seen []bool) []int {
	queue := []int{start}
	seen[start] = true
	var comp []int
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		comp = append(comp, v)
		for _, nb := range adj[v] {
			if !seen[nb] {
				seen[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	return comp
}

func strongestAnchor(comp []int, r [][]float64) int {
	anchor, bestSum := comp[0], -1.0
	for _, c := range comp {
		sum := 0.0
		for _, d := range comp {
			sum += r[c][d]
		}
		if sum > bestSum {
			bestSum, anchor = sum, c
		}
	}
	return anchor
}

// overlapsAny reports whether a new family's spans already intersect an
// accepted family (non-maximum suppression across fragment lengths).
func overlapsAny(family []Occurrence, accepted [][]Occurrence) bool {
	for _, old := range accepted {
		for _, o := range family {
			for _, p := range old {
				if o.Range.Start < p.Range.End && p.Range.Start < o.Range.End {
					return true
				}
			}
		}
	}
	return false
}

func safeRMS(x []float64, lo, hi int) float64 {
	v := dsp.RMS(x, lo, hi)
	if v < 1e-9 {
		return 1e-9
	}
	return v
}

// LocateExport exposes fragment-family detection for diagnostics.
func LocateExport(xs []float64, cfg Config) [][]Occurrence {
	return locateRepeats(xs, cfg)
}
