package vdetect

import (
	"math"
	"sort"
)

type fieldRowVote struct {
	y          int
	d          int
	confidence float64
	blocks     int
	coverage   float64
}

// detectFieldOrder finds widespread alternating parity displacement (combing).
func detectFieldOrder(f *Frame, opt Options) *FieldOrder {
	if f.W < 64 || f.H < 48 {
		return nil
	}
	maxD := clampInt(int(math.Round(float64(f.W)*0.014)), 4, 22)
	blockW := clampInt(f.W/24, 32, 96)
	step := maxInt(16, blockW*2/3)
	sensitivity := clamp(opt.Sensitivity, 0.1, 1)

	var rows []fieldRowVote
	texturedRows := 0
	for y := 3; y+3 < f.H; y += 2 {
		even := fieldSignature(f, y)
		odd := fieldSignature(f, y+1)
		votes := fieldBlockVotes(even, odd, f.W, y, maxD, blockW, step, sensitivity)
		if len(votes) < 3 {
			continue
		}
		texturedRows++
		if rv, ok := aggregateFieldRow(votes, f.W, maxD, sensitivity); ok {
			rv.y = y
			rows = append(rows, rv)
		}
	}

	if len(rows) < maxInt(10, f.H/32) {
		return &FieldOrder{Detected: false, TexturedRows: texturedRows, SupportedRows: len(rows)}
	}

	ds := make([]int, len(rows))
	var absSum, confSum float64
	for i, r := range rows {
		ds[i] = r.d
		absSum += math.Abs(float64(r.d)) * r.confidence
		confSum += r.confidence
	}
	sort.Ints(ds)
	medianD := ds[len(ds)/2]
	consistent := 0
	signPositive, signNegative := 0, 0
	for _, r := range rows {
		if absInt(r.d-medianD) <= 2 {
			consistent++
		}
		if r.d > 0 {
			signPositive++
		} else if r.d < 0 {
			signNegative++
		}
	}
	signRatio := float64(maxInt(signPositive, signNegative)) / float64(len(rows))
	consistency := float64(consistent) / float64(len(rows))
	rowSupport := float64(len(rows)) / float64(maxInt(1, texturedRows))
	verticalSupport := clamp01(float64(len(rows)) / float64(f.H/3))
	shift := absSum / math.Max(confSum, 1)

	conf := 0.12 + 0.25*clamp01(rowSupport/0.45) + 0.22*verticalSupport +
		0.20*consistency + 0.13*signRatio + 0.08*clamp01(shift/3)
	conf = clamp01(conf * (0.85 + 0.25*sensitivity))
	detected := rowSupport >= 0.24 && len(rows) >= maxInt(12, f.H/28) &&
		(consistency >= 0.42 || signRatio >= 0.72) && shift >= 0.8 && conf >= 0.48

	fo := &FieldOrder{
		Detected:      detected,
		Confidence:    round3(conf),
		ParityShiftPx: round2(shift),
		CombScore:     round3(rowSupport * consistency),
		RowSupport:    round3(rowSupport),
		SupportedRows: len(rows),
		TexturedRows:  texturedRows,
	}
	if detected {
		fo.Evidence = []string{"alternating_field_displacement", "widespread_comb_edges", "parity_block_agreement"}
		if consistency > 0.7 {
			fo.Evidence = append(fo.Evidence, "consistent_motion_vector")
		}
	}
	return fo
}

func fieldSignature(f *Frame, centerY int) []int16 {
	w, h := f.W, f.H
	sig := make([]int16, w)
	for x := 1; x < w-1; x++ {
		var sum, n int
		for _, dy := range []int{-2, 0, 2} {
			y := centerY + dy
			if y >= 0 && y < h {
				sum += int(f.Y[y*w+x+1]) - int(f.Y[y*w+x-1])
				n++
			}
		}
		if n > 0 {
			sig[x] = int16(sum / n)
		}
	}
	return sig
}

type fieldVote struct {
	x0, x1 int
	d      int
	weight float64
}

func fieldBlockVotes(a, b []int16, w, y, maxD, blockW, step int, sensitivity float64) []fieldVote {
	_ = y
	textureFloor := 5.0 - 2.0*sensitivity
	var votes []fieldVote
	for x0 := 4; x0+blockW < w-4; x0 += step {
		x1 := x0 + blockW
		var texture, zero float64
		for x := x0; x < x1; x++ {
			texture += (math.Abs(float64(a[x])) + math.Abs(float64(b[x]))) / 2
			zero += math.Abs(float64(a[x]) - float64(b[x]))
		}
		texture /= float64(blockW)
		zero /= float64(blockW)
		if texture < textureFloor {
			continue
		}
		bestD, bestCost := 0, math.Inf(1)
		secondCost := math.Inf(1)
		for d := -maxD; d <= maxD; d++ {
			var sum float64
			n := 0
			for x := x0; x < x1; x++ {
				bx := x + d
				if bx < 1 || bx >= w-1 {
					continue
				}
				sum += math.Abs(float64(a[x]) - float64(b[bx]))
				n++
			}
			cost := sum / float64(maxInt(1, n))
			if cost < bestCost {
				secondCost = bestCost
				bestCost, bestD = cost, d
			} else if cost < secondCost {
				secondCost = cost
			}
		}
		improvement := (zero - bestCost) / math.Max(zero, texture*1.3)
		relative := bestCost / math.Max(texture, 3)
		margin := (secondCost - bestCost) / math.Max(secondCost, texture)
		weight := clamp01((texture-textureFloor)/(textureFloor+8)) *
			clamp01(improvement*1.8) * clamp01((0.95-relative)*2) * clamp01(margin*2+0.3)
		if absInt(bestD) < 1 || weight < 0.14 || improvement < 0.14 || relative > 1.05 {
			continue
		}
		votes = append(votes, fieldVote{x0: x0 - step/3, x1: x1 + step/3, d: bestD, weight: weight})
	}
	return votes
}

func aggregateFieldRow(votes []fieldVote, width, maxD int, sensitivity float64) (fieldRowVote, bool) {
	best := fieldRowVote{}
	bestConf := 0.0
	for d := -maxD; d <= maxD; d++ {
		var weight float64
		var support []fieldVote
		intervals := make([][2]int, 0)
		for _, v := range votes {
			if absInt(v.d-d) <= 1 {
				support = append(support, v)
				weight += v.weight
				intervals = append(intervals, [2]int{v.x0, v.x1})
			}
		}
		if len(support) < 3 {
			continue
		}
		_, _, coverage := unionCoverage(intervals, width)
		if coverage < 0.16+0.08*(1-sensitivity) {
			continue
		}
		conf := clamp01(0.25*coverage + 0.35*clamp01(weight/float64(len(votes)+2)) +
			0.25*clamp01(float64(len(support))/8) + 0.15)
		if conf > bestConf {
			bestConf = conf
			best = fieldRowVote{d: d, confidence: round3(conf), blocks: len(support), coverage: round3(coverage)}
		}
	}
	if bestConf < 0.42 {
		return fieldRowVote{}, false
	}
	return best, true
}
