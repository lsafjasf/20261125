package main

import (
	"fmt"
	"sort"
	"strings"
)

const (
	reasonNormal          = "normal"
	reasonRepeatPTS       = "repeat_pts"
	reasonLongDisplay     = "long_display"
	reasonDecodeGap       = "decode_gap"
	reasonZeroDecodeDelta = "zero_decode_delta"
	reasonZeroDisplay     = "zero_display_duration"
	reasonMissingCTTS     = "ctts_missing"
)

var reasonOrder = map[string]int{
	reasonRepeatPTS:       0,
	reasonLongDisplay:     1,
	reasonDecodeGap:       2,
	reasonZeroDecodeDelta: 3,
	reasonMissingCTTS:     4,
	reasonZeroDisplay:     5,
	reasonNormal:          6,
}

type timelineOptions struct {
	LongFactor      float64
	NominalOverride int64
}

type frame struct {
	Sample         int     `json:"sample"`
	PTS            int64   `json:"pts"`
	DTS            int64   `json:"dts"`
	PTSSeconds     float64 `json:"pts_seconds"`
	DTSSeconds     float64 `json:"dts_seconds"`
	DisplayStart   int64   `json:"display_start"`
	DisplayEnd     int64   `json:"display_end"`
	DisplaySeconds float64 `json:"display_seconds"`
	StartSeconds   float64 `json:"display_start_seconds"`
	EndSeconds     float64 `json:"display_end_seconds"`
	DisplayOrder   int     `json:"display_order"`
	Sync           bool    `json:"sync"`
	Reason         string  `json:"reason"`
	Detail         string  `json:"detail,omitempty"`
}

type trackTimeline struct {
	TrackID        uint32  `json:"track_id"`
	Codec          string  `json:"codec,omitempty"`
	Timescale      uint32  `json:"timescale"`
	SampleCount    int     `json:"sample_count"`
	NominalTicks   int64   `json:"nominal_duration_ticks"`
	NominalSeconds float64 `json:"nominal_duration_seconds"`
	LongFactor     float64 `json:"long_factor"`
	Frames         []frame `json:"frames"`
	Issues         []issue `json:"issues,omitempty"`
}

func analyzeTrack(tr *track, opts timelineOptions) trackTimeline {
	result := trackTimeline{
		TrackID:     tr.ID,
		Codec:       tr.Codec,
		Timescale:   tr.Timescale,
		SampleCount: tr.SampleCount,
		LongFactor:  opts.LongFactor,
		Issues:      append([]issue(nil), tr.Issues...),
	}
	nominal := opts.NominalOverride
	if nominal <= 0 {
		nominal = nominalDuration(tr.Durations)
	}
	result.NominalTicks = nominal
	result.NominalSeconds = tr.seconds(nominal)
	if nominal <= 0 {
		result.Issues = append(result.Issues, issue{
			Code:   "no_nominal_duration",
			Detail: "no positive stts duration is available; long-duration detection is disabled",
		})
	}

	pts, dts, ok := sampleTimes(tr, &result.Issues)
	if !ok {
		result.Frames = []frame{}
		return result
	}

	order := make([]int, len(pts))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if pts[a] != pts[b] {
			return pts[a] < pts[b]
		}
		if dts[a] != dts[b] {
			return dts[a] < dts[b]
		}
		return a < b
	})

	starts := make([]int64, len(pts))
	ends := make([]int64, len(pts))
	displayOrder := make([]int, len(pts))
	repeated := make([]bool, len(pts))
	repeatPeer := make([]int, len(pts))
	repeatCount := make([]int, len(pts))
	for i := range repeatPeer {
		repeatPeer[i] = -1
	}

	for pos := 0; pos < len(order); {
		groupEnd := pos + 1
		for groupEnd < len(order) && pts[order[groupEnd]] == pts[order[pos]] {
			groupEnd++
		}
		group := order[pos:groupEnd]
		winner := group[len(group)-1]
		for _, idx := range group {
			starts[idx] = pts[idx]
			displayOrder[idx] = pos + 1
			if idx != winner {
				ends[idx] = pts[idx]
			}
		}
		if groupEnd < len(order) {
			ends[winner] = pts[order[groupEnd]]
		} else {
			end, err := checkedAdd(pts[winner], int64(tr.Durations[winner]))
			if err != nil {
				end = pts[winner]
				result.Issues = append(result.Issues, issue{
					Code:   "timestamp_overflow",
					Detail: fmt.Sprintf("cannot derive end of final display interval: %v", err),
				})
			}
			ends[winner] = maxInt64(end, pts[winner])
		}
		if len(group) > 1 {
			for _, idx := range group {
				repeated[idx] = true
				repeatPeer[idx] = winner
				repeatCount[idx] = len(group)
			}
		}
		pos = groupEnd
	}

	cutoff := float64(nominal) * opts.LongFactor
	result.Frames = make([]frame, len(pts))
	for i := range pts {
		reasons := make([]string, 0, 4)
		details := make([]string, 0, 3)
		display := maxInt64(0, ends[i]-starts[i])
		duration := int64(tr.Durations[i])

		if repeated[i] {
			reasons = append(reasons, reasonRepeatPTS)
			peer := repeatPeer[i] + 1
			if i == repeatPeer[i] {
				details = append(details,
					fmt.Sprintf("PTS %d is shared by %d samples; last decode-order sample owns the display slot",
						pts[i], repeatCount[i]))
			} else {
				details = append(details,
					fmt.Sprintf("PTS %d is superseded by sample %d; this sample has no independent display interval",
						pts[i], peer))
			}
		}
		if nominal > 0 && float64(display) > cutoff {
			reasons = append(reasons, reasonLongDisplay)
			details = append(details,
				fmt.Sprintf("display interval %d ticks exceeds %.2fx nominal (%d ticks)",
					display, opts.LongFactor, nominal))
		}
		if nominal > 0 && float64(duration) > cutoff {
			reasons = append(reasons, reasonDecodeGap)
			details = append(details,
				fmt.Sprintf("stts delta %d ticks exceeds %.2fx nominal (%d ticks)",
					duration, opts.LongFactor, nominal))
		}
		if duration == 0 {
			reasons = append(reasons, reasonZeroDecodeDelta)
			details = append(details, "stts delta is zero; DTS does not advance for this sample")
		}
		if !tr.OffsetPresent[i] {
			reasons = append(reasons, reasonMissingCTTS)
			details = append(details, "no ctts entry covers this sample; PTS falls back to DTS")
		}
		if display == 0 && !repeated[i] {
			reasons = append(reasons, reasonZeroDisplay)
			details = append(details, "presentation interval has zero length")
		}
		if len(reasons) == 0 {
			reasons = append(reasons, reasonNormal)
		}
		sort.SliceStable(reasons, func(a, b int) bool {
			return reasonOrder[reasons[a]] < reasonOrder[reasons[b]]
		})

		result.Frames[i] = frame{
			Sample:         i + 1,
			PTS:            pts[i],
			DTS:            dts[i],
			PTSSeconds:     tr.seconds(pts[i]),
			DTSSeconds:     tr.seconds(dts[i]),
			DisplayStart:   starts[i],
			DisplayEnd:     ends[i],
			DisplaySeconds: tr.seconds(display),
			StartSeconds:   tr.seconds(starts[i]),
			EndSeconds:     tr.seconds(ends[i]),
			DisplayOrder:   displayOrder[i],
			Sync:           tr.Sync[i],
			Reason:         strings.Join(reasons, "|"),
			Detail:         strings.Join(details, "; "),
		}
	}
	return result
}

func sampleTimes(tr *track, issues *[]issue) ([]int64, []int64, bool) {
	pts := make([]int64, tr.SampleCount)
	dts := make([]int64, tr.SampleCount)
	var decode int64
	for i, duration := range tr.Durations {
		dts[i] = decode
		next, err := checkedAdd(decode, int64(duration))
		if err != nil {
			*issues = append(*issues, issue{
				Code:   "timestamp_overflow",
				Detail: fmt.Sprintf("DTS overflow at sample %d: %v", i+1, err),
			})
			return nil, nil, false
		}
		decode = next
		value, err := checkedAdd(dts[i], tr.Offsets[i])
		if err != nil {
			*issues = append(*issues, issue{
				Code:   "timestamp_overflow",
				Detail: fmt.Sprintf("PTS overflow at sample %d: %v", i+1, err),
			})
			return nil, nil, false
		}
		pts[i] = value
	}
	return pts, dts, true
}

func nominalDuration(durations []uint32) int64 {
	counts := make(map[uint32]int)
	for _, duration := range durations {
		if duration > 0 {
			counts[duration]++
		}
	}
	var best uint32
	bestCount := 0
	for duration, count := range counts {
		if count > bestCount || (count == bestCount && duration < best) {
			best, bestCount = duration, count
		}
	}
	return int64(best)
}
