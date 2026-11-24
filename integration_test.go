package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSingleBinaryDemo builds exactly one binary and requires its end-to-end
// comparison (crash -> restart -> duplicate execution) to pass.
func TestSingleBinaryDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary integration test in -short mode")
	}

	bin := filepath.Join(t.TempDir(), "batchproc")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	work := filepath.Join(t.TempDir(), "demo")
	report := filepath.Join(t.TempDir(), "report.md")
	cmd := exec.Command(bin, "demo", "--dir", work, "--report", report)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("demo failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(report); err != nil {
		t.Fatalf("report not written: %v", err)
	}

	verify := exec.Command(bin, "verify", "--jobs", "jobs.example.json",
		"--out", filepath.Join(t.TempDir(), "never-created"))
	if err := verify.Run(); err == nil {
		t.Fatal("verify on a missing output directory should fail")
	}
}
