package aes3

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// UnmarshalJSON accepts a bare timestamp, [time, level], or an object using
// common field names from logic-analyser and RTL dumps.
func (e *Edge) UnmarshalJSON(data []byte) error {
	var timestamp float64
	if err := json.Unmarshal(data, &timestamp); err == nil {
		e.Time = timestamp
		e.Level = LevelUnknown
		return nil
	}

	var pair []json.RawMessage
	if err := json.Unmarshal(data, &pair); err == nil && len(pair) >= 1 {
		if err := json.Unmarshal(pair[0], &e.Time); err != nil {
			return fmt.Errorf("aes3: invalid edge timestamp: %w", err)
		}
		e.Level = LevelUnknown
		if len(pair) >= 2 {
			level, err := parseJSONLevel(pair[1])
			if err != nil {
				return err
			}
			e.Level = level
		}
		return nil
	}

	var obj struct {
		Time      *float64        `json:"time"`
		T         *float64        `json:"t"`
		At        *float64        `json:"at"`
		Timestamp *float64        `json:"timestamp"`
		Level     json.RawMessage `json:"level"`
		Value     json.RawMessage `json:"value"`
		State     json.RawMessage `json:"state"`
		Direction json.RawMessage `json:"direction"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("aes3: edge must be number, pair, or object: %w", err)
	}
	switch {
	case obj.Time != nil:
		e.Time = *obj.Time
	case obj.T != nil:
		e.Time = *obj.T
	case obj.At != nil:
		e.Time = *obj.At
	case obj.Timestamp != nil:
		e.Time = *obj.Timestamp
	default:
		return errors.New("aes3: edge object has no time/timestamp field")
	}
	e.Level = LevelUnknown
	for _, raw := range []json.RawMessage{obj.Level, obj.Value, obj.State, obj.Direction} {
		if len(raw) == 0 {
			continue
		}
		if level, err := parseJSONLevel(raw); err == nil {
			e.Level = level
			break
		}
	}
	return nil
}

func parseJSONLevel(raw json.RawMessage) (Level, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return LevelUnknown, nil
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		if number == 0 {
			return LevelLow, nil
		}
		if number == 1 {
			return LevelHigh, nil
		}
		return LevelUnknown, nil
	}
	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		if boolean {
			return LevelHigh, nil
		}
		return LevelLow, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return parseTextLevel(text)
	}
	return LevelUnknown, errors.New("aes3: invalid edge level")
}

func parseTextLevel(text string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "0", "low", "l", "falling", "fall", "f", "false":
		return LevelLow, nil
	case "1", "high", "h", "rising", "rise", "r", "true":
		return LevelHigh, nil
	case "unknown", "x", "":
		return LevelUnknown, nil
	default:
		return LevelUnknown, fmt.Errorf("aes3: unknown level %q", text)
	}
}

type jsonEnvelope struct {
	Edges         json.RawMessage `json:"edges"`
	Transitions   json.RawMessage `json:"transitions"`
	EdgeTimes     json.RawMessage `json:"edge_times"`
	Timestamps    json.RawMessage `json:"timestamps"`
	Deltas        json.RawMessage `json:"deltas"`
	Intervals     json.RawMessage `json:"intervals"`
	EdgeIntervals json.RawMessage `json:"edge_intervals"`
	Data          json.RawMessage `json:"data"`
}

// EdgesFromIntervals converts successive edge-to-edge intervals into
// timestamps. The first edge is placed at time zero.
func EdgesFromIntervals(intervals []float64) []Edge {
	edges := make([]Edge, 0, len(intervals)+1)
	total := 0.0
	edges = append(edges, Edge{Time: 0, Level: LevelUnknown})
	for _, delta := range intervals {
		total += delta
		edges = append(edges, Edge{Time: total, Level: LevelUnknown})
	}
	return edges
}

// ParseEdgesJSON parses a bare edge array or an object containing an edges,
// transitions, edge_times, timestamps, or data array.
func ParseEdgesJSON(r io.Reader) ([]Edge, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, errors.New("aes3: empty JSON input")
	}
	if strings.HasPrefix(trimmed, "[") {
		var edges []Edge
		if err := json.Unmarshal(raw, &edges); err != nil {
			return nil, err
		}
		return edges, nil
	}

	var env jsonEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	for _, field := range []json.RawMessage{env.Edges, env.Transitions, env.EdgeTimes, env.Timestamps, env.Data} {
		if len(field) == 0 {
			continue
		}
		var edges []Edge
		if err := json.Unmarshal(field, &edges); err == nil {
			return edges, nil
		}
		var times []float64
		if err := json.Unmarshal(field, &times); err == nil {
			edges = make([]Edge, len(times))
			for i, t := range times {
				edges[i] = Edge{Time: t, Level: LevelUnknown}
			}
			return edges, nil
		}
	}
	for _, field := range []json.RawMessage{env.Deltas, env.Intervals, env.EdgeIntervals} {
		if len(field) == 0 {
			continue
		}
		var intervals []float64
		if err := json.Unmarshal(field, &intervals); err == nil {
			return EdgesFromIntervals(intervals), nil
		}
	}
	return nil, errors.New("aes3: JSON object does not contain a recognised edge array")
}

// ParseEdgesJSONLines parses newline-delimited edge objects.
func ParseEdgesJSONLines(r io.Reader) ([]Edge, error) {
	decoder := json.NewDecoder(r)
	var edges []Edge
	for decoder.More() {
		var e Edge
		if err := decoder.Decode(&e); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	if len(edges) == 0 {
		return nil, errors.New("aes3: no JSON lines edges found")
	}
	return edges, nil
}

// ParseEdgesCSV reads time[,level] CSV. A header is recognised when its first
// field is not numeric.
func ParseEdgesCSV(r io.Reader) ([]Edge, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	records, err := readTableRecords(raw)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("aes3: empty CSV input")
	}

	timeCol, levelCol := 0, 1
	start := 0
	if !looksNumeric(records[0][0]) {
		start = 1
		timeCol, levelCol = csvColumns(records[0])
	}

	edges := make([]Edge, 0, len(records)-start)
	for row := start; row < len(records); row++ {
		record := records[row]
		if len(record) == 0 || strings.TrimSpace(strings.Join(record, "")) == "" {
			continue
		}
		if timeCol >= len(record) {
			return nil, fmt.Errorf("aes3: CSV row %d has no time column", row+1)
		}
		timestamp, err := parseTimeValue(strings.TrimSpace(record[timeCol]))
		if err != nil {
			return nil, fmt.Errorf("aes3: CSV row %d: %w", row+1, err)
		}
		edge := Edge{Time: timestamp, Level: LevelUnknown}
		if levelCol >= 0 && levelCol < len(record) && strings.TrimSpace(record[levelCol]) != "" {
			level, err := parseTextLevel(record[levelCol])
			if err != nil {
				return nil, fmt.Errorf("aes3: CSV row %d: %w", row+1, err)
			}
			edge.Level = level
		}
		edges = append(edges, edge)
	}
	if len(edges) == 0 {
		return nil, errors.New("aes3: CSV contains no edge rows")
	}
	return edges, nil
}

func readTableRecords(raw []byte) ([][]string, error) {
	firstLine := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			firstLine = line
			break
		}
	}

	if strings.Contains(firstLine, ",") || strings.Contains(firstLine, "\t") {
		reader := csv.NewReader(strings.NewReader(string(raw)))
		reader.FieldsPerRecord = -1
		if strings.Contains(firstLine, "\t") && !strings.Contains(firstLine, ",") {
			reader.Comma = '\t'
		}
		return reader.ReadAll()
	}

	var records [][]string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		records = append(records, strings.Fields(line))
	}
	return records, nil
}

func csvColumns(header []string) (int, int) {
	timeCol, levelCol := 0, -1
	for i, name := range header {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "time", "timestamp", "t", "at", "edge_time", "time_s", "seconds":
			timeCol = i
		case "level", "value", "state", "logic", "direction":
			levelCol = i
		}
	}
	if levelCol == -1 && len(header) > 1 {
		levelCol = 1
	}
	return timeCol, levelCol
}

func looksNumeric(value string) bool {
	_, err := parseTimeValue(value)
	return err == nil
}

func parseTimeValue(value string) (float64, error) {
	if d, err := time.ParseDuration(value); err == nil {
		return d.Seconds(), nil
	}
	value = strings.TrimSuffix(value, "s")
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid time %q", value)
	}
	return v, nil
}
