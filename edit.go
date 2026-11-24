package vfr

import (
	"errors"
	"sort"
	"strconv"
)

// Cut is a user-movable boundary between two speed segments, addressed by
// the source timeline position it currently occupies.
type Cut struct {
	// ID is the caller-chosen stable identifier of the cut.
	ID string
	// At is the current source-time position of the cut.
	At Ticks
}

// Marker is an audio cue or subtitle event anchored to source time. Its
// output position is derived, never stored, so retiming moves it correctly.
type Marker struct {
	ID     string
	Kind   MarkerKind
	At     Ticks
	Length Ticks
	Text   string
}

// MarkerKind distinguishes audio cues from subtitle events.
type MarkerKind string

const (
	MarkerAudio    MarkerKind = "audio"
	MarkerSubtitle MarkerKind = "subtitle"
)

// PlacedMarker is a marker projected onto the output timeline.
type PlacedMarker struct {
	Marker Marker
	OutAt  Ticks
	OutEnd Ticks
}

// Editor owns a timeline plus its cuts and anchored markers.
type Editor struct {
	tl      Timeline
	cuts    []Cut
	markers []Marker
}

// NewEditor builds an editor. Cuts are derived from internal segment
// boundaries when cutIDs is empty; otherwise cutIDs must have one entry per
// internal boundary in order.
func NewEditor(tl Timeline, cutIDs []string, markers []Marker) (*Editor, error) {
	internal := len(tl.Segments) - 1
	if len(cutIDs) != 0 && len(cutIDs) != internal {
		return nil, ErrInvalidTimeline
	}
	cuts := make([]Cut, internal)
	for i := range cuts {
		id := cutID(cutIDs, i)
		cuts[i] = Cut{ID: id, At: tl.Segments[i+1].SrcStart}
	}
	return &Editor{tl: tl, cuts: cuts, markers: append([]Marker(nil), markers...)}, nil
}

func cutID(ids []string, i int) string {
	if len(ids) > i {
		return ids[i]
	}
	return defaultCutID(i)
}

func defaultCutID(i int) string {
	const digits = "0123456789"
	if i < len(digits) {
		return "cut-" + string(digits[i])
	}
	return "cut-" + strconv.Itoa(i)
}

// Timeline returns the current (possibly edited) timeline.
func (e *Editor) Timeline() Timeline { return e.tl }

// Cuts returns a copy of the cuts sorted by position.
func (e *Editor) Cuts() []Cut {
	out := append([]Cut(nil), e.cuts...)
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// MoveResult reports a cut move, including candidate target frames when the
// requested output position cannot be mapped to a unique source frame.
type MoveResult struct {
	Cut        Cut
	Applied    bool
	Candidates []Candidate
	Reason     AmbiguityReason
}

// ErrCutNotFound is returned when no cut matches the given ID.
var ErrCutNotFound = errors.New("vfr: cut not found")

// MoveCut moves a cut to a new output PTS. When the output PTS maps to
// several source frames (repeated PTS or a sparse gap), nothing is applied
// and the candidates are returned for the user to choose; call
// MoveCutSource with the chosen SrcPTS to commit.
func (e *Editor) MoveCut(id string, outPTS Ticks, frames []OutputFrame) (MoveResult, error) {
	idx := e.cutIndex(id)
	if idx < 0 {
		return MoveResult{}, ErrCutNotFound
	}
	mapping, err := NewMapper(e.tl).MapOutput(outPTS, frames)
	if err != nil {
		return MoveResult{}, err
	}
	if !mapping.Unique() {
		return MoveResult{
			Cut:        e.cuts[idx],
			Applied:    false,
			Candidates: mapping.Candidates,
			Reason:     mapping.Primary.Reason,
		}, nil
	}
	if err := e.moveToSource(idx, mapping.Primary.SrcPTS); err != nil {
		return MoveResult{}, err
	}
	return MoveResult{Cut: e.cuts[idx], Applied: true, Candidates: mapping.Candidates}, nil
}

// MoveCutSource commits a cut move to an explicit source position, typically
// one chosen from MoveResult.Candidates.
func (e *Editor) MoveCutSource(id string, srcPTS Ticks) error {
	idx := e.cutIndex(id)
	if idx < 0 {
		return ErrCutNotFound
	}
	return e.moveToSource(idx, srcPTS)
}

func (e *Editor) moveToSource(boundary int, at Ticks) error {
	left := e.tl.Segments[boundary]
	right := e.tl.Segments[boundary+1]
	if at <= left.SrcStart || at >= right.SrcEnd {
		return ErrInvalidTimeline
	}
	segs := append([]Segment(nil), e.tl.Segments...)
	segs[boundary].SrcEnd = at
	segs[boundary+1].SrcStart = at
	tl, err := BuildTimeline(segs)
	if err != nil {
		return err
	}
	e.tl = tl
	e.cuts[boundary].At = at
	return nil
}

func (e *Editor) cutIndex(id string) int {
	for i := range e.cuts {
		if e.cuts[i].ID == id {
			return i
		}
	}
	return -1
}

// Recalc projects every anchored audio/subtitle marker onto the current
// output timeline. Markers whose anchor is no longer covered are dropped.
func (e *Editor) Recalc() []PlacedMarker {
	m := NewMapper(e.tl)
	out := make([]PlacedMarker, 0, len(e.markers))
	for _, mk := range e.markers {
		start, err := m.Forward(mk.At)
		if err != nil {
			continue
		}
		placed := PlacedMarker{Marker: mk, OutAt: start.Ticks()}
		if mk.Length > 0 {
			if end, err := m.Forward(mk.At + mk.Length); err == nil {
				placed.OutEnd = end.Ticks()
			} else {
				placed.OutEnd = e.tl.TotalDuration()
			}
		} else {
			placed.OutEnd = placed.OutAt
		}
		out = append(out, placed)
	}
	return out
}

// AnchorOutput maps an existing output PTS back to source time so a new
// audio/subtitle marker can be anchored correctly. Ambiguous PTS values
// return candidates just like MoveCut.
func (e *Editor) AnchorOutput(outPTS Ticks, frames []OutputFrame) (Mapping, error) {
	return NewMapper(e.tl).MapOutput(outPTS, frames)
}
