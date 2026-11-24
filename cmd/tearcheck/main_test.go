package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tearcheck/internal/analyze"
	"tearcheck/internal/frameio"
)

func TestAnalyzeVerifyAndReviewCLI(t *testing.T) {
	dir := t.TempDir()
	writeCLITestFrame(t, dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"analyze", "--input", dir, "--out", filepath.Join(dir, "report.json"),
		"--max-shift", "16", "--fail-on-tear",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitTear {
		t.Fatalf("analyze exit = %d, want %d; stderr=%s", code, exitTear, stderr.String())
	}
	report := readCLIReport(t, filepath.Join(dir, "report.json"))
	if report.Summary.FinalTearFrames != 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	if !strings.Contains(stdout.String(), "tears=1") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	firstReport, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"analyze", "--input", dir, "--out", filepath.Join(dir, "report-2.json"),
		"--max-shift", "16",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("second analyze exit=%d stderr=%s", code, stderr.String())
	}
	secondReport, err := os.ReadFile(filepath.Join(dir, "report-2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstReport, secondReport) {
		t.Fatal("two analyze runs wrote different report bytes")
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"verify", "--input", dir, "--max-shift", "16"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "identical=true") {
		t.Fatalf("verify exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"review", "set", "--report", filepath.Join(dir, "report.json"),
		"--review", filepath.Join(dir, "review.json"), "--frame", "f00",
		"--decision", "clear", "--note", "intentional UI wipe",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("review set exit=%d stderr=%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"analyze", "--input", dir, "--review", filepath.Join(dir, "review.json"),
		"--out", filepath.Join(dir, "cleared.json"), "--max-shift", "16",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("reviewed analyze exit=%d stderr=%s", code, stderr.String())
	}
	cleared := readCLIReport(t, filepath.Join(dir, "cleared.json"))
	if cleared.Summary.FinalTearFrames != 0 || cleared.Summary.ReviewedFrames != 1 {
		t.Fatalf("cleared summary = %+v", cleared.Summary)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"review", "list", "--report", filepath.Join(dir, "cleared.json"), "--state", "reviewed"},
		strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "f00") || !strings.Contains(stdout.String(), "intentional UI wipe") {
		t.Fatalf("review list exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"review", "walk", "--report", filepath.Join(dir, "report.json"),
		"--review", filepath.Join(dir, "walk-review.json"), "--reviewer", "alice",
	}, strings.NewReader("x\n"), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("review walk exit=%d stderr=%s", code, stderr.String())
	}
	walked, err := os.ReadFile(filepath.Join(dir, "walk-review.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(walked, []byte(`"frame_id": "f00"`)) || !bytes.Contains(walked, []byte(`"decision": "clear"`)) {
		t.Fatalf("walk review = %s", walked)
	}
}

func writeCLITestFrame(t *testing.T, dir string) {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 160, 96))
	const shift = 10
	for y := 0; y < 96; y++ {
		for x := 0; x < 160; x++ {
			sourceX := x
			if y >= 30 && y < 62 {
				sourceX = (x - shift + 160) % 160
			}
			img.SetGray(x, y, color.Gray{Y: uint8((sourceX*37 + (sourceX/5)*71 + (sourceX*sourceX)%251) % 128)})
		}
	}
	file, err := os.Create(filepath.Join(dir, "f00.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(frameio.Manifest{
		Schema: frameio.ManifestSchema,
		FPSNum: 25,
		FPSDen: 1,
		Frames: []frameio.ManifestFrame{{
			ID: "f00", File: "f00.png", PTSUS: 0, DurationUS: 40_000,
		}},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "frames.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readCLIReport(t *testing.T, path string) *analyze.Report {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report analyze.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	return &report
}
