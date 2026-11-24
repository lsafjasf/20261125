package vfr

import "testing"

func TestSparseSampling(t *testing.T) {
	tl := demoTimeline(t)
	src := SourceFrames(TickRate/2, 20) // 2 fps across 10 s of source
	out, err := Sample(tl, src, SampleConfig{SourceFrameInterval: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 10 {
		t.Fatalf("kept frames: got %d want 10", len(out))
	}
	for _, f := range out {
		if f.SourceIndex%2 != 0 {
			t.Fatalf("unexpected source frame %d", f.SourceIndex)
		}
		if f.Repeat != 0 {
			t.Fatalf("unexpected repeat: %+v", f)
		}
	}
}

func TestRepeatedPTSInverse(t *testing.T) {
	tl, err := BuildTimeline([]Segment{
		{SrcStart: 0, SrcEnd: sec(2), Num: 1, Den: 2}, // half speed: 4s output
	})
	if err != nil {
		t.Fatal(err)
	}
	src := SourceFrames(TickRate/2, 4) // frames at 0, .5, 1, 1.5s source
	out, err := Sample(tl, src, SampleConfig{
		SourceFrameInterval: 1,
		MinOutputInterval:   TickRate / 9, // denser than frame spacing -> repeats
	})
	if err != nil {
		t.Fatal(err)
	}
	// The first source frame must be presented multiple times.
	var reps []OutputFrame
	for _, f := range out {
		if f.SourceIndex == 0 {
			reps = append(reps, f)
		}
	}
	if len(reps) < 2 {
		t.Fatalf("expected repeated frames, got %+v", out)
	}
	m := NewMapper(tl)
	mapping, err := m.MapOutput(reps[1].OutPTS, out)
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Unique() {
		t.Fatalf("repeated PTS mapped uniquely: %+v", mapping)
	}
	if mapping.Primary.Reason != ReasonRepeatedPTS {
		t.Fatalf("reason = %q", mapping.Primary.Reason)
	}
	if len(mapping.Candidates) != 1 || mapping.Candidates[0].SrcPTS != 0 {
		t.Fatalf("candidates = %+v", mapping.Candidates)
	}
}

func TestSparseGapCandidates(t *testing.T) {
	tl, err := BuildTimeline([]Segment{{SrcStart: 0, SrcEnd: sec(4), Num: 1, Den: 1}})
	if err != nil {
		t.Fatal(err)
	}
	src := SourceFrames(TickRate, 4) // one frame per second
	out, err := Sample(tl, src, SampleConfig{SourceFrameInterval: 2})
	if err != nil {
		t.Fatal(err)
	}
	m := NewMapper(tl)
	// Output PTS 1.5s sits between sampled frames at 0s and 2s.
	mapping, err := m.MapOutput(TickRate+TickRate/2, out)
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Unique() || mapping.Primary.Reason != ReasonSparseGap {
		t.Fatalf("expected sparse gap, got %+v", mapping)
	}
	if len(mapping.Candidates) != 2 {
		t.Fatalf("candidates = %+v", mapping.Candidates)
	}
}

func TestUniqueMapping(t *testing.T) {
	tl, err := BuildTimeline([]Segment{{SrcStart: 0, SrcEnd: sec(2), Num: 1, Den: 1}})
	if err != nil {
		t.Fatal(err)
	}
	src := SourceFrames(TickRate/2, 4)
	out, err := Sample(tl, src, SampleConfig{SourceFrameInterval: 1})
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := NewMapper(tl).MapOutput(TickRate, out)
	if err != nil {
		t.Fatal(err)
	}
	if !mapping.Unique() || mapping.Candidates[0].SrcPTS != TickRate {
		t.Fatalf("mapping = %+v", mapping)
	}
}

func TestSegmentBoundaryFlag(t *testing.T) {
	tl, err := BuildTimeline([]Segment{
		{SrcStart: 0, SrcEnd: sec(2), Num: 2, Den: 1},
		{SrcStart: sec(2), SrcEnd: sec(4), Num: 1, Den: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	src := SourceFrames(TickRate/2, 8)
	out, err := Sample(tl, src, SampleConfig{SourceFrameInterval: 1})
	if err != nil {
		t.Fatal(err)
	}
	// First segment output ends at 4s, which is the second segment's start.
	mapping, err := NewMapper(tl).MapOutput(sec(4), out)
	if err != nil {
		t.Fatal(err)
	}
	if !mapping.Boundary {
		t.Fatalf("expected boundary flag: %+v", mapping)
	}
	mid, err := NewMapper(tl).MapOutput(sec(1), out)
	if err != nil || mid.Boundary {
		t.Fatalf("unexpected boundary: %+v err=%v", mid, err)
	}
}

func TestMapOutputErrors(t *testing.T) {
	tl, err := BuildTimeline([]Segment{{SrcStart: 0, SrcEnd: sec(2), Num: 1, Den: 1}})
	if err != nil {
		t.Fatal(err)
	}
	src := SourceFrames(TickRate, 2)
	out, err := Sample(tl, src, SampleConfig{SourceFrameInterval: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewMapper(tl).MapOutput(sec(9), out); err != ErrOutOfRange {
		t.Fatalf("got %v", err)
	}
	if _, err := Sample(tl, src, SampleConfig{SourceFrameInterval: 0}); err != ErrInvalidSample {
		t.Fatalf("got %v", err)
	}
}
