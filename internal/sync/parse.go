package sync

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type jsonEnvelope struct {
	Records []Record `json:"records"`
	Events  []Record `json:"events"`
}

// ParseJSON accepts JSONL, a JSON array, or an object containing records/events.
func ParseJSON(data []byte) ([]Record, error) {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) == 0 {
		return nil, nil
	}

	if trimmed[0] == '[' {
		var records []Record
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, fmt.Errorf("parse JSON array: %w", err)
		}
		return records, nil
	}

	var envelope jsonEnvelope
	if err := json.Unmarshal(data, &envelope); err == nil {
		if len(envelope.Records) > 0 {
			return envelope.Records, nil
		}
		if len(envelope.Events) > 0 {
			return envelope.Events, nil
		}
	}

	var records []Record
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var record Record
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse JSONL record: %w", err)
		}
		records = append(records, record)
	}
	return records, nil
}

// ParseCSV reads a header-driven trace. Nanosecond and millisecond columns
// are both accepted to make instrumentation export easier.
func ParseCSV(reader io.Reader) ([]Record, error) {
	cr := csv.NewReader(reader)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	columns := make(map[string]int, len(header))
	for i, name := range header {
		columns[strings.ToLower(strings.TrimSpace(name))] = i
	}
	if _, hasStage := columns["stage"]; !hasStage {
		return parseWideCSVRecords(header, cr)
	}
	get := func(row []string, names ...string) string {
		for _, name := range names {
			if idx, ok := columns[name]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
		}
		return ""
	}
	parseIntPtr := func(raw string) (*int64, error) {
		if raw == "" {
			return nil, nil
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, err
		}
		return &value, nil
	}
	parseMSPtr := func(raw string) (*int64, error) {
		if raw == "" {
			return nil, nil
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, err
		}
		ns := int64(value * float64(time.Millisecond))
		return &ns, nil
	}

	var records []Record
	for rowNumber := 2; ; rowNumber++ {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row %d: %w", rowNumber, err)
		}

		record := Record{}
		record.Stage = Stage(get(row, "stage"))
		if record.Stage == "" {
			return nil, fmt.Errorf("CSV row %d: stage is required", rowNumber)
		}
		if raw := get(row, "frame"); raw != "" {
			frame, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("CSV row %d: frame: %w", rowNumber, err)
			}
			record.Frame = &frame
		}
		if raw := get(row, "output_frame", "out_frame"); raw != "" {
			frame, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("CSV row %d: output_frame: %w", rowNumber, err)
			}
			record.OutputFrame = &frame
		}
		if raw := get(row, "wall_ns"); raw != "" {
			record.WallNS, err = strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("CSV row %d: wall_ns: %w", rowNumber, err)
			}
		} else if raw := get(row, "wall_time"); raw != "" {
			parsed, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				return nil, fmt.Errorf("CSV row %d: wall_time: %w", rowNumber, err)
			}
			record.WallNS = parsed.UnixNano()
		}
		record.Stream = get(row, "stream")
		record.Note = get(row, "note")

		if raw := get(row, "position_ns", "pos_ns"); raw != "" {
			record.PositionNS, err = parseIntPtr(raw)
		} else if raw := get(row, "position_ms", "pos_ms"); raw != "" {
			record.PositionNS, err = parseMSPtr(raw)
		}
		if err != nil {
			return nil, fmt.Errorf("CSV row %d: position: %w", rowNumber, err)
		}

		if raw := get(row, "video_pos_ns", "video_ns"); raw != "" {
			record.VideoPosNS, err = parseIntPtr(raw)
		} else if raw := get(row, "video_pos_ms", "video_ms"); raw != "" {
			record.VideoPosNS, err = parseMSPtr(raw)
		}
		if err != nil {
			return nil, fmt.Errorf("CSV row %d: video position: %w", rowNumber, err)
		}

		if raw := get(row, "audio_pos_ns", "audio_ns"); raw != "" {
			record.AudioPosNS, err = parseIntPtr(raw)
		} else if raw := get(row, "audio_pos_ms", "audio_ms"); raw != "" {
			record.AudioPosNS, err = parseMSPtr(raw)
		}
		if err != nil {
			return nil, fmt.Errorf("CSV row %d: audio position: %w", rowNumber, err)
		}

		records = append(records, record)
	}
	return records, nil
}

// Normalize validates stages and pivots long-format stream records.
func Normalize(records []Record) ([]Point, error) {
	points := make([]Point, 0, len(records))
	for i, record := range records {
		stage, err := ParseStage(string(record.Stage))
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", i, err)
		}

		point := Point{
			Stage:       stage,
			OutputFrame: record.OutputFrame,
			WallNS:      record.WallNS,
			PositionNS:  record.PositionNS,
			VideoNS:     record.VideoPosNS,
			AudioNS:     record.AudioPosNS,
		}
		if record.Frame != nil {
			point.Frame = *record.Frame
			point.HasFrame = true
		} else if record.OutputFrame != nil {
			point.Frame = *record.OutputFrame
			point.HasFrame = true
		}

		switch strings.ToLower(record.Stream) {
		case "video", "v":
			point.VideoNS = record.PositionNS
			if point.PositionNS != nil && point.VideoNS == point.PositionNS {
				point.PositionNS = nil
			}
		case "audio", "a":
			point.AudioNS = record.PositionNS
			if point.PositionNS != nil && point.AudioNS == point.PositionNS {
				point.PositionNS = nil
			}
		case "", "main", "position":
		default:
			return nil, fmt.Errorf("record %d: unknown stream %q", i, record.Stream)
		}

		if point.PositionNS == nil && point.VideoNS != nil {
			value := *point.VideoNS
			point.PositionNS = &value
		}
		points = append(points, point)
	}
	return points, nil
}
