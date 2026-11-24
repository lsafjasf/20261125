package vfr

import (
	"errors"
	"math/big"
	"sort"
)

// ExactTime is a timeline position expressed as an exact rational number of
// ticks. Most mapping math is not exactly representable in integer ticks
// (e.g. 1 tick of source at 2x speed maps to 0.5 output ticks), so the
// reversible mapping keeps a big.Rat alongside the floored integer value.
type ExactTime struct {
	rat *big.Rat
}

// NewExactTime builds an exact time from integer ticks.
func NewExactTime(t Ticks) ExactTime {
	return ExactTime{rat: big.NewRat(int64(t), 1)}
}

// Rat returns the underlying rational value.
func (e ExactTime) Rat() *big.Rat {
	if e.rat == nil {
		return big.NewRat(0, 1)
	}
	return new(big.Rat).Set(e.rat)
}

// Ticks floors the exact value to the integer tick grid.
func (e ExactTime) Ticks() Ticks {
	if e.rat == nil {
		return 0
	}
	return Ticks(new(big.Int).Quo(e.rat.Num(), e.rat.Denom()).Int64())
}

// Cmp compares two exact times: -1 when e < other, 0 equal, +1 greater.
func (e ExactTime) Cmp(other ExactTime) int { return e.Rat().Cmp(other.Rat()) }

// Mapper performs reversible, exact time mapping across a Timeline.
type Mapper struct{ tl Timeline }

// NewMapper wraps a built timeline.
func NewMapper(tl Timeline) *Mapper { return &Mapper{tl: tl} }

// Timeline returns the mapped timeline.
func (m *Mapper) Timeline() Timeline { return m.tl }

// Forward maps a source (edit) time onto the output timeline exactly.
func (m *Mapper) Forward(src Ticks) (ExactTime, error) {
	s, ok := m.tl.segmentAt(src)
	if !ok {
		return ExactTime{}, ErrOutOfRange
	}
	off := new(big.Rat).SetFrac(big.NewInt(int64(src-s.SrcStart)), big.NewInt(1))
	off.Mul(off, big.NewRat(s.Num, s.Den))
	return ExactTime{rat: new(big.Rat).Add(big.NewRat(int64(s.OutStart), 1), off)}, nil
}

// InverseTime maps an output time back to the source timeline exactly.
func (m *Mapper) InverseTime(out Ticks) (ExactTime, error) {
	return m.InverseExact(NewExactTime(out))
}

// InverseExact maps an exact (possibly fractional) output time back to the
// source timeline, preserving full rational precision for round trips.
func (m *Mapper) InverseExact(out ExactTime) (ExactTime, error) {
	i := sort.Search(len(m.tl.Segments), func(i int) bool {
		return big.NewRat(int64(m.tl.Segments[i].OutEnd), 1).Cmp(out.Rat()) > 0
	})
	if i >= len(m.tl.Segments) {
		return ExactTime{}, ErrOutOfRange
	}
	s := m.tl.Segments[i]
	if out.Rat().Cmp(big.NewRat(int64(s.OutStart), 1)) < 0 {
		return ExactTime{}, ErrOutOfRange
	}
	off := new(big.Rat).Sub(out.Rat(), big.NewRat(int64(s.OutStart), 1))
	off.Mul(off, big.NewRat(s.Den, s.Num))
	return ExactTime{rat: new(big.Rat).Add(big.NewRat(int64(s.SrcStart), 1), off)}, nil
}

// Candidate is one possible interpretation of an ambiguous mapping.
type Candidate struct {
	// SrcPTS is the presentation timestamp of the candidate source frame.
	SrcPTS Ticks
	// Reason explains why the candidate was generated.
	Reason AmbiguityReason
}

// AmbiguityReason classifies why a mapping is not unique.
type AmbiguityReason string

const (
	ReasonUnique      AmbiguityReason = ""
	ReasonSparseGap   AmbiguityReason = "sparse_gap: output time falls between sampled source frames"
	ReasonRepeatedPTS AmbiguityReason = "repeated_pts: several output frames share this source frame"
	ReasonBoundary    AmbiguityReason = "boundary: output time sits on a speed-segment seam"
)

// Mapping is the result of mapping an output position back to source.
type Mapping struct {
	// OutPTS is the output presentation time that was queried.
	OutPTS Ticks
	// Exact is the exact (rational) source time before frame snapping.
	Exact ExactTime
	// Boundary is true when OutPTS sits exactly on a speed-segment seam, so
	// the source time is shared by the two adjacent segments.
	Boundary bool
	// Primary is the preferred candidate (first frame at or after the exact
	// source time); zero value only when Candidates is empty.
	Primary Candidate
	// Candidates lists every plausible source frame, ordered by source PTS.
	// A one-element slice means the mapping was unique.
	Candidates []Candidate
}

// Unique reports whether exactly one source frame explains the output point.
// A single candidate produced by a repeated-PTS presentation is still
// ambiguous: the user asked for one copy of a frame that is shown many times.
func (r Mapping) Unique() bool {
	return len(r.Candidates) == 1 && r.Candidates[0].Reason == ReasonUnique
}

// ErrOutOfRange is returned for times outside the timeline.
var ErrOutOfRange = errors.New("vfr: time outside timeline")

// onBoundary reports whether outPTS is an internal speed-segment seam.
func (m *Mapper) onBoundary(outPTS Ticks) bool {
	for i := 1; i < len(m.tl.Segments); i++ {
		if m.tl.Segments[i].OutStart == outPTS {
			return true
		}
	}
	return false
}
