package vfr

import "testing"

func newTestEditor(t *testing.T) *Editor {
	t.Helper()
	tl := demoTimeline(t)
	markers := []Marker{
		{ID: "cue-1", Kind: MarkerAudio, At: sec(2), Length: TickRate / 2},
		{ID: "sub-1", Kind: MarkerSubtitle, At: sec(5), Length: TickRate},
	}
	ed, err := NewEditor(tl, []string{"cut-a", "cut-b"}, markers)
	if err != nil {
		t.Fatal(err)
	}
	return ed
}

func TestRecalcAfterCutMove(t *testing.T) {
	ed := newTestEditor(t)
	before := ed.Recalc()
	if len(before) != 2 || before[0].OutAt != sec(4) {
		t.Fatalf("before = %+v", before)
	}

	tl := ed.Timeline()
	src := SourceFrames(TickRate/10, 100)
	frames, err := Sample(tl, src, SampleConfig{SourceFrameInterval: 1})
	if err != nil {
		t.Fatal(err)
	}

	// Move the first cut (currently at source 4s / output 8s) to output 6s,
	// which sits inside the first 2x segment -> source 3s.
	res, err := ed.MoveCut("cut-a", sec(6), frames)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied {
		t.Fatalf("move not applied: %+v", res)
	}
	if ed.Cuts()[0].At != sec(3) {
		t.Fatalf("cut at %d, want 3s", ed.Cuts()[0].At)
	}

	after := ed.Recalc()
	// cue-1 at source 2s is still in the 2x segment -> output 4s unchanged.
	if after[0].OutAt != sec(4) {
		t.Fatalf("cue-1 moved unexpectedly: %+v", after[0])
	}
	// sub-1 at source 5s now lies one second into the 0.5x segment whose
	// output start shifted from 8s to 6s: 6 + (5-3)*0.5 = 7s.
	if after[1].OutAt != sec(7) {
		t.Fatalf("sub-1 OutAt = %d, want 7s", after[1].OutAt)
	}
}

func TestMoveCutAmbiguousReturnsCandidates(t *testing.T) {
	tl, err := BuildTimeline([]Segment{
		{SrcStart: 0, SrcEnd: sec(4), Num: 1, Den: 2},
		{SrcStart: sec(4), SrcEnd: sec(8), Num: 1, Den: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ed, err := NewEditor(tl, []string{"cut"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	src := SourceFrames(TickRate/2, 16)
	frames, err := Sample(tl, src, SampleConfig{
		SourceFrameInterval: 1,
		MinOutputInterval:   TickRate / 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Find a repeated PTS belonging to frame 0 and try to move the cut there.
	var dupPTS Ticks = -1
	for _, f := range frames {
		if f.SourceIndex == 0 && f.Repeat > 0 {
			dupPTS = f.OutPTS
			break
		}
	}
	if dupPTS < 0 {
		t.Fatal("no repeated PTS produced")
	}
	res, err := ed.MoveCut("cut", dupPTS, frames)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied || len(res.Candidates) == 0 {
		t.Fatalf("expected ambiguity, got %+v", res)
	}
	if err := ed.MoveCutSource("cut", sec(1)); err != nil {
		t.Fatal(err)
	}
	if ed.Cuts()[0].At != sec(1) {
		t.Fatalf("cut not committed: %+v", ed.Cuts())
	}
}

func TestMoveCutRejectsOutOfOrder(t *testing.T) {
	ed := newTestEditor(t)
	// First cut cannot move past the second cut (source 7s).
	if err := ed.MoveCutSource("cut-a", sec(8)); err != ErrInvalidTimeline {
		t.Fatalf("got %v", err)
	}
	if err := ed.MoveCutSource("missing", sec(5)); err != ErrCutNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestAnchorOutput(t *testing.T) {
	ed := newTestEditor(t)
	src := SourceFrames(TickRate, 10)
	frames, err := Sample(ed.Timeline(), src, SampleConfig{SourceFrameInterval: 1})
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := ed.AnchorOutput(sec(8), frames)
	if err != nil {
		t.Fatal(err)
	}
	if !mapping.Unique() || mapping.Primary.SrcPTS != sec(4) {
		t.Fatalf("anchor = %+v", mapping)
	}
}

func TestRecalcDropsOrClampsMarkers(t *testing.T) {
	tl := demoTimeline(t)
	ed, err := NewEditor(tl, nil, []Marker{
		{ID: "inside", Kind: MarkerAudio, At: sec(1)},
		{ID: "outside", Kind: MarkerAudio, At: sec(99)},
		{ID: "runs-out", Kind: MarkerSubtitle, At: sec(9), Length: sec(5)},
	})
	if err != nil {
		t.Fatal(err)
	}
	placed := ed.Recalc()
	if len(placed) != 2 {
		t.Fatalf("placed = %+v", placed)
	}
	last := placed[1]
	if last.OutEnd != tl.TotalDuration() {
		t.Fatalf("overflow marker not clamped: %+v", last)
	}
}

func TestMoveCutOnOutOfRangePTS(t *testing.T) {
	ed := newTestEditor(t)
	src := SourceFrames(TickRate, 10)
	frames, err := Sample(ed.Timeline(), src, SampleConfig{SourceFrameInterval: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ed.MoveCut("cut-a", sec(99), frames); err != ErrOutOfRange {
		t.Fatalf("got %v", err)
	}
}
