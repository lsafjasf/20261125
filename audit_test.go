package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEndDemoFixtureAndCorrectedFrame(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").CombinedOutput(); err != nil ||
		!strings.Contains(string(out), "libx264") {
		t.Skip("ffmpeg libx264 encoder not available")
	}

	dir := t.TempDir()
	video := filepath.Join(dir, "double-rotation.mp4")
	reference, err := MakeDemoFixture(video, "ffmpeg")
	if err != nil {
		t.Fatal(err)
	}

	tracks, err := ReadMP4TrackDisplays(video)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) == 0 {
		t.Fatal("expected MP4 tkhd track")
	}
	matrix := matrixFromRaw(tracks[0].Matrix, "test")
	if matrix.TurnsClockwise != 1 {
		t.Fatalf("demo matrix turns=%d, want 1", matrix.TurnsClockwise)
	}
	if tracks[0].DisplayWidth != 320 || tracks[0].DisplayHeight != 180 {
		t.Fatalf("display dimensions=%.1fx%.1f, want 320x180", tracks[0].DisplayWidth, tracks[0].DisplayHeight)
	}

	out := filepath.Join(dir, "frames")
	err = run([]string{
		"-input", video,
		"-reference", reference,
		"-out", out,
		"-now", "2026-11-25T01:10:00Z",
	})
	if err != nil {
		t.Fatalf("run audit: %v", err)
	}

	reportBytes, err := os.ReadFile(filepath.Join(out, "rotation-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatal(err)
	}
	if report.Observation.PixelWidth != 180 || report.Observation.PixelHeight != 320 {
		t.Fatalf("raw pixels=%dx%d, want 180x320", report.Observation.PixelWidth, report.Observation.PixelHeight)
	}
	if report.Observation.TrackDisplayWidth != 320 || report.Observation.DisplayClaimOrient != OrientationLandscape {
		t.Fatalf("bad display claim: %+v", report.Observation)
	}
	if report.Observation.Pixel.CorrectionTurnsCW == nil || *report.Observation.Pixel.CorrectionTurnsCW != 3 {
		t.Fatalf("reference correction=%+v, want 3 CW turns", report.Observation.Pixel)
	}
	if len(report.HighRisk) < 2 {
		t.Fatalf("high risk combos=%d, want >=2", len(report.HighRisk))
	}
	for _, name := range []string{"01-raw-pixels-noautorotate.png", "02-matrix-respecting-player.png", "04-corrected.png", "comparison.png"} {
		info, err := os.Stat(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("expected output %s: %v", name, err)
		}
		if info.Size() == 0 {
			t.Fatalf("output %s is empty", name)
		}
	}
}
