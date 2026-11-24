package aes3

import (
	"errors"
	"sort"
)

// Decode recovers AES3 subframes and loss-of-lock intervals from BMC edges.
func Decode(input []Edge, option ...Options) (*Report, error) {
	opt := Options{}
	if len(option) > 0 {
		opt = option[0]
	}
	opt = opt.normalized()

	edges := prepareEdges(input)
	if len(edges) == 0 {
		return nil, errors.New("aes3: no edges supplied")
	}
	for _, e := range edges {
		if !validTime(e.Time) {
			return nil, errors.New("aes3: edge times must be finite numbers")
		}
	}

	h, source, clockErr := recoverHalfUnit(edges, opt.HalfUnit, opt.JitterTolerance)

	d := &decoder{edges: edges, wf: newWaveform(edges), h: h, opt: opt}
	report := &Report{
		Clock: ClockReport{
			HalfBitTime: h,
			BitCellTime: 2 * h,
			BitRate:     bitRate(h),
			Source:      source,
		},
		HalfUnit:       h,
		HalfUnitSource: source,
		Edges:          len(edges),
		Subframes:      []Subframe{},
		LockIntervals:  []LockInterval{},
		ChannelStatus: ChannelStatusReport{
			Left:  []StatusSegment{},
			Right: []StatusSegment{},
		},
	}
	if clockErr == nil {
		candidates := d.findCandidates()
		chosen := selectCandidates(candidates, h)
		if len(chosen) > 0 {
			fillReport(report, edges, chosen, h, opt.JitterTolerance)
			return report, nil
		}
	}

	reason := "no valid X/Y/Z preamble and complete BMC subframe found"
	if clockErr != nil {
		report.HalfUnit = 0
		report.Clock.HalfBitTime = 0
		report.Clock.BitCellTime = 0
		report.Clock.BitRate = 0
		report.HalfUnitSource = "unresolved"
		report.Clock.Source = "unresolved"
		reason = "no recoverable BMC clock or valid X/Y/Z preamble"
	}
	report.LockIntervals = []LockInterval{{
		Index:     0,
		StartEdge: 0,
		EndEdge:   len(edges),
		StartTime: edges[0].Time,
		EndTime:   edges[len(edges)-1].Time,
		Duration:  edges[len(edges)-1].Time - edges[0].Time,
		Reason:    reason,
	}}
	report.Summary = buildSummary(edges, nil, report.LockIntervals)
	report.LossOfLock = report.LockIntervals
	return report, nil
}

func fillReport(report *Report, edges []Edge, chosen []candidate, h, tolerance float64) {
	subframes := make([]Subframe, len(chosen))
	for i, c := range chosen {
		subframes[i] = c.subframe
		subframes[i].Index = i
	}

	gapBefore := make([]bool, len(subframes))
	for i := 1; i < len(subframes); i++ {
		a := subframes[i-1]
		b := &subframes[i]
		delta := b.StartTime - a.StartTime
		if absFloat(delta-64*h) <= h {
			if !expectedNextChannel(a.Channel, b.Channel) {
				b.Trusted = false
				b.Errors = append(b.Errors, "unexpected left/right subframe sequence")
			}
			continue
		}

		gapBefore[i] = true
		startTime := a.StartTime + 64*a.HalfUnit
		endTime := b.StartTime
		startThreshold := startTime - tolerance*h - h*1e-9
		startEdge := sort.Search(len(edges), func(j int) bool {
			return edges[j].Time >= startThreshold
		})
		if startEdge > b.StartEdge {
			startEdge = b.StartEdge
		}
		edgeCount := b.StartEdge - startEdge
		reason := "invalid BMC timing or preamble; waiting for X/Y/Z relock"
		if edgeCount == 0 {
			reason = "no transitions in interval; clock absent until relock preamble"
		}
		missing := int(floatRound((b.StartTime-a.StartTime)/(64*h))) - 1
		if missing < 0 {
			missing = 0
		}
		report.LockIntervals = append(report.LockIntervals, LockInterval{
			Index:        len(report.LockIntervals),
			StartEdge:    startEdge,
			EndEdge:      b.StartEdge,
			StartTime:    startTime,
			EndTime:      endTime,
			Duration:     endTime - startTime,
			Reason:       reason,
			RelockCode:   b.Code,
			MissingSlots: missing,
		})
	}

	assignFrames(subframes, gapBefore)
	report.Subframes = subframes
	report.ChannelStatus = buildChannelStatus(subframes, gapBefore)
	report.Summary = buildSummary(edges, subframes, report.LockIntervals)
	report.LossOfLock = report.LockIntervals
}

func assignFrames(sfs []Subframe, gapBefore []bool) {
	group := -1
	for i := range sfs {
		if sfs[i].Channel == "left" {
			group++
			sfs[i].Frame = group
			continue
		}

		if i > 0 && !gapBefore[i] && sfs[i-1].Channel == "left" {
			sfs[i].Frame = sfs[i-1].Frame
			continue
		}

		group++
		sfs[i].Frame = group
	}
}

func buildSummary(edges []Edge, sfs []Subframe, intervals []LockInterval) Summary {
	s := Summary{EdgeCount: len(edges), Subframes: len(sfs), LockIntervals: len(intervals)}
	for _, sf := range sfs {
		switch sf.Channel {
		case "left":
			s.Left++
		case "right":
			s.Right++
		}
		if sf.BlockStart {
			s.BlockStarts++
		}
		if !sf.ParityOK {
			s.ParityErrors++
		}
		if !sf.Trusted {
			s.Untrusted++
		}
	}
	return s
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func bitRate(halfUnit float64) float64 {
	if halfUnit <= 0 {
		return 0
	}
	return 1 / (2 * halfUnit)
}

func floatRound(v float64) float64 {
	if v < 0 {
		return -floatRound(-v)
	}
	return float64(int64(v + 0.5))
}
