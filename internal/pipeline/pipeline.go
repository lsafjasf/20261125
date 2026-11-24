// Package pipeline orchestrates export, re-open, cross-format mapping,
// diagnosis and equivalence re-test.
package pipeline

import (
	"cutcheck/internal/cmx"
	"cutcheck/internal/compare"
	"cutcheck/internal/fcpxml"
	"cutcheck/internal/timeline"
	"fmt"
	"os"
	"path/filepath"
)

// PhaseResult holds everything learned from re-opening one pair of exports.
type PhaseResult struct {
	CMXPath   string
	FCPXPath  string
	Warnings  []string
	Mapping   []compare.MappingRow
	Deltas    []compare.EventDelta
	Missing   []string
	Diagnoses []compare.Diagnosis
	Failures  []string
}

// DeclaredNonEquivalents derives the items CMX3600 cannot represent.
func DeclaredNonEquivalents(el *timeline.EditList) []compare.NonEquivalent {
	var out []compare.NonEquivalent
	for _, ev := range el.Video {
		if ev.IsRetime() {
			out = append(out, compare.NonEquivalent{
				EventID: ev.ID,
				Format:  "cmx",
				Reason:  fmt.Sprintf("retime %s not representable in CMX3600; exported at 1.0x", ev.Speed()),
			})
		}
	}
	hasFades := false
	for _, ev := range el.Audio {
		if ev.FadeIn > 0 || ev.FadeOut > 0 {
			hasFades = true
		}
	}
	if hasFades {
		out = append(out, compare.NonEquivalent{
			Format: "cmx",
			Reason: "audio fades not representable in CMX3600; dropped",
		})
	}
	out = append(out, compare.NonEquivalent{
		Format: "cmx",
		Reason: "audio timing quantized to frame boundary (CMX has no sample precision)",
	})
	return out
}

// ExportPair renders both formats into dir and returns the file paths.
func ExportPair(el *timeline.EditList, dir string, cmxCfg cmx.ExportConfig, fcpCfg fcpxml.ExportConfig) (cmxPath, fcpPath string, warnings []string, err error) {
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", "", nil, err
	}
	cmxText, w1 := cmx.Export(el, cmxCfg)
	fcpText, _ := fcpxml.Export(el, fcpCfg)
	warnings = append(warnings, w1...)
	cmxPath = filepath.Join(dir, "promo.cmx.edl")
	fcpPath = filepath.Join(dir, "promo.fcpxml")
	if err = os.WriteFile(cmxPath, []byte(cmxText), 0o644); err != nil {
		return "", "", nil, err
	}
	if err = os.WriteFile(fcpPath, []byte(fcpText), 0o644); err != nil {
		return "", "", nil, err
	}
	return cmxPath, fcpPath, warnings, nil
}

// ReopenAndCheck re-opens both export files from disk, builds the
// cross-format event mapping, and runs diagnosis + equivalence checks.
func ReopenAndCheck(el *timeline.EditList, cmxPath, fcpPath string, declared []compare.NonEquivalent) (*PhaseResult, error) {
	res := &PhaseResult{CMXPath: cmxPath, FCPXPath: fcpPath}

	cmxText, err := os.ReadFile(cmxPath)
	if err != nil {
		return nil, err
	}
	// Timecodes are interpreted at the canonical project rate: a file
	// written with a wrong export rate is exposed here.
	cmxEvents, err := cmx.Parse(string(cmxText), el.FPSNum, el.FPSDen)
	if err != nil {
		return nil, fmt.Errorf("reopen %s: %w", cmxPath, err)
	}
	fcpText, err := os.ReadFile(fcpPath)
	if err != nil {
		return nil, err
	}
	fcpEvents, err := fcpxml.Parse(string(fcpText))
	if err != nil {
		return nil, fmt.Errorf("reopen %s: %w", fcpPath, err)
	}

	files := []compare.FormatFile{
		{Format: "cmx", Events: toReopened(cmxEvents)},
		{Format: "fcpxml", Events: toReopenedFCP(fcpEvents)},
	}
	res.Mapping = compare.Map(el, files)
	res.Deltas, res.Missing = compare.Deltas(el, files, declared)
	tols := compare.DefaultTolerances(el)
	res.Diagnoses = compare.Diagnose(el, res.Deltas, tols)
	res.Failures = compare.Failures(el, res.Deltas, res.Missing, tols)
	return res, nil
}

func toReopened(events []cmx.Event) []compare.Reopened {
	out := make([]compare.Reopened, len(events))
	for i, ev := range events {
		out[i] = compare.Reopened{
			ID: ev.ID, Clip: ev.Clip, Layer: ev.Layer, Track: ev.Track,
			RecordIn: ev.RecordIn, RecordOut: ev.RecordOut,
			SourceIn: ev.SourceIn, SourceOut: ev.SourceOut,
		}
	}
	return out
}

func toReopenedFCP(events []fcpxml.Event) []compare.Reopened {
	out := make([]compare.Reopened, len(events))
	for i, ev := range events {
		out[i] = compare.Reopened{
			ID: ev.ID, Clip: ev.Clip, Layer: ev.Layer, Track: ev.Track,
			RecordIn: ev.RecordIn, RecordOut: ev.RecordOut,
			SourceIn: ev.SourceIn, SourceOut: ev.SourceOut,
		}
	}
	return out
}
