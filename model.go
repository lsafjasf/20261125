package mp4diag

const SchemaVersion = 1

const (
	LayerContainer   = "container"
	LayerSampleTable = "sample_table"
	LayerCompressed  = "compressed_data"
	LayerDisplay     = "display"
)

const (
	StatusOK          = "ok"
	StatusError       = "error"
	StatusWarning     = "warning"
	StatusNotAssessed = "not_assessed"
	PixelConfirmed    = "confirmed"
	PixelRuledOut     = "ruled_out"
	PixelNotAssessed  = "not_assessed"
	PixelSuspected    = "suspected"
	SeverityError     = "error"
	SeverityWarning   = "warning"
	SeverityInfo      = "info"
)

type Span struct {
	First int `json:"first"`
	Last  int `json:"last"`
}

type ByteSpan struct {
	Offset int64 `json:"offset"`
	Length int64 `json:"length"`
}

type BoxLocator struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Offset int64  `json:"offset"`
	Field  string `json:"field,omitempty"`
	Entry  int    `json:"entry,omitempty"`
}

type BoxInfo struct {
	Path         string `json:"path"`
	Type         string `json:"type"`
	Offset       int64  `json:"offset"`
	HeaderSize   int64  `json:"header_size"`
	DeclaredSize int64  `json:"declared_size"`
	DeclaredEnd  int64  `json:"declared_end"`
	ActualEnd    int64  `json:"actual_end,omitempty"`
	Valid        bool   `json:"valid"`
	IssueCode    string `json:"issue_code,omitempty"`
	Issue        string `json:"issue,omitempty"`
}

type Finding struct {
	ID          string      `json:"id"`
	Layer       string      `json:"layer"`
	Severity    string      `json:"severity"`
	Code        string      `json:"code"`
	Message     string      `json:"message"`
	Box         *BoxLocator `json:"box,omitempty"`
	Samples     []int       `json:"samples,omitempty"`
	Spans       []Span      `json:"spans,omitempty"`
	Bytes       []ByteSpan  `json:"bytes,omitempty"`
	PixelDamage string      `json:"pixel_damage,omitempty"`
}

type SampleInfo struct {
	Number          int    `json:"number"`
	TrackID         uint32 `json:"track_id"`
	Chunk           int    `json:"chunk"`
	Offset          int64  `json:"offset"`
	Size            uint32 `json:"size"`
	Available       bool   `json:"available"`
	OutsideMDAT     bool   `json:"outside_mdat,omitempty"`
	Sync            bool   `json:"sync"`
	DTS             int64  `json:"dts"`
	PTS             int64  `json:"pts"`
	DisplayOrder    int    `json:"display_order"`
	CTSOffset       int64  `json:"cts_offset,omitempty"`
	NALLengthSize   int    `json:"nal_length_size,omitempty"`
	NALTypes        []int  `json:"nal_types,omitempty"`
	ReferenceStatus string `json:"reference_status,omitempty"`
}

type SampleComparison struct {
	Sample       int    `json:"sample"`
	RefOffset    int64  `json:"ref_offset"`
	RefSize      uint32 `json:"ref_size"`
	CandidateEnd int64  `json:"candidate_end"`
	Status       string `json:"status"`
	RefSHA256    string `json:"ref_sha256,omitempty"`
	ActualSHA256 string `json:"actual_sha256,omitempty"`
}

type ReferenceReport struct {
	Path              string             `json:"path"`
	PixelDamage       string             `json:"pixel_damage"`
	ChangedSamples    []int              `json:"changed_samples,omitempty"`
	MissingSamples    []int              `json:"missing_samples,omitempty"`
	IntactSamples     []int              `json:"intact_samples,omitempty"`
	SampleComparisons []SampleComparison `json:"sample_comparisons,omitempty"`
}

type DisplayImpact struct {
	FaultSamples            []int  `json:"fault_samples,omitempty"`
	AffectedSamples         []int  `json:"affected_samples,omitempty"`
	AffectedDisplayFrames   []int  `json:"affected_display_frames,omitempty"`
	CleanSampleRanges       []Span `json:"clean_sample_ranges,omitempty"`
	CleanDisplayFrameRanges []Span `json:"clean_display_frame_ranges,omitempty"`
	NextSyncSample          int    `json:"next_sync_sample,omitempty"`
	Method                  string `json:"method"`
	Confidence              string `json:"confidence"`
	Note                    string `json:"note,omitempty"`
}

type LayerSummary struct {
	Status      string      `json:"status"`
	Summary     string      `json:"summary"`
	FindingIDs  []string    `json:"finding_ids,omitempty"`
	FirstBadBox *BoxLocator `json:"first_bad_box,omitempty"`
}

type Report struct {
	SchemaVersion int                     `json:"schema_version"`
	Path          string                  `json:"path"`
	FileSize      int64                   `json:"file_size"`
	BoxCount      int                     `json:"box_count"`
	FirstBadBox   *BoxLocator             `json:"first_bad_box,omitempty"`
	Layers        map[string]LayerSummary `json:"layers"`
	Findings      []Finding               `json:"findings,omitempty"`
	Boxes         []BoxInfo               `json:"boxes,omitempty"`
	Samples       []SampleInfo            `json:"samples,omitempty"`
	Reference     *ReferenceReport        `json:"reference,omitempty"`
	Display       DisplayImpact           `json:"display"`
}
