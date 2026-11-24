package cutluma

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
)

// TimelinePoint is one row of the full-frame curve export.
type TimelinePoint struct {
	Frame        int
	Segment      int
	LumaRaw      float64
	LumaRepaired float64
	SourceLuma   float64
	BlackRaw     float64
	WhiteRaw     float64
}

// CrossCutPoint is one row of a window centered on a cut.
type CrossCutPoint struct {
	Cut          int
	RelFrame     int
	AbsFrame     int
	Segment      int
	LumaRaw      float64
	LumaRepaired float64
	SourceLuma   float64
}

// BuildTimeline assembles per-frame raw/repaired/source median curves.
func BuildTimeline(job *Job, plan *RepairPlan) []TimelinePoint {
	qi := job.QIndex(0.50)
	bqi := job.QIndex(0.005)
	wqi := job.QIndex(0.995)
	if qi < 0 {
		qi = len(job.QuantileLevels) / 2
	}
	repaired := CorrectedCurve(job, plan)
	rows := make([]TimelinePoint, len(job.Frames))
	for t := range job.Frames {
		p := TimelinePoint{
			Frame: t, Segment: job.SegmentOf(t),
			LumaRaw: job.Frames[t].Out[qi], LumaRepaired: repaired[t],
		}
		if len(job.Frames[t].Src) > qi {
			p.SourceLuma = job.Frames[t].Src[qi]
		}
		if bqi >= 0 && bqi < len(job.Frames[t].Out) {
			p.BlackRaw = job.Frames[t].Out[bqi]
		}
		if wqi >= 0 && wqi < len(job.Frames[t].Out) {
			p.WhiteRaw = job.Frames[t].Out[wqi]
		}
		rows[t] = p
	}
	return rows
}

// BuildCrossCutCurves extracts Window-sized curves on both sides of each cut.
func BuildCrossCutCurves(job *Job, rep *Report, plan *RepairPlan) []CrossCutPoint {
	qi := job.QIndex(0.50)
	if qi < 0 {
		qi = len(job.QuantileLevels) / 2
	}
	raw := quantileColumn(job, qi, false)
	src := quantileColumn(job, qi, true)
	final := CorrectedCurve(job, plan)
	var rows []CrossCutPoint
	for ci, c := range job.Cuts {
		leftIdx, rightIdx := cutWindow(job, ci, rep.Config)
		for _, t := range leftIdx {
			rows = append(rows, CrossCutPoint{
				Cut: ci, RelFrame: t - c, AbsFrame: t, Segment: job.SegmentOf(t),
				LumaRaw: raw[t], LumaRepaired: final[t], SourceLuma: src[t],
			})
		}
		for _, t := range rightIdx {
			rows = append(rows, CrossCutPoint{
				Cut: ci, RelFrame: t - c, AbsFrame: t, Segment: job.SegmentOf(t),
				LumaRaw: raw[t], LumaRepaired: final[t], SourceLuma: src[t],
			})
		}
	}
	return rows
}

// WriteTimelineCSV writes the full curve to path.
func WriteTimelineCSV(path string, rows []TimelinePoint) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"frame", "segment", "luma_raw", "luma_repaired", "source_luma", "black_raw", "white_raw"})
	for _, r := range rows {
		_ = w.Write([]string{
			fmt.Sprintf("%d", r.Frame), fmt.Sprintf("%d", r.Segment),
			fmt.Sprintf("%.6f", r.LumaRaw), fmt.Sprintf("%.6f", r.LumaRepaired),
			fmt.Sprintf("%.6f", r.SourceLuma), fmt.Sprintf("%.6f", r.BlackRaw),
			fmt.Sprintf("%.6f", r.WhiteRaw),
		})
	}
	w.Flush()
	return w.Error()
}

// WriteCrossCutCSV writes all cut-centered curve windows to one file.
func WriteCrossCutCSV(path string, rows []CrossCutPoint) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"cut", "rel_frame", "abs_frame", "segment", "luma_raw", "luma_repaired", "source_luma"})
	for _, r := range rows {
		_ = w.Write([]string{
			fmt.Sprintf("%d", r.Cut), fmt.Sprintf("%d", r.RelFrame),
			fmt.Sprintf("%d", r.AbsFrame), fmt.Sprintf("%d", r.Segment),
			fmt.Sprintf("%.6f", r.LumaRaw), fmt.Sprintf("%.6f", r.LumaRepaired),
			fmt.Sprintf("%.6f", r.SourceLuma),
		})
	}
	w.Flush()
	return w.Error()
}
