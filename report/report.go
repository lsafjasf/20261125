// Package report builds per-track colour metadata and an inconsistency
// list for multi-track MP4/MOV files mixing HDR and SDR material.
package report

import (
	"fmt"

	"trackmeta/mp4"
	"trackmeta/sps"
)

// Issue is a single finding tied to one track.
type Issue struct {
	TrackIndex int    `json:"trackIndex"`
	TrackID    uint32 `json:"trackId"`
	Severity   string `json:"severity"` // error / warning / info
	Code       string `json:"code"`
	Message    string `json:"message"`
}

// SampleReport is the colour audit of one stsd sample description.
type SampleReport struct {
	Format         string         `json:"format"`
	Width          int            `json:"width"`
	Height         int            `json:"height"`
	ContainerColor *mp4.ColorInfo `json:"containerColor,omitempty"`
	BitstreamColor *mp4.ColorInfo `json:"bitstreamColor,omitempty"`
	Classification string         `json:"classification"` // HDR / SDR / unknown
}

// TrackReport is the colour audit of one track.
type TrackReport struct {
	Index          int            `json:"index"`
	ID             uint32         `json:"id"`
	Handler        string         `json:"handler"`
	Classification string         `json:"classification"`
	Samples        []SampleReport `json:"sampleDescriptions"`
}

// Report is the full audit result for one file.
type Report struct {
	File            string        `json:"file"`
	VideoTrackCount int           `json:"videoTrackCount"`
	Tracks          []TrackReport `json:"tracks"`
	Inconsistencies []Issue       `json:"inconsistencies"`
}

// Build audits parsed tracks and produces the report.
func Build(file string, tracks []*mp4.Track) *Report {
	rep := &Report{File: file}
	for _, t := range tracks {
		if t.Handler != "vide" {
			continue
		}
		rep.VideoTrackCount++
		tr := TrackReport{Index: t.Index, ID: t.ID, Handler: t.Handler}
		for i := range t.Samples {
			sd := &t.Samples[i]
			sr := SampleReport{Format: sd.Format, Width: sd.Width, Height: sd.Height}
			sr.ContainerColor = sd.ContainerColr
			if len(sd.CodecConfig) > 0 {
				if c, err := sps.ColorFromConfig(sd.ConfigType, sd.CodecConfig); err == nil {
					sr.BitstreamColor = &mp4.ColorInfo{
						Primaries:  c.Primaries,
						Transfer:   c.Transfer,
						Matrix:     c.Matrix,
						FullRange:  c.FullRange,
						RangeKnown: true,
						Source:     "bitstream/" + sd.ConfigType,
					}
					mp4.NameColor(sr.BitstreamColor)
				}
			}
			sr.Classification = classify(sr.ContainerColor, sr.BitstreamColor)
			tr.Samples = append(tr.Samples, sr)
			rep.checkSample(t, sd, &sr)
		}
		tr.Classification = classifyTrack(tr.Samples)
		rep.Tracks = append(rep.Tracks, tr)
	}
	rep.checkMix()
	if rep.Inconsistencies == nil {
		rep.Inconsistencies = []Issue{}
	}
	return rep
}

func classify(container, bitstream *mp4.ColorInfo) string {
	// The bitstream is authoritative: a wrong container tag must not
	// mask what the track actually contains.
	for _, c := range []*mp4.ColorInfo{bitstream, container} {
		if c == nil {
			continue
		}
		if mp4.IsHDRTransfer(c.Transfer) {
			return "HDR"
		}
		if c.Transfer != 0 && c.Transfer != 2 {
			return "SDR"
		}
	}
	return "unknown"
}

func classifyTrack(samples []SampleReport) string {
	seen := ""
	for _, s := range samples {
		if s.Classification == "unknown" {
			continue
		}
		if seen == "" {
			seen = s.Classification
		} else if seen != s.Classification {
			return "mixed"
		}
	}
	if seen == "" {
		return "unknown"
	}
	return seen
}

func (rep *Report) add(t *mp4.Track, sev, code, msg string) {
	rep.Inconsistencies = append(rep.Inconsistencies, Issue{
		TrackIndex: t.Index, TrackID: t.ID,
		Severity: sev, Code: code, Message: msg,
	})
}

func (rep *Report) checkSample(t *mp4.Track, sd *mp4.SampleDescription, sr *SampleReport) {
	where := fmt.Sprintf("track %d (id=%d, %s)", t.Index, t.ID, sd.Format)
	if sd.ContainerColr == nil {
		rep.add(t, "error", "missing-colr",
			where+" has no colr box: the file-level/first-track colour tag does not describe this track")
	} else {
		if sd.ColrType == "nclc" {
			rep.add(t, "warning", "legacy-colr",
				where+" uses deprecated nclc colr (no full-range flag)")
		}
		if !sd.ContainerColr.RangeKnown {
			rep.add(t, "warning", "range-unknown",
				where+" colr box omits the full-range flag")
		}
	}
	rep.crossCheck(t, sd, sr)
}

// crossCheck compares container colr against bitstream VUI field by field.
func (rep *Report) crossCheck(t *mp4.Track, sd *mp4.SampleDescription, sr *SampleReport) {
	c, b := sr.ContainerColor, sr.BitstreamColor
	if c == nil || b == nil {
		return
	}
	where := fmt.Sprintf("track %d (id=%d, %s)", t.Index, t.ID, sd.Format)
	type field struct {
		name   string
		cv, bv uint16
		cn, bn string
	}
	fields := []field{
		{"primaries", c.Primaries, b.Primaries, c.PrimariesName, b.PrimariesName},
		{"transfer", c.Transfer, b.Transfer, c.TransferName, b.TransferName},
		{"matrix", c.Matrix, b.Matrix, c.MatrixName, b.MatrixName},
	}
	for _, f := range fields {
		if f.cv != f.bv {
			rep.add(t, "error", "colr-vs-bitstream",
				fmt.Sprintf("%s %s mismatch: container=%d (%s) bitstream=%d (%s)",
					where, f.name, f.cv, f.cn, f.bv, f.bn))
		}
	}
	if c.RangeKnown && c.FullRange != b.FullRange {
		rep.add(t, "error", "range-mismatch",
			fmt.Sprintf("%s full-range mismatch: container=%v bitstream=%v",
				where, c.FullRange, b.FullRange))
	}
}

// checkMix flags HDR/SDR mixing across tracks and tracks whose effective
// classification contradicts the first track's container-level tag.
func (rep *Report) checkMix() {
	if len(rep.Tracks) == 0 {
		return
	}
	classes := map[string]bool{}
	for _, tr := range rep.Tracks {
		classes[tr.Classification] = true
	}
	if classes["HDR"] && (classes["SDR"] || classes["unknown"]) {
		rep.Inconsistencies = append(rep.Inconsistencies, Issue{
			TrackIndex: -1, Severity: "info", Code: "hdr-sdr-mix",
			Message: "file mixes HDR and SDR/untagged video tracks; per-track tags are required",
		})
	}
	first := rep.Tracks[0]
	for _, tr := range rep.Tracks[1:] {
		if first.Classification != "unknown" && tr.Classification != "unknown" &&
			tr.Classification != first.Classification {
			rep.Inconsistencies = append(rep.Inconsistencies, Issue{
				TrackIndex: tr.Index, TrackID: tr.ID,
				Severity: "error", Code: "first-track-tag-mismatch",
				Message: fmt.Sprintf("track %d is %s but the container-level tag (from track %d) says %s",
					tr.Index, tr.Classification, first.Index, first.Classification),
			})
		}
	}
}
