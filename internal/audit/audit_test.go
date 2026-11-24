package audit

import (
	"testing"

	"subaudit/internal/caps"
	"subaudit/internal/strip"
)

func manifest(strips ...strip.Strip) *strip.Manifest {
	return &strip.Manifest{Video: strip.Video{Width: 1920, Height: 1080}, Strips: strips}
}

func base() strip.Strip {
	return strip.Strip{
		ID: "a", Start: "00:00:01.000", End: "00:00:02.000",
		Text: "hello", Anchor: "bottom-center", Align: "center",
		Box: strip.Box{X: 660, Y: 980, W: 600, H: 60},
	}
}

func verdictFor(t *testing.T, format string, s strip.Strip, extra ...strip.Strip) Result {
	t.Helper()
	profile := caps.Profiles()[format]
	rep := Run(manifest(append([]strip.Strip{s}, extra...)...), profile)
	for _, r := range rep.Results {
		if r.Strip.ID == s.ID {
			return r
		}
	}
	t.Fatalf("strip %s not audited", s.ID)
	return Result{}
}

func TestCleanStripMigrates(t *testing.T) {
	for _, format := range []string{"srt", "webvtt", "ass", "ttml"} {
		if r := verdictFor(t, format, base()); r.Verdict != Migrated {
			t.Errorf("%s: got %s, want migrated (findings=%v)", format, r.Verdict, r.Findings)
		}
	}
}

func TestOutlineLossByFormat(t *testing.T) {
	s := base()
	s.Outline = &strip.Outline{WidthPx: 2, Color: "#000000"}
	cases := map[string]Verdict{
		"srt":    Loss,
		"webvtt": Degraded,
		"ass":    Migrated,
		"ttml":   Migrated,
	}
	for format, want := range cases {
		if r := verdictFor(t, format, s); r.Verdict != want {
			t.Errorf("%s: got %s, want %s", format, r.Verdict, want)
		}
	}
}

func TestFadeLoss(t *testing.T) {
	s := base()
	s.FadeInMs, s.FadeOutMs = 300, 300
	if r := verdictFor(t, "webvtt", s); r.Verdict != Loss {
		t.Errorf("webvtt fade: got %s, want loss", r.Verdict)
	}
	if r := verdictFor(t, "ass", s); r.Verdict != Migrated {
		t.Errorf("ass fade: got %s, want migrated", r.Verdict)
	}
}

func TestComplexPosition(t *testing.T) {
	s := base()
	s.Anchor, s.Align = "top-left", "left"
	s.Box = strip.Box{X: 80, Y: 60, W: 500, H: 70}
	if r := verdictFor(t, "webvtt", s); r.Verdict != Degraded {
		t.Errorf("webvtt position: got %s, want degraded", r.Verdict)
	}
	if r := verdictFor(t, "srt", s); r.Verdict != Loss {
		t.Errorf("srt position: got %s, want loss", r.Verdict)
	}
}

func TestRotationLoss(t *testing.T) {
	s := base()
	s.Rotation = 90
	if r := verdictFor(t, "webvtt", s); r.Verdict != Loss {
		t.Errorf("webvtt rotation: got %s, want loss", r.Verdict)
	}
	if r := verdictFor(t, "ass", s); r.Verdict != Degraded {
		t.Errorf("ass rotation: got %s, want degraded", r.Verdict)
	}
}

func TestNoTextNotMigrated(t *testing.T) {
	s := base()
	s.Text = ""
	r := verdictFor(t, "ass", s)
	if r.Verdict != NotMigrated {
		t.Fatalf("got %s, want not_migrated", r.Verdict)
	}
	if len(r.Findings) != 1 || r.Findings[0].Fix == "" {
		t.Errorf("expected one finding with fix suggestion, got %+v", r.Findings)
	}
}

func TestOverlapDetection(t *testing.T) {
	a := base()
	b := base()
	b.ID = "b"
	b.Start, b.End = "00:00:01.500", "00:00:03.000"
	c := base()
	c.ID = "c"
	c.Start, c.End = "00:00:05.000", "00:00:06.000"
	rep := Run(manifest(a, b, c), caps.Profiles()["ass"])
	got := map[string]Verdict{}
	for _, r := range rep.Results {
		got[r.Strip.ID] = r.Verdict
	}
	// ASS handles overlap exactly, so no finding; SRT must flag a and b but not c.
	rep = Run(manifest(a, b, c), caps.Profiles()["srt"])
	for _, r := range rep.Results {
		got[r.Strip.ID] = r.Verdict
	}
	if got["a"] != Degraded || got["b"] != Degraded {
		t.Errorf("overlapping strips should degrade in srt: %v", got)
	}
	if got["c"] != Migrated {
		t.Errorf("isolated strip should migrate cleanly: %v", got)
	}
}

func TestEveryFindingHasFix(t *testing.T) {
	s := base()
	s.Outline = &strip.Outline{WidthPx: 2, Color: "#000"}
	s.Shadow = &strip.Shadow{OffsetXPx: 2, OffsetYPx: 2, Color: "#000"}
	s.FadeInMs = 100
	s.Rotation = 45
	s.Anchor = "top-left"
	for format := range caps.Profiles() {
		r := verdictFor(t, format, s)
		for _, f := range r.Findings {
			if f.Reason == "" || f.Fix == "" {
				t.Errorf("%s: finding %+v missing reason/fix", format, f)
			}
		}
	}
}
