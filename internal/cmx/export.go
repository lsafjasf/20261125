// Package cmx exports and re-opens CMX3600-style EDL files.
package cmx

import (
	"fmt"
	"sort"
	"strings"

	"cutcheck/internal/rational"
	"cutcheck/internal/timecode"
	"cutcheck/internal/timeline"
)

// ExportConfig controls how the canonical edit list is flattened to CMX.
type ExportConfig struct {
	// FPSNum/FPSDen: frame rate the exporter *assumes* when converting
	// timeline seconds to timecode frames. Must match the edit list rate;
	// a mismatch (e.g. 30/1 vs 30000/1001) causes cumulative frame drift.
	FPSNum int64
	FPSDen int64
	// DropFrame selects drop-frame timecode (only valid for 29.97).
	DropFrame bool
	// ReelName is the tape/reel name written for every event.
	ReelName string
}

// CorrectConfig returns the export config that faithfully represents el.
func CorrectConfig(el *timeline.EditList) ExportConfig {
	return ExportConfig{
		FPSNum:    el.FPSNum,
		FPSDen:    el.FPSDen,
		DropFrame: true,
		ReelName:  "REEL001",
	}
}

// nominalFPS is the integer timecode base for the configured rate
// (29.97 -> 30). Only whole-frame rates near an integer are supported.
func (c ExportConfig) nominalFPS() int64 {
	return rational.New(c.FPSNum, c.FPSDen).RoundInt64()
}

// frameToTC converts a timeline frame to a timecode string using the
// configured (possibly wrong) frame rate.
func (c ExportConfig) frameToTC(el *timeline.EditList, frame int64) string {
	sec := el.FrameSeconds(frame)
	outFrames := sec.Mul(rational.New(c.FPSNum, c.FPSDen)).RoundInt64()
	return timecode.Format(outFrames, c.nominalFPS(), c.DropFrame)
}

// samplesToTC quantizes a sample position to a timecode at frame
// granularity: CMX cannot express sub-frame audio placement.
func (c ExportConfig) samplesToTC(el *timeline.EditList, sample int64) string {
	sec := el.SampleSeconds(sample)
	outFrames := sec.Mul(rational.New(c.FPSNum, c.FPSDen)).RoundInt64()
	return timecode.Format(outFrames, c.nominalFPS(), c.DropFrame)
}

// Export renders the edit list as a CMX3600 EDL. Retime events are written
// at 1.0x speed (a known CMX limitation) and reported in the returned
// warnings. Audio fades are dropped by the format and also reported.
func Export(el *timeline.EditList, cfg ExportConfig) (string, []string) {
	var b strings.Builder
	var warnings []string

	fmt.Fprintf(&b, "TITLE: %s\n", el.Title)
	if cfg.DropFrame {
		b.WriteString("FCM: DROP FRAME\n")
	} else {
		b.WriteString("FCM: NON-DROP FRAME\n")
	}
	b.WriteString("\n")

	type row struct {
		recIn int64
		lines []string
	}
	var rows []row

	n := 0
	for _, ev := range el.Video {
		n++
		srcIn, srcOut := ev.SourceIn, ev.SourceOut
		if ev.IsRetime() {
			warnings = append(warnings, fmt.Sprintf(
				"event %s: retime %s not representable in CMX; exported at 1.0x",
				ev.ID, ev.Speed()))
			srcOut = srcIn + (ev.RecordOut - ev.RecordIn)
		}
		trans := "C       "
		if ev.Transition == timeline.TransitionDissolve {
			trans = fmt.Sprintf("D    %03d", ev.TransitionDur)
		}
		line := fmt.Sprintf("%03d  %-8s V     %s  %s %s %s %s",
			n, cfg.ReelName, trans,
			cfg.frameToTC(el, srcIn), cfg.frameToTC(el, srcOut),
			cfg.frameToTC(el, ev.RecordIn), cfg.frameToTC(el, ev.RecordOut))
		rows = append(rows, row{ev.RecordIn, []string{
			line,
			fmt.Sprintf("* FROM CLIP NAME: %s", ev.Clip),
			fmt.Sprintf("* EVENT ID: %s", ev.ID),
			fmt.Sprintf("* TRACK: V%d", ev.Track),
		}})
	}

	for _, ev := range el.Audio {
		n++
		if ev.FadeIn > 0 || ev.FadeOut > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"event %s: audio fades not representable in CMX; dropped", ev.ID))
		}
		line := fmt.Sprintf("%03d  %-8s A%d    C        %s %s %s %s",
			n, cfg.ReelName, ev.Track,
			cfg.samplesToTC(el, ev.SourceIn), cfg.samplesToTC(el, ev.SourceOut),
			cfg.samplesToTC(el, ev.RecordIn), cfg.samplesToTC(el, ev.RecordOut))
		rows = append(rows, row{ev.RecordIn, []string{
			line,
			fmt.Sprintf("* FROM CLIP NAME: %s", ev.Clip),
			fmt.Sprintf("* EVENT ID: %s", ev.ID),
		}})
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].recIn < rows[j].recIn })
	for i, r := range rows {
		for j, ln := range r.lines {
			if j == 0 {
				ln = fmt.Sprintf("%03d%s", i+1, ln[3:])
			}
			b.WriteString(ln + "\n")
		}
		b.WriteString("\n")
	}
	return b.String(), warnings
}
