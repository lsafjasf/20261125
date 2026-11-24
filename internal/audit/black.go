package audit

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"cutmux/internal/media"
)

type frameEvidence struct {
	Summary BlackEvidence
	flags   []bool
}

type timeRange struct {
	start float64
	end   float64
}

func loadFrameEvidence(path string, video *media.Track) (frameEvidence, error) {
	if video == nil {
		return frameEvidence{Summary: BlackEvidence{Source: "none"}}, nil
	}
	ps := video.PresentedSamples()
	if path != "" {
		rows, err := readCSVRows(path)
		if err != nil {
			return frameEvidence{}, err
		}
		ev, err := evidenceFromRows(rows, ps)
		if err != nil {
			return frameEvidence{}, err
		}
		return ev, nil
	}
	return sizeCandidateEvidence(video, ps), nil
}

func readCSVRows(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	r.Comment = '#'
	return r.ReadAll()
}

func evidenceFromRows(rows [][]string, ps []media.PresentedSample) (frameEvidence, error) {
	out := frameEvidence{
		Summary: BlackEvidence{Source: "frame-sidecar", Definitive: true},
		flags:   make([]bool, len(ps)),
	}
	if len(ps) == 0 {
		out.Summary.Warning = "video track has no presented samples"
		return out, nil
	}
	mode := ""
	var intervals []timeRange
	frameRows := 0
	for rowNo, row := range rows {
		row = trimRow(row)
		if len(row) == 0 {
			continue
		}
		if rowNo == 0 || mode == "" {
			lower := strings.ToLower(strings.Join(row, ","))
			if strings.Contains(lower, "start") && strings.Contains(lower, "end") {
				mode = "interval"
				continue
			}
			if strings.Contains(lower, "black") || strings.Contains(lower, "pts") || strings.Contains(lower, "time") {
				mode = "frame"
				continue
			}
		}
		if mode == "interval" {
			if len(row) < 2 {
				return out, fmt.Errorf("%s: interval row needs start,end", "frame sidecar")
			}
			a, err1 := strconv.ParseFloat(row[0], 64)
			b, err2 := strconv.ParseFloat(row[1], 64)
			if err1 != nil || err2 != nil {
				return out, fmt.Errorf("frame sidecar: invalid interval %q", strings.Join(row, ","))
			}
			if b < a {
				a, b = b, a
			}
			intervals = append(intervals, timeRange{start: a, end: b})
			continue
		}
		if len(row) >= 2 {
			if black, ok := parseBlack(row[1]); ok {
				if mode == "" {
					mode = "frame"
				}
				if mode == "frame" {
					t, err := strconv.ParseFloat(row[0], 64)
					if err != nil {
						return out, fmt.Errorf("frame sidecar: invalid timestamp %q", row[0])
					}
					if black {
						intervals = append(intervals, timeRange{start: t, end: t})
					}
					frameRows++
					continue
				}
			}
		}
		if len(row) >= 2 {
			a, err1 := strconv.ParseFloat(row[0], 64)
			b, err2 := strconv.ParseFloat(row[1], 64)
			if err1 == nil && err2 == nil {
				if mode == "" {
					mode = "interval"
				}
				if b < a {
					a, b = b, a
				}
				intervals = append(intervals, timeRange{start: a, end: b})
				continue
			}
		}
		return out, fmt.Errorf("frame sidecar: cannot parse row %q", strings.Join(row, ","))
	}
	if mode == "" {
		out.Summary.Warning = "frame sidecar is empty"
		return out, nil
	}
	frameDur := float64(videoFrameDuration(ps)) / 1e9
	tolerance := frameDur * 0.75
	if tolerance < 0.010 {
		tolerance = 0.010
	}
	for i, s := range ps {
		t := float64(s.Start) / 1e9
		for _, r := range intervals {
			if r.start == r.end && r.start == t {
				// A raw per-frame row was stored with start == end.
			}
			if r.start == r.end {
				if absFloat(t-r.start) <= tolerance {
					out.flags[i] = true
					break
				}
				continue
			}
			if t+1e-9 >= r.start && t < r.end {
				out.flags[i] = true
				break
			}
		}
	}
	if frameRows > 0 && frameRows < len(ps)/2 {
		out.Summary.Warning = fmt.Sprintf(
			"sidecar has %d frame rows for %d presented frames; timestamps may be filtered",
			frameRows, len(ps))
	}
	for _, flag := range out.flags {
		if flag {
			out.Summary.Matched++
		}
	}
	out.Summary.Total = len(ps)
	out.Summary.Runs = runsFromFlags(out.flags, ps)
	return out, nil
}

func trimRow(row []string) []string {
	out := row[:0]
	for _, v := range row {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func parseBlack(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "black":
		return true, true
	case "0", "false", "no", "n", "normal":
		return false, true
	}
	return false, false
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func videoFrameDuration(ps []media.PresentedSample) int64 {
	if len(ps) < 2 {
		return 33_333_333
	}
	deltas := make([]int64, 0, len(ps)-1)
	for i := 1; i < len(ps); i++ {
		if d := ps[i].Start - ps[i-1].Start; d > 0 {
			deltas = append(deltas, d)
		}
	}
	if len(deltas) == 0 {
		return 33_333_333
	}
	sort.Slice(deltas, func(i, j int) bool { return deltas[i] < deltas[j] })
	return deltas[len(deltas)/2]
}

func runsFromFlags(flags []bool, ps []media.PresentedSample) []BlackRun {
	var runs []BlackRun
	for i := 0; i < len(flags); {
		if !flags[i] {
			i++
			continue
		}
		j := i + 1
		for j < len(flags) && flags[j] {
			j++
		}
		runs = append(runs, BlackRun{
			StartMs: float64(ps[i].Start) / 1e6,
			EndMs:   float64(ps[j-1].End) / 1e6,
			Frames:  j - i,
		})
		i = j
	}
	return runs
}

func sizeCandidateEvidence(video *media.Track, ps []media.PresentedSample) frameEvidence {
	out := frameEvidence{
		Summary: BlackEvidence{Source: "none", Total: len(ps)},
		flags:   make([]bool, len(ps)),
	}
	if len(ps) == 0 {
		return out
	}
	var sizes []int64
	for _, p := range ps {
		if p.DecodeIndex < 0 || p.DecodeIndex >= len(video.Samples) {
			continue
		}
		if s := video.Samples[p.DecodeIndex].Size; s > 0 {
			sizes = append(sizes, s)
		}
	}
	if len(sizes) == 0 {
		out.Summary.Warning = "no coded sample sizes available"
		return out
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
	median := sizes[len(sizes)/2]
	threshold := median / 20
	if threshold < 32 {
		threshold = 32
	}
	found := 0
	for i, p := range ps {
		if p.DecodeIndex < 0 || p.DecodeIndex >= len(video.Samples) {
			continue
		}
		size := video.Samples[p.DecodeIndex].Size
		if size > 0 && size <= threshold && size < median/2 {
			out.flags[i] = true
			found++
		}
	}
	if found == 0 {
		out.Summary.Warning = "no decoder sidecar supplied; sample-size heuristic found no black candidates"
		return out
	}
	out.Summary.Source = "sample-size-candidate"
	out.Summary.Matched = found
	out.Summary.Runs = runsFromFlags(out.flags, ps)
	out.Summary.Warning = "sample size is only a heuristic; use -frames-a/-frames-b for pixel-backed black-frame evidence"
	return out
}
