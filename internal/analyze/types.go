package analyze

import "time"

type ChannelStats struct {
	Index      int     `json:"index"`
	Name       string  `json:"name"`
	RMSDB      float64 `json:"rms_dbfs"`
	PeakDB     float64 `json:"peak_dbfs"`
	CrestDB    float64 `json:"crest_db"`
	LowRatio   float64 `json:"low_20_120_ratio"`
	VoiceRatio float64 `json:"voice_300_3400_ratio"`
	HighRatio  float64 `json:"high_8k_plus_ratio"`
	Silent     float64 `json:"silent_fraction"`
}

type PairResult struct {
	A             int     `json:"a_index"`
	B             int     `json:"b_index"`
	AChannel      string  `json:"a_channel"`
	BChannel      string  `json:"b_channel"`
	LagSamples    int     `json:"b_lag_samples"`
	LagMS         float64 `json:"b_lag_ms"`
	Correlation   float64 `json:"correlation"`
	EnvelopeCorr  float64 `json:"envelope_correlation"`
	Inverted      bool    `json:"mirror_polarity"`
	MedianLag     int     `json:"median_b_lag_samples"`
	VoteWarnings  int     `json:"warning_windows"`
	ActiveWindows int     `json:"active_windows"`
}

type DelayPair struct {
	A            int
	B            int
	LagSamples   int
	LagMS        float64
	Correlation  float64
	EnvelopeCorr float64
}

type WindowChannel struct {
	Index      int     `json:"index"`
	Name       string  `json:"name"`
	RMSDB      float64 `json:"rms_dbfs"`
	PeakDB     float64 `json:"peak_dbfs"`
	LowRatio   float64 `json:"low_20_120_ratio"`
	VoiceRatio float64 `json:"voice_300_3400_ratio"`
	HighRatio  float64 `json:"high_8k_plus_ratio"`
}

type WindowPair struct {
	A            int     `json:"a_index"`
	B            int     `json:"b_index"`
	LagSamples   int     `json:"b_lag_samples"`
	Correlation  float64 `json:"correlation"`
	EnvelopeCorr float64 `json:"envelope_correlation"`
	Valid        bool    `json:"valid"`
}

type Window struct {
	Index    int             `json:"index"`
	Start    float64         `json:"start_seconds"`
	End      float64         `json:"end_seconds"`
	Channels []WindowChannel `json:"channels"`
	Pairs    []WindowPair    `json:"pairs,omitempty"`
}

type Finding struct {
	Severity   string   `json:"severity"`
	Code       string   `json:"code"`
	Channels   []string `json:"channels"`
	Start      float64  `json:"start_seconds"`
	End        float64  `json:"end_seconds"`
	Message    string   `json:"message"`
	RootCause  string   `json:"possible_root_cause"`
	Confidence float64  `json:"confidence"`
	Evidence   string   `json:"evidence"`
}

type ReferenceWindow struct {
	Start           float64
	End             float64
	OutputChannel   int
	ExpectedChannel int
	MappedChannel   int
	LagSamples      int
	ExpectedCorr    float64
	ExpectedEnv     float64
	MappedCorr      float64
	MappedEnv       float64
}

type ChannelMapping struct {
	OutputIndex      int     `json:"output_index"`
	OutputChannel    string  `json:"output_channel"`
	ReferenceIndex   int     `json:"reference_index"`
	ReferenceChannel string  `json:"reference_channel"`
	LagSamples       int     `json:"reference_lag_samples"`
	Correlation      float64 `json:"correlation"`
	EnvelopeCorr     float64 `json:"envelope_correlation"`
	Confidence       float64 `json:"confidence"`
	Status           string  `json:"status"`
}

type Report struct {
	File              string           `json:"file"`
	Reference         string           `json:"reference,omitempty"`
	SampleRate        int              `json:"sample_rate"`
	Channels          int              `json:"channels"`
	Bits              int              `json:"bits"`
	Duration          float64          `json:"duration_seconds"`
	Frames            int              `json:"frames"`
	Layout            []string         `json:"declared_layout"`
	LayoutSource      string           `json:"layout_source"`
	ChannelMask       string           `json:"channel_mask"`
	Stats             []ChannelStats   `json:"channel_stats"`
	ReferenceStats    []ChannelStats   `json:"reference_stats,omitempty"`
	ChannelMap        []ChannelMapping `json:"channel_mapping,omitempty"`
	Pairs             []PairResult     `json:"strong_pairs"`
	Findings          []Finding        `json:"findings"`
	WideField         WideFieldCheck   `json:"wide_field_guard"`
	UncorrelatedPairs int              `json:"uncorrelated_pairs"`
	AnalysisNote      string           `json:"analysis_note"`
}

type WideFieldCheck struct {
	CheckedPairs []string `json:"checked_pairs"`
	NegativeCorr float64  `json:"most_negative_correlation"`
	MirrorPairs  int      `json:"mirror_evidence_pairs"`
	Verdict      string   `json:"verdict"`
}

type Config struct {
	WindowSeconds float64
	HopSeconds    float64
	MaxDelayMS    float64
	AnalysisStart time.Duration
	AnalysisEnd   time.Duration
}

func DefaultConfig() Config {
	return Config{WindowSeconds: 1, HopSeconds: 1, MaxDelayMS: 20}
}
