// Package repair scans damaged MP4/MOV media, rebuilds the minimal sample
// tables (stsz/stsc/stco/stts) and keyframe index (stss), and writes a new
// repaired file. Source files are opened read-only; output is written through
// a temp file and atomically renamed, so failures never overwrite originals.
package repair

// Interval is a contiguous, independently playable range of samples.
// Sample numbers are 1 based against the rebuilt sample table.
type Interval struct {
	Index                 int     `json:"index"`
	FirstSample           int     `json:"first_sample"`
	LastSample            int     `json:"last_sample"`
	FirstSyncSample       int     `json:"first_sync_sample"`
	LeadingNonSyncSamples int     `json:"leading_non_sync_samples"`
	Keyframes             []int   `json:"keyframes"`
	StartTimeSeconds      float64 `json:"start_time_seconds"`
	EndTimeSeconds        float64 `json:"end_time_seconds"`
	StartOffset           int64   `json:"start_offset"`
	EndOffset             int64   `json:"end_offset"`
}

// UnrecoveredRange describes bytes or index slots that could not be restored.
type UnrecoveredRange struct {
	Kind           string `json:"kind"`
	StartOffset    int64  `json:"start_offset,omitempty"`
	EndOffset      int64  `json:"end_offset,omitempty"`
	Length         int64  `json:"length,omitempty"`
	MissingSamples int    `json:"missing_samples,omitempty"`
	Detail         string `json:"detail"`
}

// DamageReport classifies problems found in the old index / stream.
type DamageReport struct {
	Truncated              bool `json:"truncated"`
	OldIndexSampleCount    int  `json:"old_index_sample_count"`
	OldIndexOffsetCount    int  `json:"old_index_offset_count"`
	IndexDuplicateOffsets  int  `json:"index_duplicate_offsets"`
	IndexDuplicateRecords  int  `json:"index_duplicate_records"`
	BadOffsets             int  `json:"bad_offsets"`
	StreamDuplicateSamples int  `json:"stream_duplicate_samples"`
	MissingMoov            bool `json:"missing_moov"`
	UsedReference          bool `json:"used_reference"`
}

// Report is the machine readable result of inspecting/repairing one file.
type Report struct {
	Input          string             `json:"input"`
	Output         string             `json:"output,omitempty"`
	FileSize       int64              `json:"file_size"`
	MediaTimescale uint32             `json:"media_timescale"`
	NominalFPS     float64            `json:"nominal_fps,omitempty"`
	ScannedSamples int                `json:"scanned_samples"`
	IndexedSamples int                `json:"indexed_samples"`
	Keyframes      []int              `json:"keyframes"`
	Intervals      []Interval         `json:"intervals"`
	Unrecovered    []UnrecoveredRange `json:"unrecovered"`
	Damage         DamageReport       `json:"damage"`
	PayloadBytes   int64              `json:"payload_bytes_copied"`
	WroteOutput    bool               `json:"wrote_output"`
}

// Options configures a repair run.
type Options struct {
	// Output is the destination MP4 path. It must differ from the source.
	Output string
	// Reference is an optional healthy MP4 from the same recorder, used to
	// borrow codec configuration when the source moov is missing.
	Reference string
	// FPS is the fallback frame rate when no stts/timing is available.
	FPS float64
	// Timescale overrides the fallback media timescale.
	Timescale uint32
	// DedupeFrames drops consecutive byte identical samples from the new
	// index. Media bytes are still copied verbatim.
	DedupeFrames bool
	// MoovLast writes ftyp+mdat+moov instead of the default faststart
	// ftyp+moov+mdat layout. Useful for index surgery tooling/tests.
	MoovLast bool
}

// SampleInfo is a read-only view of one scanned sample for callers/tests.
type SampleInfo struct {
	Offset   int64
	Size     int64
	Keyframe bool
	Kept     bool
}

// SamplesInfo returns scanned samples in file order.
func (p *Plan) SamplesInfo() []SampleInfo {
	out := make([]SampleInfo, len(p.Samples))
	for i, s := range p.Samples {
		out[i] = SampleInfo{Offset: s.srcOff, Size: s.size, Keyframe: s.key, Kept: s.keep}
	}
	return out
}
