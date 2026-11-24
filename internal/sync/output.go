package sync

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// WritePositionCSV writes the complete frame-by-stage media position table.
func WritePositionCSV(writer io.Writer, rows []PositionRow) error {
	cw := csv.NewWriter(writer)
	header := []string{"frame", "output_frame"}
	for _, stage := range Stages {
		name := string(stage)
		header = append(header,
			name+"_position_ns",
			name+"_video_ns",
			name+"_audio_ns",
			name+"_lip_offset_ns",
		)
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, row := range rows {
		record := []string{strconv.FormatInt(row.Frame, 10)}
		if row.OutputFrame != nil {
			record = append(record, strconv.FormatInt(*row.OutputFrame, 10))
		} else {
			record = append(record, "")
		}

		for _, stage := range Stages {
			record = append(record,
				formatIntValue(row.Positions[stage], row.Positions, stage),
				formatIntValue(row.Video[stage], row.Video, stage),
				formatIntValue(row.Audio[stage], row.Audio, stage),
				formatIntValue(row.LipOffset[stage], row.LipOffset, stage),
			)
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

func formatIntValue(value int64, values map[Stage]int64, stage Stage) string {
	if _, ok := values[stage]; !ok {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

// FormatMilliseconds is a small presentation helper for CLI output.
func FormatMilliseconds(ns int64) string {
	return fmt.Sprintf("%.3f", float64(ns)/1e6)
}
