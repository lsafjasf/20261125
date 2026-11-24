package vfr

import (
	"errors"
	"math/big"
	"sort"
)

// Ticks is the common time unit for both the source (edit/playout) timeline
// and the output (transcoded VFR) timeline. One second is TickRate ticks.
type Ticks int64

// TickRate defines the time resolution: 90000 ticks per second, the same
// clock MPEG-TS/MP4 presentation timestamps traditionally use.
const TickRate Ticks = 90000

// Segment describes one constant-speed region of the output timeline.
//
// Source footage on [SrcStart, SrcEnd) is played at Speed = Num/Den (a
// positive rational; >1 fast forward, <1 slow motion) and occupies
// [OutStart, OutEnd) on the output timeline. Segment boundaries are kept
// exactly consistent by BuildTimeline, so callers normally leave OutStart
// and OutEnd zero when constructing a timeline.
type Segment struct {
	SrcStart Ticks
	SrcEnd   Ticks
	OutStart Ticks
	OutEnd   Ticks
	Num      int64
	Den      int64
}

// Speed returns the segment speed as a reduced rational.
func (s Segment) Speed() (num, den int64) {
	n, d := big.NewInt(s.Num), big.NewInt(s.Den)
	g := new(big.Int).GCD(nil, nil, n, d)
	return s.Num / g.Int64(), s.Den / g.Int64()
}

// Duration returns the output duration of the segment.
func (s Segment) Duration() Ticks { return s.OutEnd - s.OutStart }

// SourceDuration returns the amount of source timeline the segment covers.
func (s Segment) SourceDuration() Ticks { return s.SrcEnd - s.SrcStart }

// Timeline is a validated, contiguous sequence of speed segments.
type Timeline struct {
	Segments []Segment
}

// ErrInvalidTimeline is returned when speed segments are empty, unordered,
// have a non-positive length or speed, or do not join contiguously.
var ErrInvalidTimeline = errors.New("vfr: invalid timeline")

// BuildTimeline validates the segments and fills in their output boundaries.
// The input may supply SrcStart/SrcEnd/Num/Den only; output offsets are
// derived as duration * speed and chained from zero.
func BuildTimeline(segs []Segment) (Timeline, error) {
	if len(segs) == 0 {
		return Timeline{}, ErrInvalidTimeline
	}
	out := make([]Segment, len(segs))
	var cursor Ticks
	for i, s := range segs {
		if s.SrcEnd <= s.SrcStart || s.Num <= 0 || s.Den <= 0 {
			return Timeline{}, ErrInvalidTimeline
		}
		if i > 0 && s.SrcStart != segs[i-1].SrcEnd {
			return Timeline{}, ErrInvalidTimeline
		}
		dur, ok := scaleTicksChecked(s.SourceDuration(), s.Num, s.Den)
		if !ok || cursor+dur < cursor {
			return Timeline{}, ErrInvalidTimeline
		}
		s.OutStart = cursor
		s.OutEnd = cursor + dur
		cursor = s.OutEnd
		out[i] = s
	}
	return Timeline{Segments: out}, nil
}

// TotalDuration returns the full output timeline length.
func (t Timeline) TotalDuration() Ticks {
	if len(t.Segments) == 0 {
		return 0
	}
	return t.Segments[len(t.Segments)-1].OutEnd
}

// SourceDuration returns the full source timeline length.
func (t Timeline) SourceDuration() Ticks {
	if len(t.Segments) == 0 {
		return 0
	}
	return t.Segments[len(t.Segments)-1].SrcEnd
}

// segmentAt returns the segment containing source time tm. Times exactly on a
// segment end boundary resolve to the following segment when possible.
func (t Timeline) segmentAt(tm Ticks) (Segment, bool) {
	i := sort.Search(len(t.Segments), func(i int) bool {
		return t.Segments[i].SrcEnd > tm
	})
	if i >= len(t.Segments) {
		return Segment{}, false
	}
	s := t.Segments[i]
	if tm < s.SrcStart {
		return Segment{}, false
	}
	return s, true
}

// scaleTicksChecked computes x*num/den rounded toward zero using arbitrary
// precision, so intermediate products never overflow int64, and reports
// whether the result fits the tick grid.
func scaleTicksChecked(x Ticks, num, den int64) (Ticks, bool) {
	v := new(big.Int).Mul(big.NewInt(int64(x)), big.NewInt(num))
	v.Quo(v, big.NewInt(den))
	if !v.IsInt64() {
		return 0, false
	}
	return Ticks(v.Int64()), true
}
