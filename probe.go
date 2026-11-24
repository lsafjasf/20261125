package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type ffProbeOutput struct {
	Streams []ffProbeStream `json:"streams"`
	Format  *ffProbeFormat  `json:"format"`
}

type ffProbeFormat struct {
	Duration string `json:"duration"`
}

type ffProbeStream struct {
	Index              int               `json:"index"`
	CodecName          string            `json:"codec_name"`
	CodecType          string            `json:"codec_type"`
	Width              int               `json:"width"`
	Height             int               `json:"height"`
	CodedWidth         int               `json:"coded_width"`
	CodedHeight        int               `json:"coded_height"`
	SampleAspectRatio  string            `json:"sample_aspect_ratio"`
	DisplayAspectRatio string            `json:"display_aspect_ratio"`
	Duration           string            `json:"duration"`
	Tags               map[string]string `json:"tags"`
	SideDataList       []ffSideData      `json:"side_data_list"`
}

type ffSideData struct {
	SideDataType  string          `json:"side_data_type"`
	Rotation      *float64        `json:"rotation"`
	DisplayMatrix json.RawMessage `json:"displaymatrix"`
}

type ProbedVideo struct {
	StreamIndex     int
	CodecName       string
	Width           int
	Height          int
	CodedWidth      int
	CodedHeight     int
	SARString       string
	DARString       string
	DurationSeconds float64
	FFProbeRotation *float64
	FFProbeMatrix   *MatrixInfo
	LegacyRotateTag *int
	LegacyTagRaw    string
}

func runFFProbe(path, ffprobeBin string) (*ProbedVideo, error) {
	cmd := exec.Command(ffprobeBin, "-v", "error", "-show_streams", "-show_format", "-of", "json", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w (%s)", ffprobeBin, err, strings.TrimSpace(stderr.String()))
	}
	var parsed ffProbeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("parse ffprobe JSON: %w", err)
	}
	var stream *ffProbeStream
	for i := range parsed.Streams {
		if parsed.Streams[i].CodecType == "video" && parsed.Streams[i].Width > 0 {
			stream = &parsed.Streams[i]
			break
		}
	}
	if stream == nil {
		return nil, fmt.Errorf("no video stream with positive dimensions found")
	}

	probed := &ProbedVideo{
		StreamIndex: stream.Index,
		CodecName:   stream.CodecName,
		Width:       stream.Width,
		Height:      stream.Height,
		CodedWidth:  stream.CodedWidth,
		CodedHeight: stream.CodedHeight,
		SARString:   stream.SampleAspectRatio,
		DARString:   stream.DisplayAspectRatio,
	}
	if probed.CodedWidth == 0 {
		probed.CodedWidth = stream.Width
	}
	if probed.CodedHeight == 0 {
		probed.CodedHeight = stream.Height
	}
	if d, err := strconv.ParseFloat(stream.Duration, 64); err == nil && d > 0 {
		probed.DurationSeconds = d
	} else if parsed.Format != nil {
		if d, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil {
			probed.DurationSeconds = d
		}
	}

	if raw := findTag(stream.Tags, "rotate"); raw != "" {
		probed.LegacyTagRaw = raw
		if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			probed.LegacyRotateTag = &value
		}
	}

	for _, side := range stream.SideDataList {
		if strings.Contains(strings.ToLower(side.SideDataType), "display matrix") {
			if side.Rotation != nil {
				r := *side.Rotation
				probed.FFProbeRotation = &r
			}
			if rawMatrix, ok := parseFFProbeMatrix(side.DisplayMatrix); ok {
				matrix := matrixFromRaw(rawMatrix, "ffprobe-side-data")
				if side.Rotation != nil {
					r := *side.Rotation
					matrix.FFprobeRotation = &r
				}
				if probed.LegacyRotateTag != nil {
					v := *probed.LegacyRotateTag
					matrix.LegacyTagRotation = &v
				}
				probed.FFProbeMatrix = &matrix
			}
		}
	}
	return probed, nil
}

func findTag(tags map[string]string, wanted string) string {
	for key, value := range tags {
		if strings.EqualFold(key, wanted) {
			return value
		}
	}
	return ""
}

func parseFFProbeMatrix(raw json.RawMessage) ([9]int64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return [9]int64{}, false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		values := make([]int64, 0, 9)
		for _, line := range strings.Split(text, "\n") {
			if !strings.Contains(line, ":") {
				continue
			}
			right := strings.SplitN(line, ":", 2)[1]
			for _, field := range strings.Fields(right) {
				value, err := strconv.ParseInt(field, 10, 64)
				if err != nil {
					break
				}
				values = append(values, value)
				if len(values) == 9 {
					return array9(values), true
				}
			}
		}
		return [9]int64{}, false
	}

	var numbers []int64
	if err := json.Unmarshal(raw, &numbers); err == nil && len(numbers) >= 9 {
		return array9(numbers[:9]), true
	}
	var floats []float64
	if err := json.Unmarshal(raw, &floats); err == nil && len(floats) >= 9 {
		var values [9]int64
		for i := range values {
			values[i] = int64(floats[i])
		}
		return values, true
	}
	return [9]int64{}, false
}

func array9(values []int64) [9]int64 {
	var result [9]int64
	copy(result[:], values[:9])
	return result
}

func firstLine(path string, args ...string) string {
	cmd := exec.Command(path, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	line := strings.SplitN(string(out), "\n", 2)[0]
	return strings.TrimSpace(line)
}
