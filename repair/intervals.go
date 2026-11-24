package repair

import "sort"

// buildIntervals groups contiguous complete samples into playable intervals.
// An interval starts at a sync sample (IDR); recovered samples before the
// first sync are indexed but not independently playable.
func buildIntervals(plan *Plan) {
	ss := plan.Samples
	if len(ss) == 0 {
		return
	}
	rep := plan.Report

	// Assign rebuilt-table numbers (1 based) and tick prefix sums.
	numByPos := make([]int, len(ss))
	tickBefore := make([]uint64, len(ss))
	var num int
	var ticks uint64
	for i := range ss {
		tickBefore[i] = ticks
		if ss[i].keep {
			num++
			numByPos[i] = num
			ticks += uint64(ss[i].duration)
		}
	}

	// Assign contiguity run ids over scanned samples.
	runOf := make([]int, len(ss))
	run := 0
	for i := 1; i < len(ss); i++ {
		if ss[i].srcOff != ss[i-1].srcOff+ss[i-1].size {
			run++
		}
		runOf[i] = run
	}

	type runRange struct{ first, last int }
	ranges := []runRange{}
	start := 0
	for i := 1; i <= len(ss); i++ {
		if i == len(ss) || runOf[i] != runOf[start] {
			ranges = append(ranges, runRange{start, i - 1})
			if i < len(ss) {
				start = i
			}
		}
	}

	ivIndex := 0
	for _, rr := range ranges {
		type kept struct {
			pos int
			n   int
		}
		var members []kept
		for i := rr.first; i <= rr.last; i++ {
			if ss[i].keep {
				members = append(members, kept{i, numByPos[i]})
			}
		}
		if len(members) == 0 {
			continue
		}

		firstKeyMember := -1
		var keys []int
		for mi, m := range members {
			if ss[m.pos].key {
				keys = append(keys, m.n)
				if firstKeyMember < 0 {
					firstKeyMember = mi
				}
			}
		}

		runStart := ss[rr.first].srcOff
		runEnd := ss[rr.last].srcOff + ss[rr.last].size

		if firstKeyMember < 0 {
			rep.Unrecovered = append(rep.Unrecovered, UnrecoveredRange{
				Kind: "no_sync_run", StartOffset: runStart, EndOffset: runEnd,
				Length: runEnd - runStart,
				Detail: "contiguous recovered run has no keyframe; not independently playable",
			})
			continue
		}

		ivIndex++
		fkPos := members[firstKeyMember].pos
		lastM := members[len(members)-1]
		firstM := members[0]
		startTicks := tickBefore[fkPos]
		endTicks := tickBefore[lastM.pos] + uint64(ss[lastM.pos].duration)
		ts := float64(plan.TimeScale)

		rep.Intervals = append(rep.Intervals, Interval{
			Index:                 ivIndex,
			FirstSample:           firstM.n,
			LastSample:            lastM.n,
			FirstSyncSample:       members[firstKeyMember].n,
			LeadingNonSyncSamples: members[firstKeyMember].n - firstM.n,
			Keyframes:             keys,
			StartTimeSeconds:      float64(startTicks) / ts,
			EndTimeSeconds:        float64(endTicks) / ts,
			StartOffset:           ss[fkPos].srcOff,
			EndOffset:             runEnd,
		})
	}

	sort.SliceStable(rep.Unrecovered, func(i, j int) bool {
		if rep.Unrecovered[i].StartOffset == 0 {
			return false
		}
		if rep.Unrecovered[j].StartOffset == 0 {
			return true
		}
		return rep.Unrecovered[i].StartOffset < rep.Unrecovered[j].StartOffset
	})
}
