package ffv1arc

import (
	"os"
	"testing"
)

// TestLegitimateRename covers the normal-rename class: an archive-managed
// rename updates manifest and file atomically, keeps payload offsets valid and
// produces no alarms and no frame index recomputation.
func TestLegitimateRename(t *testing.T) {
	a, _ := newTestArchive(t)
	frames := makeFrames(3)
	id := mustAppend(t, a, "2026-W48", frames)

	before, err := a.SnapshotBlock(id)
	if err != nil {
		t.Fatal(err)
	}
	newName := "weekly-2026-W48-archived.mkv"
	if err := a.Rename(id, newName); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if _, err := os.Stat(segmentPathFor(a, before.SegmentName)); !os.IsNotExist(err) {
		t.Fatal("old segment file should no longer exist")
	}
	if _, err := os.Stat(segmentPathFor(a, newName)); err != nil {
		t.Fatalf("renamed segment missing: %v", err)
	}

	after, err := a.SnapshotBlock(id)
	if err != nil {
		t.Fatal(err)
	}
	if after.SegmentName != newName {
		t.Fatalf("manifest name not updated: %q", after.SegmentName)
	}
	if after.PayloadHash != before.PayloadHash {
		t.Fatal("payload hash changed after rename")
	}
	if len(after.Frames) != len(before.Frames) {
		t.Fatal("frame count changed after rename")
	}
	for i := range before.Frames {
		if before.Frames[i] != after.Frames[i] {
			t.Fatalf("frame index %d changed after rename: %+v vs %+v", i, before.Frames[i], after.Frames[i])
		}
	}

	rep, err := a.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(rep.Alarms) != 0 {
		t.Fatalf("managed rename must be alarm-free, got %v", rep.Alarms)
	}
	if rep.ReindexedFrames != 0 {
		t.Fatalf("managed rename must not recompute frame indices, got %d", rep.ReindexedFrames)
	}

	// Parsed payload at the new path still verifies frame-for-frame.
	ps, err := parseSegment(segmentPathFor(a, newName))
	if err != nil {
		t.Fatal(err)
	}
	if !ps.sealed || len(ps.frames) != len(frames) {
		t.Fatalf("renamed segment no longer parses as sealed: %+v", ps)
	}
	for i, fr := range ps.frames {
		if fr.Hash != hashB64(frames[i]) {
			t.Fatalf("frame %d payload mismatch after rename", i+1)
		}
	}
}

func TestRenameGuards(t *testing.T) {
	a, dir := newTestArchive(t)

	if err := a.Rename(99, "x.mkv"); err == nil {
		t.Fatal("renaming unknown block must fail")
	}
	if err := a.Rename(1, "../escape.mkv"); err == nil {
		t.Fatal("path traversal rename must be rejected")
	}

	frames := makeFrames(2)
	if _, err := a.Append("2026-W48", frames, AppendOptions{CrashAfter: 1}); err == nil {
		t.Fatal("expected simulated crash")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Rename(1, "x.mkv"); err == nil {
		t.Fatal("incomplete block must not be renameable")
	}
}
