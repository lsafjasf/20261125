package main

import (
	"fmt"
	"io"
)

func printManifestText(w io.Writer, m *Manifest) {
	fmt.Fprintf(w, "format: %s\n", m.Format)
	fmt.Fprintf(w, "size: %d\n", m.Size)
	fmt.Fprintf(w, "sha256: %s\n", m.SHA256)
	fmt.Fprintf(w, "container blocks: %d (block size %d)\n", len(m.Blocks), m.Options.BlockSize)
	fmt.Fprintf(w, "container boxes: %d\n", len(m.Boxes))
	fmt.Fprintf(w, "mp4 chunks: %d\n", len(m.Chunks))
	fmt.Fprintf(w, "tracks: %d\n", len(m.Tracks))
	for _, track := range m.Tracks {
		fmt.Fprintf(w, "  track %d: %s handler=%s samples=%d timescale=%d\n", track.TrackID, track.Kind, track.Handler, track.SampleCount, track.Timescale)
	}
	fmt.Fprintf(w, "media samples: %d\n", len(m.Samples))
	fmt.Fprintf(w, "audio intervals: %d (window %d samples)\n", len(m.AudioIntervals), m.Options.AudioIntervalSamples)
	for _, warning := range m.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}
}

func printReportText(w io.Writer, r *Report) {
	switch {
	case r.WholeFileIdentical:
		fmt.Fprintln(w, "IDENTICAL: whole files have the same SHA-256")
	case r.MediaIdentical && r.NormalRearrangement:
		fmt.Fprintln(w, "MEDIA IDENTICAL: only normal rearrangement/container differences were found")
	case r.MediaIdentical:
		fmt.Fprintln(w, "MEDIA IDENTICAL: container/metadata differences do not change samples or audio intervals")
	default:
		fmt.Fprintln(w, "MEDIA DIFFERENCES: picture or sound content changed")
	}

	fmt.Fprintf(w, "whole_file_identical=%v media_identical=%v container_identical=%v normal_rearrangement=%v\n",
		r.WholeFileIdentical, r.MediaIdentical, r.ContainerIdentical, r.NormalRearrangement)

	if r.EarliestByte != nil {
		c := r.EarliestByte
		fmt.Fprintf(w, "earliest raw byte: original=%d copy=%d block=%d (%s)\n",
			c.OriginalOffset, c.CopyOffset, c.BlockIndex, c.Message)
	}
	if r.EarliestMediaChange != nil {
		c := r.EarliestMediaChange
		fmt.Fprintf(w, "earliest media byte: layer=%s original=%d copy=%d relative=%d\n",
			c.Layer, c.OriginalOffset, c.CopyOffset, c.RelativeOffset)
		fmt.Fprintf(w, "  %s\n", c.Message)
		if c.Media != nil {
			fmt.Fprintf(w, "  affected: %s at %.6f-%.6fs\n", c.Media.Description, c.Media.StartTime, c.Media.EndTime)
		}
	}

	fmt.Fprintf(w, "media changes: %d; rearrangements: %d; container changes: %d; ignorable box changes: %d\n",
		len(r.Changes), len(r.Rearrangements), len(r.ContainerChanges), len(r.IgnoredChanges))

	for _, c := range r.Changes {
		fmt.Fprintf(w, "- [%s] %s: original@%d copy@%d", c.Layer, c.Kind, c.OriginalOffset, c.CopyOffset)
		if c.RelativeOffset != 0 {
			fmt.Fprintf(w, " relative=%d", c.RelativeOffset)
		}
		fmt.Fprintln(w)
		if c.Media != nil && c.Media.Description != "" {
			fmt.Fprintf(w, "    %s\n", c.Media.Description)
		}
	}

	printSomeChanges(w, "container", r.ContainerChanges, 20)
	printSomeChanges(w, "ignored", r.IgnoredChanges, 20)

	for _, warning := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}
}

func printSomeChanges(w io.Writer, label string, changes []Change, limit int) {
	for i, c := range changes {
		if i >= limit {
			fmt.Fprintf(w, "... %d more %s changes available in JSON\n", len(changes)-limit, label)
			return
		}
		fmt.Fprintf(w, "  %s: %s %s original@%d copy@%d\n", label, c.Layer, c.Kind, c.OriginalOffset, c.CopyOffset)
	}
}
