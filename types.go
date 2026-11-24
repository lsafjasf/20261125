package mixloud

type Options struct {
	WindowMS          int     `json:"window_ms"`
	HopMS             int     `json:"hop_ms"`
	SilenceDBFS       float64 `json:"silence_dbfs"`
	MinSilenceSec     float64 `json:"min_silence_sec"`
	DialogueThreshold float64 `json:"dialogue_threshold"`
	AdJumpLU          float64 `json:"ad_jump_lu"`
	AdMinSec          float64 `json:"ad_min_sec"`
	PhaseThreshold    float64 `json:"phase_threshold"`
	PeakLimitDBFS     float64 `json:"peak_limit_dbfs"`
	DialogueDuckDB    float64 `json:"dialogue_duck_db"`
	IncludeWindows    bool    `json:"include_windows"`
}

func DefaultOptions() Options {
	return Options{
		WindowMS:          400,
		HopMS:             100,
		SilenceDBFS:       -70,
		MinSilenceSec:     0.25,
		DialogueThreshold: 0.55,
		AdJumpLU:          8,
		AdMinSec:          0.6,
		PhaseThreshold:    -0.5,
		PeakLimitDBFS:     -1.0,
		DialogueDuckDB:    -6.0,
		IncludeWindows:    true,
	}
}

type Window struct {
	Index               int      `json:"index"`
	StartSec            float64  `json:"start_sec"`
	EndSec              float64  `json:"end_sec"`
	LoudnessLUFS        *float64 `json:"loudness_lufs"`
	RMSDBFS             *float64 `json:"rms_dbfs"`
	PeakDBFS            *float64 `json:"peak_dbfs"`
	Correlation         *float64 `json:"correlation,omitempty"`
	MonoCompatibilityDB *float64 `json:"mono_compatibility_db,omitempty"`
	CenterShare         *float64 `json:"center_share,omitempty"`
	DialogueScore       float64  `json:"dialogue_score"`
	Dialogue            bool     `json:"dialogue"`
	Silence             bool     `json:"silence"`
	PhaseInverted       bool     `json:"phase_inverted"`
	ClippedSamples      int64    `json:"clipped_samples"`
	NearClipSamples     int64    `json:"near_clip_samples"`
}

type Metrics struct {
	Scheme                 string   `json:"scheme"`
	Label                  string   `json:"label"`
	SampleRate             int      `json:"sample_rate"`
	Channels               int      `json:"channels"`
	DurationSec            float64  `json:"duration_sec"`
	WindowMS               int      `json:"window_ms"`
	HopMS                  int      `json:"hop_ms"`
	WindowCount            int      `json:"window_count"`
	IntegratedLUFS         *float64 `json:"integrated_lufs"`
	LRA400MS               *float64 `json:"lra_400ms"`
	LRA10                  *float64 `json:"lra_p10_lufs,omitempty"`
	LRA95                  *float64 `json:"lra_p95_lufs,omitempty"`
	LRAEBU3S               *float64 `json:"lra_ebu_3s,omitempty"`
	MaxWindowLUFS          *float64 `json:"max_400ms_lufs"`
	P95WindowLUFS          *float64 `json:"p95_400ms_lufs,omitempty"`
	MedianWindowLUFS       *float64 `json:"median_400ms_lufs,omitempty"`
	MaxPeakDBFS            *float64 `json:"max_peak_dbfs"`
	ClippedSamples         int64    `json:"clipped_samples"`
	NearClipSamples        int64    `json:"near_clip_samples"`
	DialogueDurationSec    float64  `json:"dialogue_duration_sec"`
	DialogueRatioTotal     float64  `json:"dialogue_ratio_total"`
	DialogueRatioActive    float64  `json:"dialogue_ratio_active"`
	ActiveDurationSec      float64  `json:"active_duration_sec"`
	DialogueClassification string   `json:"dialogue_classification"`
}

type Event struct {
	Kind          string   `json:"kind"`
	Severity      string   `json:"severity"`
	StartSec      float64  `json:"start_sec"`
	EndSec        float64  `json:"end_sec"`
	DurationSec   float64  `json:"duration_sec"`
	PeakDBFS      *float64 `json:"peak_dbfs,omitempty"`
	LoudnessLUFS  *float64 `json:"loudness_lufs,omitempty"`
	BaselineLUFS  *float64 `json:"baseline_lufs,omitempty"`
	DeltaLU       *float64 `json:"delta_lu,omitempty"`
	Correlation   *float64 `json:"correlation,omitempty"`
	MonoCompatDB  *float64 `json:"mono_compatibility_db,omitempty"`
	CrestFactorDB *float64 `json:"crest_factor_db,omitempty"`
	Message       string   `json:"message"`
}

type SchemeReport struct {
	Metrics         Metrics  `json:"metrics"`
	Windows         []Window `json:"windows,omitempty"`
	SilenceSegments []Event  `json:"silence_segments"`
	AdIntrusions    []Event  `json:"ad_intrusions"`
	PhaseInversions []Event  `json:"phase_inversions"`
	BurstEvents     []Event  `json:"burst_events"`
}

type Report struct {
	Input    string         `json:"input"`
	Options  Options        `json:"options"`
	Notes    []string       `json:"notes,omitempty"`
	Verdicts []string       `json:"verdicts"`
	Schemes  []SchemeReport `json:"schemes"`
	Stems    []SchemeReport `json:"stems,omitempty"`
}

func floatPtr(v float64) *float64 {
	if !finite(v) {
		return nil
	}
	x := v
	return &x
}

func finite(v float64) bool {
	return v == v && v > -1e300 && v < 1e300
}
