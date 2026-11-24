package sync

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

func parseWideCSVRecords(header []string, cr *csv.Reader) ([]Record, error) {
	columns := make(map[string]int, len(header))
	for i, name := range header {
		columns[strings.ToLower(strings.TrimSpace(name))] = i
	}

	var records []Record
	for rowNumber := 2; ; rowNumber++ {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read wide CSV row %d: %w", rowNumber, err)
		}
		get := func(name string) string {
			if idx, ok := columns[name]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}

		var framePtr *int64
		if raw := get("frame"); raw != "" {
			frame, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("wide CSV row %d: frame: %w", rowNumber, err)
			}
			framePtr = &frame
		}
		var outputPtr *int64
		if raw := get("output_frame"); raw != "" {
			outputFrame, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("wide CSV row %d: output_frame: %w", rowNumber, err)
			}
			outputPtr = &outputFrame
		}
		var wallNS int64
		if raw := get("wall_ns"); raw != "" {
			wallNS, err = strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("wide CSV row %d: wall_ns: %w", rowNumber, err)
			}
		} else if raw := get("wall_time"); raw != "" {
			parsed, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				return nil, fmt.Errorf("wide CSV row %d: wall_time: %w", rowNumber, err)
			}
			wallNS = parsed.UnixNano()
		}

		for _, stage := range Stages {
			prefix := string(stage)
			position, err := parseNSField(get(prefix+"_position_ns"), get(prefix+"_position_ms"))
			if err != nil {
				return nil, fmt.Errorf("wide CSV row %d: %s position: %w", rowNumber, stage, err)
			}
			video, err := parseNSField(get(prefix+"_video_ns"), get(prefix+"_video_ms"))
			if err != nil {
				return nil, fmt.Errorf("wide CSV row %d: %s video: %w", rowNumber, stage, err)
			}
			audio, err := parseNSField(get(prefix+"_audio_ns"), get(prefix+"_audio_ms"))
			if err != nil {
				return nil, fmt.Errorf("wide CSV row %d: %s audio: %w", rowNumber, stage, err)
			}
			if position == nil && video == nil && audio == nil {
				continue
			}
			records = append(records, Record{
				Stage:       stage,
				Frame:       framePtr,
				OutputFrame: outputPtr,
				WallNS:      wallNS,
				PositionNS:  position,
				VideoPosNS:  video,
				AudioPosNS:  audio,
			})
		}
	}
	return records, nil
}

func parseNSField(nsRaw, msRaw string) (*int64, error) {
	if nsRaw != "" {
		value, err := strconv.ParseInt(nsRaw, 10, 64)
		if err != nil {
			return nil, err
		}
		return &value, nil
	}
	if msRaw != "" {
		value, err := strconv.ParseFloat(msRaw, 64)
		if err != nil {
			return nil, err
		}
		ns := int64(value * float64(time.Millisecond))
		return &ns, nil
	}
	return nil, nil
}
