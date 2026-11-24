package audit_test

import (
	"bytes"
	"testing"

	"editlist-audit/internal/audit"
	"editlist-audit/internal/fixture"
)

func auditConfig(t *testing.T, cfg fixture.Config) audit.FileReport {
	t.Helper()
	data := fixture.BuildMOV(cfg)
	report := audit.AuditReaderAt(bytes.NewReader(data), int64(len(data)), cfg.FileName)
	if report.Error != "" {
		t.Fatalf("AuditReaderAt returned error: %s", report.Error)
	}
	if len(report.Tracks) != 1 {
		t.Fatalf("len(Tracks)=%d, want 1", len(report.Tracks))
	}
	if len(report.Tracks[0].Edits) != len(cfg.Edits) {
		t.Fatalf("len(Edits)=%d, want %d", len(report.Tracks[0].Edits), len(cfg.Edits))
	}
	return report
}

func TestEmptyEditDelaysAudio(t *testing.T) {
	report := auditConfig(t, fixture.Config{
		FileName: "empty.mov", MovieTimescale: 1000, MovieDuration: 1250,
		TrackID: 1, TrackDuration: 1250, MediaTimescale: 48000, MediaDuration: 48000,
		PCMFrames: 48000,
		Edits: []fixture.Edit{
			{Duration: 250, MediaTime: -1, Rate: 0x10000},
			{Duration: 1000, MediaTime: 0, Rate: 0x10000},
		},
	})
	track := report.Tracks[0]
	if !track.Edits[0].Empty || track.Edits[0].Media != nil {
		t.Fatalf("first edit = %+v, want empty without media interval", track.Edits[0])
	}
	if got := track.Edits[0].Composition.EndSeconds; got != "1/4" {
		t.Fatalf("empty composition end = %q, want 1/4", got)
	}
	if track.AudioStart == nil {
		t.Fatal("AudioStart is nil")
	}
	if got := track.AudioStart.CompositionStartSeconds; got != "1/4" {
		t.Fatalf("composition start = %q, want 1/4", got)
	}
	if got := track.AudioStart.UnitRateOffsetMedia.UnitsExact; got != "-12000" {
		t.Fatalf("unit-rate offset = %q, want -12000", got)
	}
}

func TestNegativeCompositionOrigin(t *testing.T) {
	report := auditConfig(t, fixture.Config{
		FileName: "negative-origin.mov", MovieTimescale: 1000, MovieDuration: 800,
		TrackID: 1, TrackDuration: 800, MediaTimescale: 44100, MediaDuration: 44100,
		PCMFrames: 44100,
		Edits:     []fixture.Edit{{Duration: 800, MediaTime: 8820, Rate: 0x10000}},
	})
	start := report.Tracks[0].AudioStart
	if start == nil || start.MediaZeroCompUnits == nil {
		t.Fatalf("start/origin missing: %+v", start)
	}
	if got := start.MediaZeroCompUnits.UnitsExact; got != "-200" {
		t.Fatalf("origin = %q, want -200", got)
	}
	if !start.NegativeTimelineOrigin {
		t.Fatal("NegativeTimelineOrigin=false, want true")
	}
	if got := start.UnitRateOffsetMedia.UnitsExact; got != "8820" {
		t.Fatalf("offset = %q, want 8820", got)
	}
}

func TestLiteralNegativeMediaTime(t *testing.T) {
	report := auditConfig(t, fixture.Config{
		FileName: "negative-media-time.mov", MovieTimescale: 1000, MovieDuration: 800,
		TrackID: 1, TrackDuration: 800, MediaTimescale: 48000, MediaDuration: 48000,
		PCMFrames: 48000,
		Edits:     []fixture.Edit{{Duration: 800, MediaTime: -2, Rate: 0x10000}},
	})
	edit := report.Tracks[0].Edits[0]
	if !edit.NegativeMediaTime {
		t.Fatal("NegativeMediaTime=false")
	}
	if edit.Media == nil || edit.Media.StartUnits != "-2" {
		t.Fatalf("media interval = %+v, want start -2", edit.Media)
	}
}

func TestZeroRateDwell(t *testing.T) {
	report := auditConfig(t, fixture.Config{
		FileName: "dwell.mov", MovieTimescale: 1000, MovieDuration: 1000,
		TrackID: 1, TrackDuration: 1000, MediaTimescale: 48000, MediaDuration: 48000,
		PCMFrames: 48000,
		Edits: []fixture.Edit{
			{Duration: 500, MediaTime: 12000, Rate: 0},
			{Duration: 500, MediaTime: 24000, Rate: 0x10000},
		},
	})
	edit := report.Tracks[0].Edits[0]
	if !edit.Dwell || edit.Media == nil {
		t.Fatalf("edit = %+v, want dwell with point interval", edit)
	}
	if edit.Media.DurationUnits != "0" || edit.Media.StartUnits != edit.Media.EndUnits {
		t.Fatalf("dwell interval = %+v, want zero-length point", edit.Media)
	}
}

func TestCrossTimescaleExactRounding(t *testing.T) {
	report := auditConfig(t, fixture.Config{
		FileName: "cross.mov", MovieTimescale: 600, MovieDuration: 250,
		TrackID: 1, TrackDuration: 250, MediaTimescale: 22050, MediaDuration: 22050,
		PCMFrames: 22050,
		Edits:     []fixture.Edit{{Duration: 250, MediaTime: 0, Rate: 0x10000}},
	})
	edit := report.Tracks[0].Edits[0]
	if edit.Media == nil || edit.Media.DurationUnits != "18375/2" {
		t.Fatalf("duration = %+v, want 18375/2", edit.Media)
	}
	if edit.MediaDurationRound == nil || edit.MediaDurationRound.Nearest != "9188" {
		t.Fatalf("rounding = %+v, want nearest 9188", edit.MediaDurationRound)
	}
}

func TestMediaRateScalesInterval(t *testing.T) {
	report := auditConfig(t, fixture.Config{
		FileName: "rate2x.mov", MovieTimescale: 1000, MovieDuration: 500,
		TrackID: 1, TrackDuration: 500, MediaTimescale: 48000, MediaDuration: 48000,
		PCMFrames: 48000,
		Edits:     []fixture.Edit{{Duration: 500, MediaTime: 0, Rate: 0x20000}},
	})
	edit := report.Tracks[0].Edits[0]
	if edit.RateRational != "2" || edit.Media == nil || edit.Media.EndUnits != "48000" {
		t.Fatalf("rate edit = %+v / %+v", edit.RateRational, edit.Media)
	}
}
