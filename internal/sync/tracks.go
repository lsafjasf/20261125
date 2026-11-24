package sync

import (
	"fmt"
	"sort"
)

// BuildTracks groups normalized points by stage and makes frame ordering explicit.
// If a stage has no frame numbers, records are ordered by wall time/input order
// and assigned zero-based ordinal frame numbers.
func BuildTracks(points []Point) (map[Stage][]Point, error) {
	grouped := make(map[Stage][]Point)
	for _, point := range points {
		grouped[point.Stage] = append(grouped[point.Stage], point)
	}

	for stage, track := range grouped {
		framed := 0
		for _, point := range track {
			if point.HasFrame {
				framed++
			}
		}
		if framed > 0 && framed < len(track) {
			return nil, fmt.Errorf("stage %s mixes framed and frameless records", stage)
		}

		sort.SliceStable(track, func(i, j int) bool {
			if framed == 0 {
				if track[i].WallNS != track[j].WallNS {
					return track[i].WallNS < track[j].WallNS
				}
				return false
			}
			if track[i].Frame != track[j].Frame {
				return track[i].Frame < track[j].Frame
			}
			return track[i].WallNS < track[j].WallNS
		})

		if framed == 0 {
			for i := range track {
				track[i].Frame = int64(i)
				track[i].HasFrame = true
			}
		}
		grouped[stage] = track
	}
	return grouped, nil
}

// BuildPositionRows pivots stage tracks into one row per correlated frame.
func BuildPositionRows(tracks map[Stage][]Point) []PositionRow {
	byFrame := make(map[int64]*PositionRow)

	for _, stage := range Stages {
		for _, point := range tracks[stage] {
			row := byFrame[point.Frame]
			if row == nil {
				row = &PositionRow{
					Frame:     point.Frame,
					Positions: make(map[Stage]int64),
					Video:     make(map[Stage]int64),
					Audio:     make(map[Stage]int64),
					LipOffset: make(map[Stage]int64),
				}
				byFrame[point.Frame] = row
			}
			if point.OutputFrame != nil && row.OutputFrame == nil {
				outputFrame := *point.OutputFrame
				row.OutputFrame = &outputFrame
			}
			if point.PositionNS != nil {
				row.Positions[stage] = *point.PositionNS
			}
			if point.VideoNS != nil {
				row.Video[stage] = *point.VideoNS
			}
			if point.AudioNS != nil {
				row.Audio[stage] = *point.AudioNS
			}
			if point.VideoNS != nil && point.AudioNS != nil {
				row.LipOffset[stage] = *point.AudioNS - *point.VideoNS
			}
		}
	}

	rows := make([]PositionRow, 0, len(byFrame))
	for _, row := range byFrame {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Frame < rows[j].Frame })
	return rows
}

func stageRank(stage Stage) int {
	for i, candidate := range Stages {
		if candidate == stage {
			return i
		}
	}
	return len(Stages)
}

func sortAnomalies(anomalies []Anomaly) {
	sort.SliceStable(anomalies, func(i, j int) bool {
		if anomalies[i].Frame != anomalies[j].Frame {
			return anomalies[i].Frame < anomalies[j].Frame
		}
		if stageRank(anomalies[i].Stage) != stageRank(anomalies[j].Stage) {
			return stageRank(anomalies[i].Stage) < stageRank(anomalies[j].Stage)
		}
		return anomalies[i].Series < anomalies[j].Series
	})
}

func evidenceWindow(rows []PositionRow, frame int64, context int) []PositionRow {
	if len(rows) == 0 {
		return nil
	}
	center := sort.Search(len(rows), func(i int) bool { return rows[i].Frame >= frame })
	if center == len(rows) {
		center = len(rows) - 1
	}
	start := maxInt(0, center-context)
	end := minInt(len(rows), center+context+1)
	return append([]PositionRow(nil), rows[start:end]...)
}

func baselineAndTailRows(rows []PositionRow, context int) []PositionRow {
	if len(rows) == 0 {
		return nil
	}
	count := context + 1
	if count > len(rows) {
		count = len(rows)
	}
	selected := make(map[int]struct{})
	for i := 0; i < count; i++ {
		selected[i] = struct{}{}
	}
	for i := len(rows) - count; i < len(rows); i++ {
		selected[i] = struct{}{}
	}
	indexes := make([]int, 0, len(selected))
	for i := range selected {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	result := make([]PositionRow, 0, len(indexes))
	for _, i := range indexes {
		result = append(result, rows[i])
	}
	return result
}
