package vdetect

// Point is an integer pixel coordinate in the source frame.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Box is an axis-aligned rectangle in source-frame pixels.
type Box struct {
	X      int     `json:"x"`
	Y      int     `json:"y"`
	W      int     `json:"w"`
	H      int     `json:"h"`
	Conf   float64 `json:"confidence,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

// Tear is one coherent intra-frame discontinuity.
type Tear struct {
	Y             int      `json:"y"`
	YEnd          int      `json:"y_end,omitempty"`
	XStart        int      `json:"x_start"`
	XEnd          int      `json:"x_end"`
	Coverage      float64  `json:"coverage"`
	ShiftPx       float64  `json:"shift_px"`
	ShiftSamples  float64  `json:"shift_samples,omitempty"`
	WidthPx       int      `json:"width_px"`
	Mismatch      float64  `json:"mismatch"`
	AlignedCost   float64  `json:"aligned_cost"`
	ZeroCost      float64  `json:"zero_cost"`
	Texture       float64  `json:"texture"`
	SupportBlocks int      `json:"support_blocks"`
	TotalBlocks   int      `json:"total_blocks"`
	Confidence    float64  `json:"confidence"`
	TrackID       int      `json:"track_id,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
}

// FieldOrder contains alternating-row/field parity evidence.
type FieldOrder struct {
	Detected      bool     `json:"detected"`
	Confidence    float64  `json:"confidence"`
	ParityShiftPx float64  `json:"parity_shift_px"`
	CombScore     float64  `json:"comb_score"`
	RowSupport    float64  `json:"row_support"`
	SupportedRows int      `json:"supported_rows"`
	TexturedRows  int      `json:"textured_rows"`
	TrackID       int      `json:"track_id,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
}

// BlockDefect is a grid-aligned coding/macroblock anomaly.
type BlockDefect struct {
	Box
	BlockSize   int      `json:"block_size"`
	Boundary    float64  `json:"boundary_score"`
	Internal    float64  `json:"internal_score"`
	ColorScore  float64  `json:"color_score,omitempty"`
	Temporal    float64  `json:"temporal_score,omitempty"`
	GridAligned bool     `json:"grid_aligned"`
	TrackID     int      `json:"track_id,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
}

// EffectRegion describes benign/motion/effect activity used by guards.
type EffectRegion struct {
	Box
	Kind       string  `json:"kind"`
	Motion     float64 `json:"motion,omitempty"`
	Brightness float64 `json:"brightness_delta,omitempty"`
}

// Guards records false-positive suppression signals.
type Guards struct {
	MeanLuma             float64 `json:"mean_luma"`
	LumaDelta            float64 `json:"luma_delta"`
	BrightnessSlope      float64 `json:"brightness_slope,omitempty"`
	GlobalBrightening    bool    `json:"global_brightening"`
	MotionCoverage       float64 `json:"motion_coverage"`
	ForegroundCoverage   float64 `json:"foreground_coverage"`
	EffectCoverage       float64 `json:"effect_coverage"`
	CameraShiftPx        float64 `json:"camera_shift_px,omitempty"`
	SuppressedCandidates int     `json:"suppressed_candidates"`
}

// FrameResult is the complete per-frame diagnostic record.
type FrameResult struct {
	Index          int            `json:"frame"`
	PTSSeconds     float64        `json:"pts_seconds,omitempty"`
	Width          int            `json:"width"`
	Height         int            `json:"height"`
	Classification string         `json:"classification"`
	Labels         []string       `json:"labels,omitempty"`
	Confidence     float64        `json:"confidence"`
	Tears          []Tear         `json:"tears"`
	FieldOrder     *FieldOrder    `json:"field_order,omitempty"`
	Blocks         []BlockDefect  `json:"block_defects,omitempty"`
	Effects        []EffectRegion `json:"effects,omitempty"`
	Guards         Guards         `json:"guards"`
	TrackIDs       []int          `json:"track_ids,omitempty"`
	Notes          []string       `json:"notes,omitempty"`
}

// TrackPoint stores one frame observation in a cross-frame track.
type TrackPoint struct {
	Frame      int     `json:"frame"`
	PTSSeconds float64 `json:"pts_seconds,omitempty"`
	Y          int     `json:"y,omitempty"`
	X          int     `json:"x,omitempty"`
	W          int     `json:"w,omitempty"`
	H          int     `json:"h,omitempty"`
	ShiftPx    float64 `json:"shift_px,omitempty"`
	Temporal   float64 `json:"temporal_score,omitempty"`
	Confidence float64 `json:"confidence"`
}

// Track links related defects across frames.
type Track struct {
	ID             int          `json:"id"`
	Kind           string       `json:"kind"`
	Classification string       `json:"classification"`
	StartFrame     int          `json:"start_frame"`
	EndFrame       int          `json:"end_frame"`
	FrameCount     int          `json:"observation_count"`
	GapFrames      int          `json:"gap_frames"`
	Continuity     float64      `json:"continuity"`
	Confidence     float64      `json:"confidence"`
	YStart         int          `json:"y_start,omitempty"`
	YEnd           int          `json:"y_end,omitempty"`
	VelocityYPx    float64      `json:"velocity_y_per_frame,omitempty"`
	MonotonicRatio float64      `json:"monotonic_ratio,omitempty"`
	WrapCount      int          `json:"wrap_count,omitempty"`
	Positions      []TrackPoint `json:"positions"`
}

// Summary contains report-level counts.
type Summary struct {
	Frames             int     `json:"frames"`
	FramesWithFindings int     `json:"frames_with_findings"`
	TearFrames         int     `json:"tear_frames"`
	FieldOrderFrames   int     `json:"field_order_frames"`
	RollingFrames      int     `json:"rolling_frames"`
	BlockFrames        int     `json:"block_frames"`
	EffectFrames       int     `json:"effect_frames"`
	Tracks             int     `json:"tracks"`
	MaxConfidence      float64 `json:"max_confidence"`
}

// Report is the versioned JSON report.
type Report struct {
	SchemaVersion string        `json:"schema_version"`
	Tool          string        `json:"tool"`
	Input         string        `json:"input"`
	Width         int           `json:"width"`
	Height        int           `json:"height"`
	FPS           float64       `json:"fps,omitempty"`
	Options       Options       `json:"options"`
	Summary       Summary       `json:"summary"`
	Frames        []FrameResult `json:"frames"`
	Tracks        []Track       `json:"tracks"`
}

// Options controls detection sensitivity and output limits.
type Options struct {
	Width             int     `json:"width,omitempty"`
	Height            int     `json:"height,omitempty"`
	PixelFormat       string  `json:"pixel_format,omitempty"`
	FPS               float64 `json:"fps,omitempty"`
	Sensitivity       float64 `json:"sensitivity"`
	MinConfidence     float64 `json:"min_confidence"`
	AnalysisWidth     int     `json:"analysis_width"`
	MaxShiftRatio     float64 `json:"max_shift_ratio"`
	NativeVideo       bool    `json:"-"`
	KeepLowConfidence bool    `json:"keep_low_confidence,omitempty"`
}

// DefaultOptions returns tuned defaults.
func DefaultOptions() Options {
	return Options{
		PixelFormat:   "gray",
		FPS:           30,
		Sensitivity:   0.62,
		MinConfidence: 0.55,
		AnalysisWidth: 448,
		MaxShiftRatio: 0.09,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
