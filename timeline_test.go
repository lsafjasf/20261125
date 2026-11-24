package vfr

import "testing"

func sec(n int64) Ticks { return Ticks(n) * TickRate }

func demoTimeline(t *testing.T) Timeline {
	t.Helper()
	tl, err := BuildTimeline([]Segment{
		{SrcStart: 0, SrcEnd: sec(4), Num: 2, Den: 1},      // 0-4s source -> 0-8s out
		{SrcStart: sec(4), SrcEnd: sec(7), Num: 1, Den: 2}, // 4-7s -> 8-9.5s
		{SrcStart: sec(7), SrcEnd: sec(10), Num: 1, Den: 1},
	})
	if err != nil {
		t.Fatalf("BuildTimeline: %v", err)
	}
	return tl
}

func TestBuildTimelineBoundaries(t *testing.T) {
	tl := demoTimeline(t)
	want := []Ticks{sec(0), sec(8), sec(8) + sec(1)/2*3, sec(12) + sec(1)/2}
	got := []Ticks{
		tl.Segments[0].OutStart,
		tl.Segments[0].OutEnd,
		tl.Segments[1].OutEnd,
		tl.Segments[2].OutEnd,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("boundary %d: got %d want %d", i, got[i], want[i])
		}
	}
}

func TestBuildTimelineRejectsGaps(t *testing.T) {
	if _, err := BuildTimeline([]Segment{
		{SrcStart: 0, SrcEnd: sec(1), Num: 1, Den: 1},
		{SrcStart: sec(2), SrcEnd: sec(3), Num: 1, Den: 1},
	}); err != ErrInvalidTimeline {
		t.Fatalf("gap not rejected: %v", err)
	}
	if _, err := BuildTimeline(nil); err != ErrInvalidTimeline {
		t.Fatalf("empty not rejected: %v", err)
	}
}

func TestBuildTimelineRejectsOverflowAndBadSpeed(t *testing.T) {
	if _, err := BuildTimeline([]Segment{
		{SrcStart: 0, SrcEnd: sec(1), Num: 0, Den: 1},
	}); err != ErrInvalidTimeline {
		t.Fatalf("zero speed not rejected: %v", err)
	}
	if _, err := BuildTimeline([]Segment{
		{SrcStart: 0, SrcEnd: sec(1), Num: 1, Den: 0},
	}); err != ErrInvalidTimeline {
		t.Fatalf("zero denominator not rejected: %v", err)
	}
	if _, err := BuildTimeline([]Segment{
		{SrcStart: 0, SrcEnd: Ticks(1 << 62), Num: 1 << 40, Den: 1},
	}); err != ErrInvalidTimeline {
		t.Fatalf("overflow not rejected: %v", err)
	}
}

func TestOutOfRange(t *testing.T) {
	tl := demoTimeline(t)
	m := NewMapper(tl)
	if _, err := m.Forward(tl.SourceDuration() + 1); err != ErrOutOfRange {
		t.Fatalf("forward out of range: %v", err)
	}
	if _, err := m.InverseTime(tl.TotalDuration() + 1); err != ErrOutOfRange {
		t.Fatalf("inverse out of range: %v", err)
	}
}

func TestForwardInverseRoundTrip(t *testing.T) {
	tl := demoTimeline(t)
	m := NewMapper(tl)
	cases := []Ticks{sec(1), sec(4), sec(5), sec(7), sec(9)}
	for _, src := range cases {
		exact, err := m.Forward(src)
		if err != nil {
			t.Fatalf("Forward(%d): %v", src, err)
		}
		back, err := m.InverseTime(exact.Ticks())
		if err != nil {
			t.Fatalf("InverseTime(%d): %v", exact.Ticks(), err)
		}
		// Floored integer round trip can lose at most one tick.
		delta := back.Ticks() - src
		if delta < -1 || delta > 1 {
			t.Fatalf("round trip %d -> %d -> %d", src, exact.Ticks(), back.Ticks())
		}
	}
}

func TestExactRoundTrip(t *testing.T) {
	tl := demoTimeline(t)
	m := NewMapper(tl)
	for _, src := range []Ticks{sec(1) + 1, sec(3) - 7, sec(6) + 33} {
		out, err := m.Forward(src)
		if err != nil {
			t.Fatal(err)
		}
		back, err := m.InverseExact(out)
		if err != nil {
			t.Fatal(err)
		}
		if back.Rat().Cmp(NewExactTime(src).Rat()) != 0 {
			t.Fatalf("exact round trip %v -> %v -> %v", src, out.Rat(), back)
		}
	}
}
