package audit

const SchemaVersion = "1.0"

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Issue struct {
	Severity    Severity `json:"severity"`
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Offset      int64    `json:"offset,omitempty"`
	EndOffset   int64    `json:"end_offset,omitempty"`
	FrameIndex  int      `json:"frame_index,omitempty"`
	SliceIndex  int      `json:"slice_index,omitempty"`
	TrackNumber uint64   `json:"track_number,omitempty"`
}

type UnknownElement struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ParentID     string `json:"parent_id,omitempty"`
	Offset       int64  `json:"offset"`
	HeaderSize   int    `json:"header_size"`
	DataOffset   int64  `json:"data_offset"`
	DeclaredSize int64  `json:"declared_size"`
	SizeUnknown  bool   `json:"size_unknown,omitempty"`
}

type TrackInfo struct {
	Number          uint64 `json:"number"`
	UID             uint64 `json:"uid,omitempty"`
	Type            uint64 `json:"type"`
	CodecID         string `json:"codec_id,omitempty"`
	CodecFourCC     string `json:"codec_fourcc,omitempty"`
	FFV1            bool   `json:"ffv1,omitempty"`
	Width           uint64 `json:"width,omitempty"`
	Height          uint64 `json:"height,omitempty"`
	DefaultDuration uint64 `json:"default_duration,omitempty"`
	PrivateSize     int    `json:"private_size,omitempty"`
	PrivateCRC32    string `json:"private_crc32,omitempty"`
	PrivateCRCOK    *bool  `json:"private_crc_ok,omitempty"`
}

type CuePosition struct {
	CueTimeNS         int64  `json:"cue_time_ns,omitempty"`
	Track             uint64 `json:"track"`
	ClusterPosition   uint64 `json:"cluster_position"`
	RelativePosition  uint64 `json:"relative_position,omitempty"`
	BlockNumber       uint64 `json:"block_number,omitempty"`
	ResolvedOffset    int64  `json:"resolved_offset,omitempty"`
	ResolvedElementID string `json:"resolved_element_id,omitempty"`
	PositionOK        bool   `json:"position_ok"`
}

type ContainerCRC struct {
	ParentElementID  string `json:"parent_element_id"`
	ParentOffset     int64  `json:"parent_offset"`
	CRCElementOffset int64  `json:"crc_element_offset"`
	StoredCRC        string `json:"stored_crc"`
	CalculatedCRC    string `json:"calculated_crc"`
	OK               bool   `json:"ok"`
}

type SliceReport struct {
	Index         int    `json:"index"`
	FrameOffset   int    `json:"frame_offset"`
	Offset        int64  `json:"offset"`
	EndOffset     int64  `json:"end_offset"`
	Size          int    `json:"size"`
	SizeField     uint32 `json:"size_field"`
	FooterOffset  int64  `json:"footer_offset"`
	ErrorStatus   uint8  `json:"error_status,omitempty"`
	CRCEnabled    bool   `json:"crc_enabled"`
	CRCStored     string `json:"crc_stored,omitempty"`
	CRCCalculated string `json:"crc_calculated,omitempty"`
	CRCOK         *bool  `json:"crc_ok,omitempty"`
	BoundaryOK    bool   `json:"boundary_ok"`
}

type FrameReport struct {
	Index             int           `json:"index"`
	FrameNumber       int           `json:"frame_number"`
	BlockNumber       int           `json:"block_number"`
	TrackNumber       uint64        `json:"track_number"`
	ClusterOffset     int64         `json:"cluster_offset"`
	ClusterTimecode   int64         `json:"cluster_timecode,omitempty"`
	TimestampNS       int64         `json:"timestamp_ns"`
	TimestampSeconds  float64       `json:"timestamp_seconds"`
	BlockElementID    string        `json:"block_element_id"`
	ElementOffset     int64         `json:"element_offset"`
	BlockHeaderOffset int64         `json:"block_header_offset"`
	Position          int64         `json:"position"`
	SamplePosition    int64         `json:"sample_position"`
	FrameOffset       int64         `json:"frame_offset"`
	Size              int           `json:"size"`
	Keyframe          bool          `json:"keyframe"`
	KeyframeSource    string        `json:"keyframe_source,omitempty"`
	BlockFlags        string        `json:"block_flags,omitempty"`
	SHA256            string        `json:"sha256"`
	SliceCount        int           `json:"slice_count"`
	ExpectedSlices    int           `json:"expected_slices,omitempty"`
	SliceCountOK      *bool         `json:"slice_count_ok,omitempty"`
	CRCEnabled        bool          `json:"crc_enabled,omitempty"`
	CRCOK             *bool         `json:"crc_ok,omitempty"`
	Status            string        `json:"status"`
	Slices            []SliceReport `json:"slices,omitempty"`
	Issues            []Issue       `json:"issues,omitempty"`
}

type FrameHash struct {
	FrameIndex  int    `json:"frame_index"`
	FrameNumber int    `json:"frame_number"`
	Offset      int64  `json:"offset"`
	Size        int    `json:"size"`
	SHA256      string `json:"sha256"`
}

type FrameRange struct {
	StartFrameIndex  int   `json:"start_frame_index"`
	EndFrameIndex    int   `json:"end_frame_index"`
	StartFrameNumber int   `json:"start_frame_number"`
	EndFrameNumber   int   `json:"end_frame_number"`
	FrameCount       int   `json:"frame_count"`
	StartOffset      int64 `json:"start_offset"`
	EndOffset        int64 `json:"end_offset"`
}

type CorruptBlock struct {
	FrameIndex        int    `json:"frame_index,omitempty"`
	FrameNumber       int    `json:"frame_number,omitempty"`
	TrackNumber       uint64 `json:"track_number,omitempty"`
	ElementID         string `json:"element_id"`
	ElementOffset     int64  `json:"element_offset"`
	PayloadOffset     int64  `json:"payload_offset"`
	Size              int    `json:"size"`
	CorruptOffset     int64  `json:"corrupt_offset,omitempty"`
	CorruptEndOffset  int64  `json:"corrupt_end_offset,omitempty"`
	Kind              string `json:"kind"`
	Detail            string `json:"detail"`
	RecoverableSlices []int  `json:"recoverable_slices,omitempty"`
}

type Summary struct {
	Frames            int `json:"frames"`
	Keyframes         int `json:"keyframes"`
	CorruptFrames     int `json:"corrupt_frames"`
	OKFrames          int `json:"ok_frames"`
	UnknownElements   int `json:"unknown_elements"`
	Errors            int `json:"errors"`
	Warnings          int `json:"warnings"`
	RecoverableFrames int `json:"recoverable_frames"`
	RecoverableRanges int `json:"recoverable_ranges"`
}

type Report struct {
	SchemaVersion     string           `json:"schema_version"`
	Path              string           `json:"path"`
	FileSize          int64            `json:"file_size"`
	FileSHA256        string           `json:"file_sha256,omitempty"`
	DocType           string           `json:"doc_type,omitempty"`
	DocTypeVersion    int              `json:"doc_type_version,omitempty"`
	TimecodeScaleNS   uint64           `json:"timecode_scale_ns,omitempty"`
	Tracks            []TrackInfo      `json:"tracks,omitempty"`
	Frames            []FrameReport    `json:"frames"`
	FrameHashes       []FrameHash      `json:"frame_hashes"`
	CuePositions      []CuePosition    `json:"cue_positions,omitempty"`
	ContainerCRCs     []ContainerCRC   `json:"container_crcs,omitempty"`
	UnknownElements   []UnknownElement `json:"unknown_elements,omitempty"`
	Issues            []Issue          `json:"issues,omitempty"`
	CorruptBlocks     []CorruptBlock   `json:"corrupt_blocks,omitempty"`
	RecoverableRanges []FrameRange     `json:"recoverable_ranges,omitempty"`
	Summary           Summary          `json:"summary"`
}

type FrameExpectation struct {
	Index    *int   `json:"index,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Size     int    `json:"size,omitempty"`
	Position int64  `json:"position,omitempty"`
	Keyframe *bool  `json:"keyframe,omitempty"`
	Slices   int    `json:"slices,omitempty"`
}

type Expectations struct {
	ExpectedSlices int                `json:"expected_slices,omitempty"`
	Frames         []FrameExpectation `json:"frames,omitempty"`
}
