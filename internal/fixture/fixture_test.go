package fixture

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ffv1compare/internal/testutil"
)

func TestReorderIsDeterministic(t *testing.T) {
	src := testutil.DataPath("machine-a.mkv")
	want := testutil.DataPath("machine-b-reorder.mkv")
	dst := filepath.Join(t.TempDir(), "reorder.mkv")
	if err := ReorderSlices(src, dst, 1, 2); err != nil {
		t.Fatal(err)
	}
	gotBytes, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatal("generated reorder fixture differs from checked-in fixture")
	}
}

func TestShiftIsDeterministic(t *testing.T) {
	src := testutil.DataPath("machine-a.mkv")
	want := testutil.DataPath("machine-b-shift.mkv")
	dst := filepath.Join(t.TempDir(), "shift.mkv")
	if err := ShiftTimecodes(src, dst, 500); err != nil {
		t.Fatal(err)
	}
	gotBytes, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatal("generated shift fixture differs from checked-in fixture")
	}
}
