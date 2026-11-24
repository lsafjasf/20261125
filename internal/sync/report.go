package sync

// Anomaly is a frame-localized discontinuity or mapping error.
type Anomaly struct {
	Frame         int64  `json:"frame"`
	Stage         Stage  `json:"stage,omitempty"`
	Series        string `json:"series,omitempty"`
	Kind          string `json:"kind"`
	Message       string `json:"message"`
	DeltaNS       int64  `json:"delta_ns,omitempty"`
	JumpNS        int64  `json:"jump_ns,omitempty"`
	MissingFrames int64  `json:"missing_frames,omitempty"`
}

// FrameRef points at the first frame that violated the expected trajectory.
type FrameRef struct {
	Frame  int64  `json:"frame"`
	Stage  Stage  `json:"stage,omitempty"`
	Series string `json:"series,omitempty"`
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}

// StageSummary describes one pipeline position track.
type StageSummary struct {
	Stage           Stage  `json:"stage"`
	Samples         int    `json:"samples"`
	FirstFrame      int64  `json:"first_frame,omitempty"`
	LastFrame       int64  `json:"last_frame,omitempty"`
	FirstPositionNS *int64 `json:"first_position_ns,omitempty"`
	LastPositionNS  *int64 `json:"last_position_ns,omitempty"`
	PositionJumps   int    `json:"position_jumps"`
	FrameIndexGaps  int    `json:"frame_index_gaps"`
	OutputIndexGaps int    `json:"output_index_gaps"`
	FrameStride     int64  `json:"frame_stride"`
	OutputStride    int64  `json:"output_stride,omitempty"`
}

// PositionRow is the frame-correlated media position table.
type PositionRow struct {
	Frame       int64           `json:"frame"`
	OutputFrame *int64          `json:"output_frame,omitempty"`
	Positions   map[Stage]int64 `json:"positions_ns"`
	Video       map[Stage]int64 `json:"video_positions_ns,omitempty"`
	Audio       map[Stage]int64 `json:"audio_positions_ns,omitempty"`
	LipOffset   map[Stage]int64 `json:"lip_offset_audio_minus_video_ns,omitempty"`
}

// Report is the complete staged diagnosis.
type Report struct {
	Schema         string  `json:"schema"`
	Options        Options `json:"options"`
	Classification string  `json:"classification"`
	Confidence     string  `json:"confidence"`
	Summary        string  `json:"summary"`

	FirstAbnormalFrame        *FrameRef `json:"first_abnormal_frame,omitempty"`
	FirstRootCauseFrame       *FrameRef `json:"first_root_cause_frame,omitempty"`
	FirstPlaybackAnomalyFrame *FrameRef `json:"first_playback_anomaly_frame,omitempty"`

	ObservedFinalOffsetNS    int64   `json:"observed_final_offset_ns,omitempty"`
	EstimatedDriftNSPerFrame float64 `json:"estimated_drift_ns_per_frame,omitempty"`
	EstimatedSkippedFrames   int64   `json:"estimated_skipped_frames,omitempty"`
	EstimatedOnsetFrame      *int64  `json:"estimated_onset_frame,omitempty"`

	StageSummaries []StageSummary `json:"stage_summaries"`
	Series         []SeriesReport `json:"series"`
	Anomalies      []Anomaly      `json:"anomalies"`
	EvidenceRows   []PositionRow  `json:"evidence_rows,omitempty"`
}
