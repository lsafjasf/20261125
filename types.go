package matroska

// Lacing identifies the frame length coding used by a Block.
type Lacing uint8

const (
	LacingNone  Lacing = 0
	LacingXiph  Lacing = 2
	LacingFixed Lacing = 4
	LacingEBML  Lacing = 6
)

func (l Lacing) String() string {
	switch l {
	case LacingNone:
		return "none"
	case LacingXiph:
		return "xiph"
	case LacingFixed:
		return "fixed"
	case LacingEBML:
		return "ebml"
	default:
		return "unknown"
	}
}

// Frame is one laced frame. Start and End are absolute byte offsets in the
// scanned file, with End exclusive. Timecode is in Matroska timestamp-scale
// ticks, while TimeNanos is the same instant in nanoseconds.
type Frame struct {
	Index     int   `json:"index"`
	Start     int64 `json:"start"`
	End       int64 `json:"end"`
	Timecode  int64 `json:"timecode"`
	TimeNanos int64 `json:"time_ns"`
}

// Size returns the frame's byte length.
func (f Frame) Size() int64 { return f.End - f.Start }

// Block is a parsed Matroska Block or SimpleBlock.
type Block struct {
	TrackNumber      uint64  `json:"track_number"`
	RelativeTimecode int16   `json:"relative_timecode"`
	Timecode         int64   `json:"timecode"`
	TimeNanos        int64   `json:"time_ns"`
	Flags            byte    `json:"flags"`
	Lacing           Lacing  `json:"lacing"`
	FrameCount       int     `json:"frame_count"`
	ElementOffset    int64   `json:"element_offset"`
	DataOffset       int64   `json:"data_offset"`
	Frames           []Frame `json:"frames"`
}

// BlockOptions supplies file position and timing context for ParseBlock.
// PayloadOffset is the absolute file offset of payload[0].
type BlockOptions struct {
	ElementOffset        int64
	PayloadOffset        int64
	ClusterTimecode      int64
	TimestampScale       int64
	DefaultFrameDuration int64
}

// Issue describes a recoverable or terminal problem found by Scan.
type Issue struct {
	Kind      ErrorKind `json:"kind"`
	Offset    int64     `json:"offset"`
	ElementID uint64    `json:"element_id,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Recovered bool      `json:"recovered"`
}

// ScanResult is the output of Scan. Frames is a flattened view of Blocks.
type ScanResult struct {
	Blocks []Block `json:"blocks"`
	Frames []Frame `json:"frames"`
	Issues []Issue `json:"issues,omitempty"`
}

// ScanOptions controls full-file scanning.
type ScanOptions struct {
	// TimestampScale overrides the TimecodeScale read from the file when > 0.
	TimestampScale int64
	// DefaultFrameDuration overrides track DefaultDuration when > 0.
	DefaultFrameDuration int64
	// ReportUnknown records unknown/unsupported elements as issues. They are
	// always skipped safely when their size is known.
	ReportUnknown bool
}
