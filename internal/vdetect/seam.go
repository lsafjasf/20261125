package vdetect

import (
	"math"
	"sort"
)

type blockVote struct {
	x0, x1  int
	d       int
	cost    float64
	zero    float64
	texture float64
	weight  float64
}

type seamCandidate struct {
	y          int
	shift      float64
	coverage   float64
	mismatch   float64
	aligned    float64
	zero       float64
	texture    float64
	support    int
	total      int
	confidence float64
	x0, x1     int
	votes      []blockVote
}

// detectTears finds coherent horizontal buffer/scan discontinuities.
func detectTears(g *GrayFrame, opt Options) []Tear {
	if g.W < 48 || g.H < 32 {
		return nil
	}
	maxD := int(math.Round(float64(g.W) * clamp(opt.MaxShiftRatio, 0.02, 0.2)))
	maxD = clampInt(maxD, 3, maxInt(3, g.W/5))
	band := clampInt(g.H/48, 3, 6)
	blockW := clampInt(g.W/14, 24, 48)
	step := maxInt(8, blockW/2)

	type yResult struct {
		cand *seamCandidate
	}
	results := make([]seamCandidate, 0)

	for y := band; y <= g.H-band; y++ {
		upper, lower := seamSignatures(g, y, band)
		votes := seamBlockVotes(g, upper, lower, y, maxD, blockW, step, opt)
		cand := aggregateSeamVotes(votes, g.W, maxD, opt)
		if cand != nil {
			cand.y = y
			results = append(results, *cand)
		}
	}

	merged := mergeSeamCandidates(results, g, opt)
	tears := make([]Tear, 0, len(merged))
	for _, c := range merged {
		if c.support < 7 {
			continue
		}
		if math.Abs(c.shift) >= float64(maxD)-0.5 {
			if c.coverage < 0.58 || c.support < 10 {
				continue
			}
			c.confidence = clamp01(c.confidence * 0.82)
		}
		x0 := int(math.Floor(float64(c.x0) * g.ScaleX))
		x1 := int(math.Ceil(float64(c.x1) * g.ScaleX))
		ySrc := int(math.Round(float64(c.y) * g.ScaleY))
		t := Tear{
			Y:             clampInt(ySrc, 0, int(math.Round(float64(g.H)*g.ScaleY))-1),
			XStart:        clampInt(x0, 0, int(math.Round(float64(g.W)*g.ScaleX))),
			XEnd:          clampInt(x1, 0, int(math.Round(float64(g.W)*g.ScaleX))),
			Coverage:      round3(c.coverage),
			ShiftPx:       round2(c.shift * g.ScaleX),
			ShiftSamples:  round2(c.shift),
			WidthPx:       int(math.Round(float64(x1 - x0))),
			Mismatch:      round3(c.mismatch),
			AlignedCost:   round3(c.aligned),
			ZeroCost:      round3(c.zero),
			Texture:       round2(c.texture),
			SupportBlocks: c.support,
			TotalBlocks:   c.total,
			Confidence:    round3(c.confidence),
		}
		if t.ShiftPx < 0 {
			t.ShiftPx = -t.ShiftPx
		}
		t.Evidence = seamEvidence(t)
		tears = append(tears, t)
	}
	sort.Slice(tears, func(i, j int) bool { return tears[i].Confidence > tears[j].Confidence })
	return tears
}

func seamSignatures(g *GrayFrame, y, band int) ([]int16, []int16) {
	w, h := g.W, g.H
	upper := make([]int16, w)
	lower := make([]int16, w)
	for x := 1; x < w-1; x++ {
		var us, ls, un, ln int
		for k := band; k >= 1; k-- {
			yy := y - k
			if yy >= 0 && yy < h {
				us += int(g.Y[yy*w+x+1]) - int(g.Y[yy*w+x-1])
				un++
			}
		}
		for k := 0; k < band; k++ {
			yy := y + k
			if yy >= 0 && yy < h {
				ls += int(g.Y[yy*w+x+1]) - int(g.Y[yy*w+x-1])
				ln++
			}
		}
		if un > 0 {
			upper[x] = int16(us / un)
		}
		if ln > 0 {
			lower[x] = int16(ls / ln)
		}
	}
	return upper, lower
}

func seamBlockVotes(g *GrayFrame, upper, lower []int16, y, maxD, blockW, step int, opt Options) []blockVote {
	w := g.W
	var votes []blockVote
	sensitivity := clamp(opt.Sensitivity, 0.1, 1)
	textureFloor := 4.2 - 2.2*sensitivity

	for x0 := 6; x0+blockW < w-6; x0 += step {
		x1 := x0 + blockW
		var textureSum, zeroSum float64
		for x := x0; x < x1; x++ {
			textureSum += math.Abs(float64(upper[x])) + math.Abs(float64(lower[x]))
			zeroSum += math.Abs(float64(upper[x]) - float64(lower[x]))
		}
		texture := textureSum / float64(2*blockW)
		zero := zeroSum / float64(blockW)
		if texture < textureFloor {
			continue
		}

		bestD, secondD := 0, 0
		bestCost, secondCost := math.Inf(1), math.Inf(1)
		costs := make([]float64, 2*maxD+1)
		for d := -maxD; d <= maxD; d++ {
			sum, n := 0.0, 0
			for x := x0; x < x1; x++ {
				lx := x + d
				if lx < 1 || lx >= w-1 {
					continue
				}
				sum += math.Abs(float64(upper[x]) - float64(lower[lx]))
				n++
			}
			cost := sum / float64(maxInt(1, n))
			costs[d+maxD] = cost
			if cost < bestCost {
				secondCost, secondD = bestCost, bestD
				bestCost, bestD = cost, d
			} else if cost < secondCost {
				secondCost, secondD = cost, d
			}
		}

		// A zero displacement is always the null hypothesis.
		if costs[maxD] < bestCost {
			bestCost, bestD = costs[maxD], 0
		}
		_ = secondD
		improvement := (zero - bestCost) / math.Max(zero, texture*1.5)
		relativeCost := bestCost / math.Max(texture, 3)
		margin := (secondCost - bestCost) / math.Max(secondCost, texture)
		weight := clamp01((texture-textureFloor)/(textureFloor+8)) *
			clamp01(improvement*1.7) *
			clamp01((0.85-relativeCost)*2.2) *
			clamp01(margin*2.5+0.25)
		if absInt(bestD) < 1 || weight < 0.13 || improvement < 0.13 || relativeCost > 1.0 {
			continue
		}
		votes = append(votes, blockVote{x0: x0 - step/2, x1: x1 + step/2, d: bestD, cost: bestCost, zero: zero, texture: texture, weight: weight})
	}
	return votes
}

func aggregateSeamVotes(votes []blockVote, width, maxD int, opt Options) *seamCandidate {
	if len(votes) < 4 {
		return nil
	}
	minCoverage := 0.48 - 0.18*clamp(opt.Sensitivity, 0, 1)
	minCoverage = clamp(minCoverage, 0.28, 0.48)

	best := (*seamCandidate)(nil)
	for centerD := -maxD; centerD <= maxD; centerD++ {
		support := make([]blockVote, 0, len(votes))
		var weight, cost, zero, texture float64
		intervals := make([][2]int, 0, len(votes))
		for _, v := range votes {
			if absInt(v.d-centerD) <= 1 {
				support = append(support, v)
				weight += v.weight
				cost += v.cost * v.weight
				zero += v.zero * v.weight
				texture += v.texture * v.weight
				intervals = append(intervals, [2]int{v.x0, v.x1})
			}
		}
		if len(support) < 4 || weight <= 0 {
			continue
		}
		x0, x1, coverage := unionCoverage(intervals, width)
		if coverage < minCoverage {
			continue
		}
		candShift := weightedShift(support)
		if math.Abs(candShift) < 1 {
			continue
		}
		cost /= weight
		zero /= weight
		texture /= weight
		strength := clamp01((zero - cost) / math.Max(zero, texture*1.4))
		quality := clamp01(1.15 - cost/math.Max(texture, 2))
		widthConf := clamp01((coverage - minCoverage) / (0.85 - minCoverage))
		conf := clamp01(0.16 + 0.30*coverage + 0.24*strength + 0.16*quality + 0.14*widthConf)
		cand := &seamCandidate{
			shift: candShift, coverage: coverage, mismatch: strength, aligned: cost, zero: zero,
			texture: texture, support: len(support), total: len(votes), confidence: conf,
			x0: x0, x1: x1, votes: support,
		}
		if best == nil || cand.confidence > best.confidence {
			best = cand
		}
	}
	if best == nil || best.confidence < 0.43+0.12*(1-opt.Sensitivity) {
		return nil
	}
	return best
}

func weightedShift(votes []blockVote) float64 {
	var num, den float64
	for _, v := range votes {
		num += float64(v.d) * v.weight
		den += v.weight
	}
	if den == 0 {
		return 0
	}
	d := num / den
	di := int(math.Round(d))
	var cMinus, c0, cPlus float64
	var nMinus, n0, nPlus float64
	for _, v := range votes {
		// Votes retain only the winning integer cost; use a robust weighted mean.
		_ = v.cost
		if v.d == di-1 {
			cMinus += v.weight
			nMinus++
		} else if v.d == di {
			c0 += v.weight
			n0++
		} else if v.d == di+1 {
			cPlus += v.weight
			nPlus++
		}
	}
	_ = cMinus + c0 + cPlus + nMinus + n0 + nPlus
	return d
}

func unionCoverage(intervals [][2]int, width int) (int, int, float64) {
	if len(intervals) == 0 {
		return 0, 0, 0
	}
	sorted := append([][2]int(nil), intervals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i][0] < sorted[j][0] })
	merged := [][2]int{sorted[0]}
	for _, in := range sorted[1:] {
		last := &merged[len(merged)-1]
		if in[0] <= last[1] {
			if in[1] > last[1] {
				last[1] = in[1]
			}
		} else {
			merged = append(merged, in)
		}
	}
	covered, x0, x1 := 0, width, 0
	for _, in := range merged {
		covered += maxInt(0, in[1]-in[0])
		x0 = minInt(x0, in[0])
		x1 = maxInt(x1, in[1])
	}
	return clampInt(x0, 0, width), clampInt(x1, 0, width), clamp01(float64(covered) / float64(width))
}

func mergeSeamCandidates(cands []seamCandidate, g *GrayFrame, opt Options) []seamCandidate {
	if len(cands) == 0 {
		return nil
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].y < cands[j].y })
	var groups [][]seamCandidate
	var cur []seamCandidate
	for _, c := range cands {
		if len(cur) == 0 || c.y-cur[len(cur)-1].y <= 2 {
			cur = append(cur, c)
		} else {
			groups = append(groups, cur)
			cur = []seamCandidate{c}
		}
	}
	if cur != nil {
		groups = append(groups, cur)
	}

	out := make([]seamCandidate, 0, len(groups))
	for _, group := range groups {
		best := group[0]
		yWeight, wSum := 0.0, 0.0
		for _, c := range group {
			if c.confidence > best.confidence {
				best = c
			}
			yWeight += float64(c.y) * c.confidence
			wSum += c.confidence
		}
		if wSum > 0 {
			best.y = int(math.Round(yWeight / wSum))
		}
		if len(group) > 1 {
			best.confidence = clamp01(best.confidence + 0.03*float64(len(group)-1))
		}
		// A thick response can indicate a genuine comb/field pattern rather than one seam.
		if len(group) > 5 {
			best.confidence *= 0.72
		}
		best.confidence *= clamp(opt.Sensitivity+0.35, 0.75, 1.25)
		best.confidence = clamp01(best.confidence)
		if best.confidence >= 0.40 {
			out = append(out, best)
		}
	}
	return nonMaximumSuppression(out)
}

func nonMaximumSuppression(cs []seamCandidate) []seamCandidate {
	sort.Slice(cs, func(i, j int) bool { return cs[i].confidence > cs[j].confidence })
	var kept []seamCandidate
	for _, c := range cs {
		duplicate := false
		for _, k := range kept {
			if absInt(c.y-k.y) <= 8 && absFloat(c.shift-k.shift) <= 2.5 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			kept = append(kept, c)
		}
		if len(kept) >= 4 {
			break
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].y < kept[j].y })
	return kept
}

func seamEvidence(t Tear) []string {
	e := []string{"coherent_horizontal_shift", "cross_block_agreement"}
	if t.Coverage >= 0.65 {
		e = append(e, "wide_spatial_support")
	}
	if t.Mismatch >= 0.35 {
		e = append(e, "zero_shift_mismatch")
	}
	if t.Confidence >= 0.75 {
		e = append(e, "high_confidence_alignment")
	}
	return e
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func absFloat(v float64) float64 { return math.Abs(v) }

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }
