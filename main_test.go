package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"mp4index/fixture"
	"mp4index/repair"
)

func TestCLIRepairAndJSON(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "case.mp4")
	if _, err := fixture.Build(fixture.Config{Path: src, Frames: 16, GOP: 4, WithAUD: true}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "nested", "repaired.mp4")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-in", src, "-out", out, "-json", "-fps", "15"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	var rep repair.Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("json report: %v\n%s", err, stdout.String())
	}
	if rep.IndexedSamples != 16 || len(rep.Keyframes) != 4 {
		t.Fatalf("report counts wrong: samples=%d keys=%v", rep.IndexedSamples, rep.Keyframes)
	}
	if len(rep.Intervals) != 1 {
		t.Fatalf("intervals=%d", len(rep.Intervals))
	}

	// Human report path.
	stdout.Reset()
	code = run([]string{"-in", src, "-inspect"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("inspect exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "可播放区间") {
		t.Fatalf("human report missing intervals:\n%s", stdout.String())
	}

	// Missing input yields usage error exit code 2.
	code = run([]string{}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("no args exit=%d want 2", code)
	}

	// Repairing onto the source path fails with exit 1.
	code = run([]string{"-in", src, "-out", src}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "separate file") {
		t.Fatalf("same path should fail safely, code=%d err=%s", code, stderr.String())
	}
}
