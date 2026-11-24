package vfr

import (
	"errors"
	"sort"
)

// Frame is one source frame, identified by its zero-based index and its
// presentation timestamp on the source timeline.
type Frame struct {
	Index int
	PTS   Ticks
}

// OutputFrame is a frame as it appears in the VFR output. Repeated frames
// share SourceIndex/SrcPTS but get distinct output PTS values.
type OutputFrame struct {
	SourceIndex int
	SrcPTS      Ticks
	OutPTS      Ticks
	// Repeat is 0 for the first presentation, 1..n for duplicate PTS copies.
	Repeat int
}

// SampleConfig controls sparse sampling and frame repetition.
type SampleConfig struct {
	// SourceFrameInterval keeps every n-th source frame (1 = keep all).
	SourceFrameInterval int
	// MinOutputInterval, when > 0, repeats a sampled frame until at least
	// this many output ticks have elapsed (slow-motion / VFR cadence).
	MinOutputInterval Ticks
	// MaxRepeat caps repetitions per sampled frame; 0 means unlimited.
	MaxRepeat int
}

// ErrInvalidSample is returned for an unusable SampleConfig.
var ErrInvalidSample = errors.New("vfr: invalid sample config")

// SourceFrames materializes a constant-cadence source stream starting at
// PTS zero with the given frame duration in ticks.
func SourceFrames(frameDuration Ticks, count int) []Frame {
	frames := make([]Frame, count)
	for i := range frames {
		frames[i] = Frame{Index: i, PTS: Ticks(i) * frameDuration}
	}
	return frames
}

// Sample renders the sampled output frame list for a timeline.
func Sample(tl Timeline, src []Frame, cfg SampleConfig) ([]OutputFrame, error) {
	if cfg.SourceFrameInterval <= 0 {
		return nil, ErrInvalidSample
	}
	m := NewMapper(tl)
	type sampled struct {
		frame OutputFrame
		next  Ticks // output PTS of the next sampled frame, or total duration
		ok    bool
	}
	var kept []sampled
	for i := 0; i < len(src); i += cfg.SourceFrameInterval {
		f := src[i]
		if _, ok := tl.segmentAt(f.PTS); !ok {
			continue
		}
		exact, err := m.Forward(f.PTS)
		if err != nil {
			continue
		}
		kept = append(kept, sampled{
			frame: OutputFrame{SourceIndex: f.Index, SrcPTS: f.PTS, OutPTS: exact.Ticks()},
		})
	}
	for i := range kept {
		if i+1 < len(kept) {
			kept[i].next, kept[i].ok = kept[i+1].frame.OutPTS, true
		} else {
			kept[i].next, kept[i].ok = tl.TotalDuration(), true
		}
	}
	var out []OutputFrame
	for _, sm := range kept {
		first := sm.frame
		out = append(out, first)
		if cfg.MinOutputInterval <= 0 {
			continue
		}
		for rep := 1; ; rep++ {
			if cfg.MaxRepeat > 0 && rep > cfg.MaxRepeat {
				break
			}
			dup := first
			dup.OutPTS = first.OutPTS + Ticks(rep)*cfg.MinOutputInterval
			dup.Repeat = rep
			// Stop once the duplicate would reach or pass the next sampled
			// frame's presentation time (or the end of the timeline).
			if dup.OutPTS >= sm.next {
				break
			}
			out = append(out, dup)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].OutPTS < out[b].OutPTS })
	return out, nil
}

// MapOutput maps an output PTS back to candidate source frames.
//
// Returned candidates cover, in order of preference:
//   - every output frame presented exactly at outPTS (unique PTS, repeated
//     PTS, or a sparse-gap snap),
//   - the bracketing frames when outPTS lies in a gap between sampled PTS.
func (m *Mapper) MapOutput(outPTS Ticks, frames []OutputFrame) (Mapping, error) {
	exact, err := m.InverseTime(outPTS)
	if err != nil {
		return Mapping{}, err
	}
	res := Mapping{OutPTS: outPTS, Exact: exact, Boundary: m.onBoundary(outPTS)}

	var exactHits []OutputFrame
	for _, f := range frames {
		if f.OutPTS == outPTS {
			exactHits = append(exactHits, f)
		}
	}
	if len(exactHits) > 0 {
		seen := map[Ticks]bool{}
		for _, f := range exactHits {
			if seen[f.SrcPTS] {
				continue
			}
			seen[f.SrcPTS] = true
			reason := ReasonUnique
			if f.Repeat > 0 || len(exactHits) > 1 {
				reason = ReasonRepeatedPTS
			}
			res.Candidates = append(res.Candidates, Candidate{SrcPTS: f.SrcPTS, Reason: reason})
		}
		res.Primary = res.Candidates[0]
		return res, nil
	}

	// No presented frame on this PTS: it falls inside a sparse-sampling gap
	// (or between repeat copies). Report the bracketing output frames.
	i := sort.Search(len(frames), func(i int) bool { return frames[i].OutPTS >= outPTS })
	add := func(idx int, reason AmbiguityReason) {
		if idx < 0 || idx >= len(frames) {
			return
		}
		c := Candidate{SrcPTS: frames[idx].SrcPTS, Reason: reason}
		res.Candidates = append(res.Candidates, c)
	}
	add(i-1, ReasonSparseGap)
	add(i, ReasonSparseGap)
	if len(res.Candidates) == 0 {
		return Mapping{}, ErrOutOfRange
	}
	res.Primary = res.Candidates[len(res.Candidates)-1]
	return res, nil
}
