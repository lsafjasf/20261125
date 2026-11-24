// Package hdrmeta selects dynamic HDR metadata by presentation time.
//
// Metadata is indexed by the PTS at which it is allowed to reach the screen,
// not by scene number or packet arrival order. Scene numbers may repeat, so a
// monotonically assigned generation identifies the parameter set actually
// adopted by every presented frame.
package hdrmeta

import "time"

// Tick is one presentation-timestamp unit. PTSRate is the conventional MPEG
// 90 kHz video clock, but callers may use any consistent integer time base.
type Tick int64

// PTSRate is the number of PTS ticks in one second.
const PTSRate Tick = 90_000

// TickFromDuration converts a duration to 90 kHz PTS ticks.
func TickFromDuration(d time.Duration) Tick {
	seconds := int64(d / time.Second)
	remainder := int64(d % time.Second)
	return Tick(seconds*int64(PTSRate) +
		remainder*int64(PTSRate)/int64(time.Second))
}

// Duration converts 90 kHz PTS ticks back to a duration.
func (t Tick) Duration() time.Duration {
	seconds := int64(t / PTSRate)
	remainder := int64(t % PTSRate)
	return time.Duration(seconds)*time.Second +
		time.Duration(remainder)*time.Second/time.Duration(PTSRate)
}

// Frame is a presented video frame. Number is used for deterministic gap
// detection; PTS is authoritative for metadata selection.
type Frame struct {
	Number int64 `json:"number"`
	PTS    Tick  `json:"pts"`
}

// Metadata is one dynamically updated HDR parameter block.
//
// PTS is the display moment for which the block is valid. UpdatedAt is the
// source's continuous update timestamp and orders revisions sharing a PTS.
// Scene may repeat; it is audit context rather than the state-machine key.
type Metadata[P any] struct {
	Scene       int64 `json:"scene"`
	FrameNumber int64 `json:"frame_number"`
	PTS         Tick  `json:"pts"`
	UpdatedAt   int64 `json:"updated_at"`
	Parameters  P     `json:"parameters"`
}

// Params is a small concrete HDR parameter set for callers that do not need
// their own payload type. Generic StateMachine users may replace it.
type Params struct {
	PeakNits    int    `json:"peak_nits"`
	AverageNits int    `json:"average_nits"`
	ToneCurveID int    `json:"tone_curve_id"`
	Label       string `json:"label,omitempty"`
}

// Generation is an immutable parameter revision accepted by the machine.
type Generation[P any] struct {
	// ID is the machine-assigned, monotonically increasing parameter epoch.
	ID int64 `json:"id"`
	Metadata[P]
}

// State is the query/presentation state at a display moment.
type State string

const (
	// StateIdle means no metadata has been ingested.
	StateIdle State = "idle"
	// StateWaiting means metadata exists, but every block is in the future.
	StateWaiting State = "waiting_for_activation"
	// StateActive means a metadata generation is eligible at the display time.
	StateActive State = "active"
)

// Adoption describes how a presented frame related to the previous frame's
// adopted generation.
type Adoption string

const (
	// AdoptionNone means no generation was available at the frame PTS.
	AdoptionNone Adoption = "none"
	// AdoptionNew means the frame adopted a different eligible generation.
	AdoptionNew Adoption = "new_generation"
	// AdoptionHeld means the previous eligible generation remained active,
	// including across missing frames or missing per-frame metadata.
	AdoptionHeld Adoption = "held_generation"
)

// FrameGap describes absent frame numbers between two presented frames.
type FrameGap struct {
	PreviousFrame int64 `json:"previous_frame,omitempty"`
	CurrentFrame  int64 `json:"current_frame,omitempty"`
	From          int64 `json:"from,omitempty"`
	To            int64 `json:"to,omitempty"`
	Count         int64 `json:"count"`
}

// Missing reports whether at least one frame number was skipped.
func (g FrameGap) Missing() bool { return g.Count > 0 }

// Numbers returns the skipped frame numbers. Avoid it for unbounded gaps;
// use Count and the range fields for large discontinuities.
func (g FrameGap) Numbers() []int64 {
	if g.Count <= 0 {
		return nil
	}
	out := make([]int64, g.Count)
	for i := range out {
		out[i] = g.From + int64(i)
	}
	return out
}

// Snapshot is a point-in-time metadata query result.
type Snapshot[P any] struct {
	At           Tick          `json:"at"`
	Found        bool          `json:"found"`
	State        State         `json:"state"`
	GenerationID int64         `json:"generation_id,omitempty"`
	Generation   Generation[P] `json:"generation,omitempty"`
}

// FrameRecord is the immutable audit entry for one actual presentation.
type FrameRecord[P any] struct {
	AuditID              int64         `json:"audit_id"`
	Frame                Frame         `json:"frame"`
	State                State         `json:"state"`
	Adoption             Adoption      `json:"adoption"`
	Found                bool          `json:"found"`
	GenerationID         int64         `json:"generation_id"`
	PreviousGenerationID int64         `json:"previous_generation_id"`
	Generation           Generation[P] `json:"generation,omitempty"`
	Gap                  FrameGap      `json:"gap,omitempty"`
}

// IngestResult reports whether an update was new, an idempotent duplicate, or
// a correction carrying the same natural update key.
type IngestResult[P any] struct {
	Generation Generation[P] `json:"generation"`
	Duplicate  bool          `json:"duplicate"`
	Corrected  bool          `json:"corrected"`
}

// Machine is the concrete-payload convenience alias.
type Machine = StateMachine[Params]

// NewMachine creates a state machine using the built-in Params payload.
func NewMachine() *Machine {
	return New[Params]()
}
