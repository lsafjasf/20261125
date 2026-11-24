package repair_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mp4index/fixture"
	"mp4index/repair"
)

func TestWriteNeverOverwritesSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	if _, err := fixture.Build(fixture.Config{Path: src, Frames: 12}); err != nil {
		t.Fatal(err)
	}
	srcHash := hashFile(t, src)

	// Output path identical to input is rejected.
	_, err := repair.RepairFile(src, src, repair.Options{})
	if !errors.Is(err, repair.ErrSourceDestinationSame) {
		t.Fatalf("same path: expected ErrSourceDestinationSame, got %v", err)
	}

	// Hardlink alias is rejected via SameFile.
	link := filepath.Join(dir, "link.mp4")
	if err := os.Link(src, link); err != nil {
		t.Skipf("hardlinks unsupported: %v", err)
	}
	_, err = repair.RepairFile(src, link, repair.Options{})
	if !errors.Is(err, repair.ErrSourceDestinationSame) {
		t.Fatalf("hardlink: expected ErrSourceDestinationSame, got %v", err)
	}
	if hashFile(t, src) != srcHash {
		t.Fatal("source changed after rejected writes")
	}

	// Destination under a regular file cannot create a temp output.
	blocker := filepath.Join(dir, "regular-file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	badDst := filepath.Join(blocker, "out.mp4")
	if _, err := repair.RepairFile(src, badDst, repair.Options{}); err == nil {
		t.Fatal("expected error for unwritable destination directory")
	}
	if hashFile(t, src) != srcHash {
		t.Fatal("source changed after failed output creation")
	}
	// stat must fail: the path is inaccessible (ENOTDIR) or absent.
	if _, err := os.Stat(badDst); err == nil {
		t.Fatal("no destination should exist after failure")
	}

	// A successful run leaves source intact and writes a distinct file.
	dst := filepath.Join(dir, "out", "repaired.mp4")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := repair.RepairFile(src, dst, repair.Options{FPS: 15}); err != nil {
		t.Fatalf("successful repair: %v", err)
	}
	if hashFile(t, src) != srcHash {
		t.Fatal("source changed after successful repair")
	}
	entries, _ := os.ReadDir(filepath.Dir(dst))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
}

func TestReplacesExistingGeneratedOutput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	if _, err := fixture.Build(fixture.Config{Path: src, Frames: 8}); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "repaired.mp4")
	if err := os.WriteFile(dst, []byte("stale artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repair.RepairFile(src, dst, repair.Options{FPS: 15}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dst); err != nil || info.Size() < 100 {
		t.Fatalf("destination not replaced with repaired movie: size=%d err=%v", info.Size(), err)
	}
}
