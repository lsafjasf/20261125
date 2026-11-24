package main

const (
	SchemaVersion = "mediahash/v1"

	FormatMP4   = "mp4"
	FormatWAV   = "wav"
	FormatOther = "other"

	LayerRawBlock      = "container-block"
	LayerMP4Chunk      = "mp4-chunk"
	LayerBox           = "container-box"
	LayerSample        = "media-sample"
	LayerAudioInterval = "audio-interval"
	LayerByte          = "byte"
)

// Options controls manifest construction and comparison.
type Options struct {
	// BlockSize is the fixed-size raw container block hash window.
	BlockSize int `json:"block_size"`
	// AudioIntervalSamples is the number of PCM frames or encoded audio
	// samples grouped by one audio interval hash.
	AudioIntervalSamples int `json:"audio_interval_samples"`
}

// DefaultOptions returns bounded defaults suitable for forensic manifests.
func DefaultOptions() Options {
	return Options{
		BlockSize:            1 << 20,
		AudioIntervalSamples: 1024,
	}
}

func (o Options) withDefaults() Options {
	d := DefaultOptions()
	if o.BlockSize <= 0 {
		o.BlockSize = d.BlockSize
	}
	if o.AudioIntervalSamples <= 0 {
		o.AudioIntervalSamples = d.AudioIntervalSamples
	}
	return o
}

// RangeHash is a hash over a byte range in the physical container file.
type RangeHash struct {
	Layer  string `json:"layer"`
	Key    string `json:"key,omitempty"`
	Index  int    `json:"index"`
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// BoxHash describes one ISO BMFF box and its raw on-disk range.
type BoxHash struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	Index     int    `json:"index"`
	Start     int64  `json:"start"`
	HeaderEnd int64  `json:"header_end"`
	End       int64  `json:"end"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Ignorable bool   `json:"ignorable,omitempty"`
}

// TrackInfo contains timing and identification for one media track.
type TrackInfo struct {
	TrackID     uint32       `json:"track_id"`
	Handler     string       `json:"handler,omitempty"`
	Kind        string       `json:"kind"`
	Timescale   uint32       `json:"timescale,omitempty"`
	Duration    uint64       `json:"duration,omitempty"`
	SampleCount int          `json:"sample_count"`
	ChunkCount  int          `json:"chunk_count,omitempty"`
	Audio       *AudioFormat `json:"audio_format,omitempty"`
}

// AudioFormat describes audio layout when the container exposes it.
type AudioFormat struct {
	FormatTag     uint16 `json:"format_tag,omitempty"`
	Channels      int    `json:"channels,omitempty"`
	SampleRate    uint32 `json:"sample_rate,omitempty"`
	BitsPerSample int    `json:"bits_per_sample,omitempty"`
	BlockAlign    int    `json:"block_align,omitempty"`
}

// SampleHash is the content hash of one MP4/media sample.
type SampleHash struct {
	Key          string `json:"key"`
	TrackID      uint32 `json:"track_id"`
	TrackKind    string `json:"track_kind"`
	Index        int    `json:"index"`
	ChunkIndex   int    `json:"chunk_index,omitempty"`
	Offset       int64  `json:"offset"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	DTS          int64  `json:"dts,omitempty"`
	PTS          int64  `json:"pts,omitempty"`
	Duration     uint32 `json:"duration,omitempty"`
	Sync         bool   `json:"sync,omitempty"`
	DisplayOrder int    `json:"display_order,omitempty"`
}

// AudioIntervalHash is a hash over a contiguous audio sampling interval.
//
// For PCM WAV, StartFrame and EndFrame are interleaved PCM frame numbers.
// For MP4 audio, they are encoded media sample numbers unless PCM is known.
type AudioIntervalHash struct {
	Key          string  `json:"key"`
	TrackID      uint32  `json:"track_id,omitempty"`
	Source       string  `json:"source"`
	Index        int     `json:"index"`
	StartSample  int64   `json:"start_sample"`
	EndSample    int64   `json:"end_sample"`
	StartTicks   int64   `json:"start_ticks,omitempty"`
	EndTicks     int64   `json:"end_ticks,omitempty"`
	StartSeconds float64 `json:"start_seconds,omitempty"`
	EndSeconds   float64 `json:"end_seconds,omitempty"`
	Offset       int64   `json:"offset,omitempty"`
	Size         int64   `json:"size,omitempty"`
	SHA256       string  `json:"sha256"`
}

// Manifest is the layered hash description of one archive file.
type Manifest struct {
	Schema         string              `json:"schema"`
	FileName       string              `json:"file_name,omitempty"`
	Format         string              `json:"format"`
	Size           int64               `json:"size"`
	SHA256         string              `json:"sha256"`
	Options        Options             `json:"options"`
	Blocks         []RangeHash         `json:"blocks,omitempty"`
	Boxes          []BoxHash           `json:"boxes,omitempty"`
	Chunks         []RangeHash         `json:"chunks,omitempty"`
	Tracks         []TrackInfo         `json:"tracks,omitempty"`
	Samples        []SampleHash        `json:"samples,omitempty"`
	AudioIntervals []AudioIntervalHash `json:"audio_intervals,omitempty"`
	Warnings       []string            `json:"warnings,omitempty"`
}
