package vfr

import "testing"

func TestDefaultCutIDs(t *testing.T) {
	var segs []Segment
	for i := 0; i < 12; i++ {
		segs = append(segs, Segment{
			SrcStart: Ticks(i), SrcEnd: Ticks(i + 1), Num: 1, Den: 1,
		})
	}
	tl, err := BuildTimeline(segs)
	if err != nil {
		t.Fatal(err)
	}
	ed, err := NewEditor(tl, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cuts := ed.Cuts()
	want := map[int]string{0: "cut-0", 9: "cut-9", 10: "cut-10"}
	for idx, id := range want {
		if cuts[idx].ID != id {
			t.Fatalf("cut %d id = %q, want %q", idx, cuts[idx].ID, id)
		}
	}
}

func TestSegmentHelpers(t *testing.T) {
	s := Segment{SrcStart: 0, SrcEnd: sec(2), Num: 4, Den: 6}
	num, den := s.Speed()
	if num != 2 || den != 3 {
		t.Fatalf("Speed = %d/%d", num, den)
	}
	if s.SourceDuration() != sec(2) {
		t.Fatalf("SourceDuration = %d", s.SourceDuration())
	}
}

func TestExactTimeCmpAndNil(t *testing.T) {
	var zero ExactTime
	if zero.Ticks() != 0 || zero.Rat().Sign() != 0 {
		t.Fatal("nil ExactTime should behave as zero")
	}
	a, b := NewExactTime(3), NewExactTime(5)
	if a.Cmp(a) != 0 || a.Cmp(b) >= 0 || b.Cmp(a) <= 0 {
		t.Fatal("Cmp ordering wrong")
	}
}

func TestNewEditorRejectsMismatchedCutIDs(t *testing.T) {
	tl := demoTimeline(t)
	if _, err := NewEditor(tl, []string{"only-one"}, nil); err != ErrInvalidTimeline {
		t.Fatalf("got %v", err)
	}
}
