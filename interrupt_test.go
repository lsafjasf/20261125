package ffv1arc

import (
	"errors"
	"testing"
)

// TestInterruptedAppend covers the interruption class: a crash mid-append
// leaves an open block whose frame index is reconciled incrementally; earlier
// sealed blocks are never recomputed.
func TestInterruptedAppend(t *testing.T) {
	a, dir := newTestArchive(t)
	full := makeFrames(5)

	sealedID := mustAppend(t, a, "2026-W47", full[:2])
	sealedBefore, err := a.SnapshotBlock(sealedID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Append("2026-W48", full, AppendOptions{CrashAfter: 3})
	assertErrIs(t, err, ErrSimulatedCrash)

	// Reopen the archive like a fresh process after the crash.
	a, err = Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	open, err := a.SnapshotBlock(2)
	if err != nil {
		t.Fatalf("open block missing after crash: %v", err)
	}
	if open.Sealed {
		t.Fatal("block must remain open after a crash")
	}
	if len(open.Frames) != 3 {
		t.Fatalf("manifest should hold 3 durable frames, got %d", len(open.Frames))
	}

	// A new append is refused while an open block exists.
	if _, err := a.Append("2026-W49", makeFrames(1), AppendOptions{}); !errors.Is(err, ErrBlockStillOpen) {
		t.Fatalf("expected ErrBlockStillOpen, got %v", err)
	}

	rec, err := a.Recover()
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if len(rec.RecoveredBlocks) != 1 || rec.RecoveredBlocks[0] != 2 {
		t.Fatalf("recover should touch only block 2, got %+v", rec)
	}
	if rec.ReindexedFrames != 3 {
		t.Fatalf("only 3 frame indices may be rebuilt, got %d", rec.ReindexedFrames)
	}

	// The previously sealed block is byte-for-byte untouched in the manifest.
	sealedAfter, err := a.SnapshotBlock(sealedID)
	if err != nil {
		t.Fatal(err)
	}
	if sealedAfter.PayloadHash != sealedBefore.PayloadHash {
		t.Fatal("sealed block payload hash changed during recovery")
	}
	if len(sealedAfter.Frames) != len(sealedBefore.Frames) {
		t.Fatal("sealed block frame index was recomputed")
	}

	res, err := a.Resume(2, full)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !res.Sealed || res.Frames != 5 {
		t.Fatalf("resume should seal all 5 frames, got %+v", res)
	}

	rep, err := a.Check()
	if err != nil {
		t.Fatalf("Check after resume: %v", err)
	}
	if len(rep.Alarms) != 0 {
		t.Fatalf("healthy archive must have no alarms, got %v", rep.Alarms)
	}
	if rep.ReindexedFrames != 0 {
		t.Fatalf("no frame index recomputation expected on a healthy check, got %d", rep.ReindexedFrames)
	}
}

func TestInterruptedBeforeAnyFrame(t *testing.T) {
	a, dir := newTestArchive(t)
	frames := makeFrames(3)

	if _, err := a.Append("2026-W48", frames, AppendOptions{CrashAfter: 1}); err == nil {
		t.Fatal("expected simulated crash error")
	}
	a2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := a2.Recover()
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.RecoveredBlocks) != 1 || rec.TruncatedFrames[1] != 1 {
		t.Fatalf("unexpected recovery report: %+v", rec)
	}
	if _, err := a2.Resume(1, frames); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	rep, err := a2.Check()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Alarms) != 0 {
		t.Fatalf("alarms after full recovery: %v", rep.Alarms)
	}
}
