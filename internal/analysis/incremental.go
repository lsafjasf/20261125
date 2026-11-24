package analysis

import "sort"

type IntervalChange struct {
	Index   int      `json:"index"`
	Start   float64  `json:"start"`
	End     float64  `json:"end"`
	Score   float64  `json:"score"`
	Flagged bool     `json:"flagged"`
	Reasons []string `json:"reasons,omitempty"`
}

type Revision struct {
	Before              Stats            `json:"before"`
	After               Stats            `json:"after"`
	Threshold           ThresholdInfo    `json:"threshold"`
	RecomputedIntervals int              `json:"recomputedIntervals"`
	RecomputedRanges    []Range          `json:"recomputedRanges"`
	Changes             []IntervalChange `json:"changes"`
	Changed             int              `json:"changed"`
	Aligned             Alignment        `json:"alignment"`
}

// SetSensitivity only re-evaluates intervals whose immutable confidence score
// lies between the old and new cutoffs. All other interval decisions are reused.
func (a *Analysis) SetSensitivity(sensitivity float64) Revision {
	sensitivity = clamp(sensitivity, 0, 1)
	oldCutoff := a.cutoff
	newCutoff := DecisionCutoff(sensitivity)
	before := cloneStats(a.Stats)
	rev := Revision{
		Before:           before,
		Threshold:        ThresholdInfo{Sensitivity: round(sensitivity, 3), ConfidenceCutoff: round(newCutoff, 4)},
		Aligned:          a.Alignment,
		RecomputedRanges: []Range{},
		Changes:          []IntervalChange{},
	}
	if oldCutoff == newCutoff {
		rev.After = cloneStats(a.Stats)
		return rev
	}
	low := mathMin(oldCutoff, newCutoff)
	high := mathMax(oldCutoff, newCutoff)
	first := sort.Search(len(a.sortOrder), func(i int) bool {
		return a.Intervals[a.sortOrder[i]].Confidence >= low
	})
	var affected []int
	for pos := first; pos < len(a.sortOrder); pos++ {
		idx := a.sortOrder[pos]
		iv := &a.Intervals[idx]
		if iv.Confidence >= high {
			break
		}
		affected = append(affected, idx)
		newFlagged := iv.Confidence >= newCutoff
		if newFlagged != iv.Flagged {
			iv.Flagged = newFlagged
			rev.Changes = append(rev.Changes, IntervalChange{
				Index:   idx,
				Start:   iv.Start,
				End:     iv.End,
				Score:   iv.Confidence,
				Flagged: newFlagged,
				Reasons: append([]string(nil), iv.Reasons...),
			})
		}
	}
	sort.Ints(affected)
	rev.RecomputedIntervals = len(affected)
	rev.RecomputedRanges = mergeRanges(a.Intervals, affected)
	rev.Changed = len(rev.Changes)
	if rev.Changes == nil {
		rev.Changes = []IntervalChange{}
	}
	a.Sensitivity = round(sensitivity, 3)
	a.cutoff = newCutoff
	a.Threshold = rev.Threshold
	a.Stats = computeStats(a.Intervals, func(iv Interval) bool { return iv.Flagged }, a.Alignment)
	rev.After = cloneStats(a.Stats)
	return rev
}

func mergeRanges(intervals []Interval, indices []int) []Range {
	if len(indices) == 0 {
		return []Range{}
	}
	out := make([]Range, 0, 4)
	start := intervals[indices[0]].Start
	end := intervals[indices[0]].End
	for _, idx := range indices[1:] {
		next := intervals[idx]
		if next.Start <= end+1e-9 {
			if next.End > end {
				end = next.End
			}
			continue
		}
		out = append(out, Range{Start: start, End: end})
		start = next.Start
		end = next.End
	}
	out = append(out, Range{Start: start, End: end})
	return out
}

func cloneStats(in Stats) Stats {
	out := in
	out.Evidence = make(map[string]int, len(in.Evidence))
	for k, v := range in.Evidence {
		out.Evidence[k] = v
	}
	return out
}

func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
