package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunSamplesDryRunDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.wav")
	original := testWAV(t, [][]int16{{1, 10}, {2, 20}})
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := run([]string{
		"repair", "--file", path,
		"--mode", "samples", "--mapping", "2,1", "--dry-run",
	}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}, time.Now)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("dry run modified input file")
	}
	if !strings.Contains(stdout.String(), "Dry run") {
		t.Fatal("output missing dry-run marker")
	}
}

func TestRunDeclarationRepairAndRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.wav")
	original := testWAV(t, [][]int16{{1, 10}, {2, 20}})
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := run([]string{
		"repair", "--file", path,
		"--mode", "declaration", "--names", "FR,FL", "--yes",
	}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}, time.Now)
	if err != nil {
		t.Fatalf("run repair: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "与预期输出一致: true") {
		t.Fatalf("repair output missing successful readback: %s", output)
	}
	if !strings.Contains(output, "回滚命令:") {
		t.Fatalf("repair output missing rollback command: %s", output)
	}

	stdout.Reset()
	err = run([]string{"rollback", "--file", path}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}, time.Now)
	if err != nil {
		t.Fatalf("run rollback: %v", err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("rollback did not restore original file")
	}
}
