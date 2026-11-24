package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"videotear/internal/vdetect"
)

type jsonlFrame struct {
	Type string `json:"type"`
	vdetect.FrameResult
}

type jsonlTracks struct {
	Type    string          `json:"type"`
	Summary vdetect.Summary `json:"summary"`
	Tracks  []vdetect.Track `json:"tracks"`
	Options vdetect.Options `json:"options"`
}

func writeReport(rep vdetect.Report, format, path string) error {
	var w io.Writer = os.Stdout
	if path != "" {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	case "jsonl", "ndjson":
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		for _, fr := range rep.Frames {
			if err := enc.Encode(jsonlFrame{Type: "frame", FrameResult: fr}); err != nil {
				return err
			}
		}
		return enc.Encode(jsonlTracks{Type: "tracks", Summary: rep.Summary, Tracks: rep.Tracks, Options: rep.Options})
	case "text", "txt":
		writeText(w, rep)
		return nil
	default:
		return fmt.Errorf("unsupported format %q (json, jsonl, text)", format)
	}
}

func writeText(w io.Writer, rep vdetect.Report) {
	fmt.Fprintf(w, "videotear %s: %d frames, %d tracks, max confidence %.2f\n", rep.SchemaVersion, rep.Summary.Frames, rep.Summary.Tracks, rep.Summary.MaxConfidence)
	fmt.Fprintf(w, "tear=%d field=%d rolling=%d blocks=%d effects=%d findings=%d\n",
		rep.Summary.TearFrames, rep.Summary.FieldOrderFrames, rep.Summary.RollingFrames,
		rep.Summary.BlockFrames, rep.Summary.EffectFrames, rep.Summary.FramesWithFindings)
	for _, fr := range rep.Frames {
		if fr.Classification == "none" {
			continue
		}
		fmt.Fprintf(w, "#%05d %s conf=%.2f", fr.Index, fr.Classification, fr.Confidence)
		if len(fr.Labels) > 0 {
			fmt.Fprintf(w, " [%s]", strings.Join(fr.Labels, ","))
		}
		fmt.Fprintln(w)
		for _, t := range fr.Tears {
			fmt.Fprintf(w, "  tear y=%d x=%d..%d shift=%.1fpx coverage=%.2f conf=%.2f track=%d\n",
				t.Y, t.XStart, t.XEnd, t.ShiftPx, t.Coverage, t.Confidence, t.TrackID)
		}
		if fr.FieldOrder != nil && fr.FieldOrder.Detected {
			fo := fr.FieldOrder
			fmt.Fprintf(w, "  field parity_shift=%.1f rows=%d/%d comb=%.2f conf=%.2f track=%d\n",
				fo.ParityShiftPx, fo.SupportedRows, fo.TexturedRows, fo.CombScore, fo.Confidence, fo.TrackID)
		}
		for _, b := range fr.Blocks {
			fmt.Fprintf(w, "  block x=%d y=%d %dx%d grid=%d conf=%.2f track=%d\n",
				b.X, b.Y, b.W, b.H, b.BlockSize, b.Conf, b.TrackID)
		}
		for _, e := range fr.Effects {
			fmt.Fprintf(w, "  %s x=%d y=%d %dx%d conf=%.2f\n", e.Kind, e.X, e.Y, e.W, e.H, e.Conf)
		}
	}
	for _, tr := range rep.Tracks {
		fmt.Fprintf(w, "track #%d %s/%s frames=%d..%d obs=%d continuity=%.2f conf=%.2f",
			tr.ID, tr.Kind, tr.Classification, tr.StartFrame, tr.EndFrame, tr.FrameCount, tr.Continuity, tr.Confidence)
		if tr.Kind == "tear" {
			fmt.Fprintf(w, " y=%d..%d vy=%.1f mono=%.2f wraps=%d", tr.YStart, tr.YEnd, tr.VelocityYPx, tr.MonotonicRatio, tr.WrapCount)
		}
		fmt.Fprintln(w)
	}
}
