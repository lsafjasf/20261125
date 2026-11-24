package timecode

import "testing"

func TestNonDropRoundTrip(t *testing.T) {
	for _, f := range []int64{0, 1, 29, 30, 1799, 1800, 107892} {
		tc := Format(f, 30, false)
		got, err := Parse(tc, 30)
		if err != nil {
			t.Fatalf("parse %q: %v", tc, err)
		}
		if got != f {
			t.Errorf("frame %d -> %q -> %d", f, tc, got)
		}
	}
}

func TestDropFrameKnownValues(t *testing.T) {
	cases := []struct {
		frames int64
		tc     string
	}{
		{0, "00:00:00;00"},
		{1799, "00:00:59;29"},
		{1800, "00:01:00;02"}, // labels ;00 and ;01 dropped
		{17982, "00:10:00;00"},
		{107892, "01:00:00;00"}, // 1 hour of 29.97 DF
	}
	for _, c := range cases {
		if got := Format(c.frames, 30, true); got != c.tc {
			t.Errorf("Format(%d) = %s, want %s", c.frames, got, c.tc)
		}
		got, err := Parse(c.tc, 30)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.tc, err)
		}
		if got != c.frames {
			t.Errorf("Parse(%q) = %d, want %d", c.tc, got, c.frames)
		}
	}
}

func TestDropFrameRoundTrip(t *testing.T) {
	for f := int64(0); f < 200000; f += 137 {
		tc := Format(f, 30, true)
		got, err := Parse(tc, 30)
		if err != nil {
			t.Fatalf("parse %q (frame %d): %v", tc, f, err)
		}
		if got != f {
			t.Fatalf("frame %d -> %q -> %d", f, tc, got)
		}
	}
}

func TestDroppedLabelRejected(t *testing.T) {
	if _, err := Parse("00:01:00;00", 30); err == nil {
		t.Fatal("expected error for dropped label")
	}
	if _, err := Parse("00:10:00;00", 30); err != nil {
		t.Fatalf("minute 10 labels are valid: %v", err)
	}
}
