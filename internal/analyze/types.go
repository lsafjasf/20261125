package analyze

import "tearcheck/internal/detect"

const (
	SchemaVersion = "tear-evidence/v1"
	ToolVersion   = "1.0.0"
)

// Report is the deterministic evidence artifact produced by one analysis run.
// It contains no wall-clock time, absolute paths, or map iteration order.
type Report struct {
	Schema      string      `json:"schema"`
	ToolVersion string      `json:"tool_version"`
	Input       InputInfo   `json:"input"`
	Settings    Settings    `json:"settings"`
	Review      ReviewInfo  `json:"review"`
	Summary     Summary     `json:"summary"`
	Events      []Event     `json:"events"`
	Frames      []FrameInfo `json:"frames"`
}

type Settings = detect.Settings
type Seam = detect.Seam
type Band = detect.Band
type LightEdge = detect.LightEdge

type InputInfo struct {
	FrameCount        int    `json:"frame_count"`
	UniqueFrameCount  int    `json:"unique_frame_count"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	TimestampSource   string `json:"timestamp_source"`
	ManifestSHA256    string `json:"manifest_sha256"`
	ManifestFrameRate string `json:"manifest_frame_rate,omitempty"`
}

type ReviewInfo struct {
	Applied    int    `json:"applied"`
	FileSHA256 string `json:"file_sha256"`
}

type Summary struct {
	Frames                   int `json:"frames"`
	UniqueFrames             int `json:"unique_frames"`
	DuplicateFrames          int `json:"duplicate_frames"`
	MachineTearFrames        int `json:"machine_tear_frames"`
	MachineUncertainFrames   int `json:"machine_uncertain_frames"`
	MachineLightEffectFrames int `json:"machine_light_effect_frames"`
	MachineCleanFrames       int `json:"machine_clean_frames"`
	FinalTearFrames          int `json:"final_tear_frames"`
	FinalNotTearFrames       int `json:"final_not_tear_frames"`
	FinalUncertainFrames     int `json:"final_uncertain_frames"`
	FinalDuplicateFrames     int `json:"final_duplicate_frames"`
	ReviewedFrames           int `json:"reviewed_frames"`
	TearEvents               int `json:"tear_events"`
	ContinuousEvents         int `json:"continuous_events"`
	IntermittentEvents       int `json:"intermittent_events"`
	IsolatedEvents           int `json:"isolated_events"`
	MaxWidthPX               int `json:"max_width_px"`
}

type FrameInfo struct {
	ID           string          `json:"id"`
	Index        int             `json:"index"`
	File         string          `json:"file"`
	PTSUS        int64           `json:"pts_us"`
	PTSMS        int64           `json:"pts_ms"`
	Timecode     string          `json:"timecode"`
	DurationUS   int64           `json:"duration_us,omitempty"`
	Hash         string          `json:"hash"`
	Duplicate    bool            `json:"duplicate"`
	DuplicateOf  string          `json:"duplicate_of,omitempty"`
	DuplicateRun int             `json:"duplicate_run,omitempty"`
	Machine      MachineResult   `json:"machine"`
	Review       *ReviewDecision `json:"review,omitempty"`
	Final        string          `json:"final"`
}

type MachineResult struct {
	Classification string      `json:"classification"`
	ConfidenceBP   int         `json:"confidence_bp"`
	WidthPX        int         `json:"width_px"`
	Continuity     string      `json:"continuity"`
	EventID        string      `json:"event_id,omitempty"`
	Seams          []Seam      `json:"seams,omitempty"`
	Bands          []Band      `json:"bands,omitempty"`
	LightEdges     []LightEdge `json:"light_edges,omitempty"`
}

type Event struct {
	ID                  string       `json:"id"`
	StartPTSUS          int64        `json:"start_pts_us"`
	EndPTSUS            int64        `json:"end_pts_us"`
	StartIndex          int          `json:"start_index"`
	EndIndex            int          `json:"end_index"`
	FrameIDs            []string     `json:"frame_ids"`
	Continuity          string       `json:"continuity"`
	WidthPX             int          `json:"width_px"`
	MinWidthPX          int          `json:"min_width_px"`
	MaxWidthPX          int          `json:"max_width_px"`
	Evidence            []EventFrame `json:"evidence"`
	FinalClassification string       `json:"final_classification"`
	ReviewedFrames      int          `json:"reviewed_frames"`
}

type EventFrame struct {
	FrameID        string `json:"frame_id"`
	PTSUS          int64  `json:"pts_us"`
	Timecode       string `json:"timecode"`
	Row            int    `json:"row"`
	OffsetPX       int    `json:"offset_px"`
	WidthPX        int    `json:"width_px"`
	MatchScoreBP   int    `json:"match_score_bp"`
	ShiftGainBP    int    `json:"shift_gain_bp"`
	BiasRatioBP    int    `json:"bias_ratio_bp"`
	StructuredDiff int    `json:"structured_diff"`
}

type ReviewDecision struct {
	Decision string `json:"decision"`
	Note     string `json:"note,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
}
