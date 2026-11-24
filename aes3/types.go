// Package aes3 recovers AES3/AES-EBU subframes from biphase-mark edges.
//
// The decoder is intentionally edge oriented and uses only the Go standard
// library. A valid preamble is the resynchronisation boundary: data before a
// loss and data after a later preamble are reported independently.
package aes3

// Level is a wire level when the capture supplies one. Unknown levels are
// reconstructed by toggling on every edge because BMC is polarity independent.
type Level int

const (
	LevelUnknown Level = -1
	LevelLow     Level = 0
	LevelHigh    Level = 1
)

// Edge is one captured signal transition.
//
// Time uses the caller's chosen coordinate. The decoder preserves that
// coordinate in reports. For logic-analyser sample ticks, set
// Options.HalfUnit in ticks or provide SampleRate to the file loader.
type Edge struct {
	Time  float64 `json:"time"`
	Level Level   `json:"level,omitempty"`
}

// Options controls decoding and clock recovery.
type Options struct {
	// HalfUnit is one half-bit time (0.5 AES3 time slot/UI). Zero enables
	// automatic recovery from edge intervals.
	HalfUnit float64 `json:"half_unit,omitempty"`

	// JitterTolerance is the maximum edge-time error divided by HalfUnit.
	// The default is 0.35, safely below one half-bit cell.
	JitterTolerance float64 `json:"jitter_tolerance,omitempty"`

	// MinSubframeScore controls candidate acceptance in [0,1]. Zero uses a
	// conservative default. Tests and noisy captures can lower it.
	MinSubframeScore float64 `json:"min_subframe_score,omitempty"`

	// RequireParity rejects candidates with a bad parity bit when true.
	// Normally parity errors are reported but do not prevent identification.
	RequireParity bool `json:"require_parity,omitempty"`
}

// LockInterval is a half-open range that cannot be decoded as locked AES3.
//
// Edge indexes refer to the time-sorted input. [StartEdge,EndEdge) contains
// edges after the last trusted boundary and before the relock preamble.
type LockInterval struct {
	Index        int     `json:"index"`
	StartEdge    int     `json:"start_edge"`
	EndEdge      int     `json:"end_edge"`
	StartTime    float64 `json:"start_time"`
	EndTime      float64 `json:"end_time"`
	Duration     float64 `json:"duration"`
	Reason       string  `json:"reason"`
	RelockCode   string  `json:"relock_code,omitempty"`
	MissingSlots int     `json:"missing_slots,omitempty"`
}

// Audio contains the linear two's-complement main field.
type Audio struct {
	Bits       []int `json:"bits"`
	Raw24      int32 `json:"raw24"`
	Sample24   int32 `json:"sample24"`
	Sample20   int32 `json:"sample20,omitempty"`
	Auxiliary4 int   `json:"auxiliary4,omitempty"`
}

// Subframe is one decoded 32-slot AES3 subframe.
type Subframe struct {
	Index          int     `json:"index"`
	Frame          int     `json:"frame"`
	SubframeNumber int     `json:"subframe_number"`
	Channel        string  `json:"channel"`
	ChannelLabel   string  `json:"channel_label"`
	Code           string  `json:"preamble"`
	BlockStart     bool    `json:"block_start"`
	StartTime      float64 `json:"start_time"`
	EndTime        float64 `json:"end_time"`
	StartEdge      int     `json:"start_edge"`
	HalfUnit       float64 `json:"half_unit"`
	Score          float64 `json:"score"`

	// SlotBits contains time slots 4..31, transmitted LSB-first where
	// applicable. Index 0 is slot 4.
	SlotBits []int `json:"slot_bits"`

	Audio Audio `json:"audio"`

	ValidityBit      bool `json:"validity_bit"`
	AudioValid       bool `json:"audio_valid"`
	UserBit          bool `json:"user_bit"`
	ChannelStatusBit bool `json:"channel_status_bit"`
	ParityBit        bool `json:"parity_bit"`
	ParityOK         bool `json:"parity_ok"`

	// Trusted means the preamble, BMC boundaries and parity check are good.
	// A structurally valid subframe with bad parity is still returned, but
	// is not folded into channel-status blocks.
	Trusted bool     `json:"trusted"`
	Errors  []string `json:"errors,omitempty"`
}

// StatusSegment is a gap-free run of per-subframe channel-status bits.
type StatusSegment struct {
	Channel       string  `json:"channel"`
	StartSubframe int     `json:"start_subframe"`
	EndSubframe   int     `json:"end_subframe"`
	StartTime     float64 `json:"start_time"`
	EndTime       float64 `json:"end_time"`
	Bits          []int   `json:"bits"`
	BytesLSBFirst []int   `json:"bytes_lsb_first"`
	Hex           string  `json:"hex"`
	BlockAligned  bool    `json:"block_aligned"`
}

// ChannelStatusReport keeps left/right state separate and split at losses.
type ChannelStatusReport struct {
	Left             []StatusSegment `json:"left"`
	Right            []StatusSegment `json:"right"`
	CompleteLeft192  []int           `json:"complete_left_192,omitempty"`
	CompleteRight192 []int           `json:"complete_right_192,omitempty"`
}

// Summary gives compact counts.
type Summary struct {
	EdgeCount     int `json:"edge_count"`
	Subframes     int `json:"subframes"`
	Left          int `json:"left"`
	Right         int `json:"right"`
	BlockStarts   int `json:"block_starts"`
	ParityErrors  int `json:"parity_errors"`
	Untrusted     int `json:"untrusted"`
	LockIntervals int `json:"lock_intervals"`
}

// ClockReport is the recovered BMC timing. AES3 time slots are one bit cell;
// BMC's shortest edge spacing is half of that cell.
type ClockReport struct {
	HalfBitTime float64 `json:"half_bit_time"`
	BitCellTime float64 `json:"bit_cell_time"`
	BitRate     float64 `json:"bit_rate_per_time_unit"`
	Source      string  `json:"source"`
}

// Report is the complete decode result.
type Report struct {
	Clock          ClockReport         `json:"clock"`
	HalfUnit       float64             `json:"half_unit"`
	HalfUnitSource string              `json:"half_unit_source"`
	Edges          int                 `json:"edges"`
	Subframes      []Subframe          `json:"subframes"`
	LockIntervals  []LockInterval      `json:"lock_intervals"`
	LossOfLock     []LockInterval      `json:"loss_of_lock_intervals"`
	ChannelStatus  ChannelStatusReport `json:"channel_status"`
	Summary        Summary             `json:"summary"`
}
