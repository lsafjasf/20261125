package ffv1arc

import (
	"errors"
	"os"
	"testing"
)

// TestDuplicateAppend covers the repeated-append class: resubmitting the same
// weekly fragment is rejected, and sealed blocks cannot be rewritten.
func TestDuplicateAppend(t *testing.T) {
	a, _ := newTestArchive(t)
	frames := makeFrames(4)
	id := mustAppend(t, a, "2026-W48", frames)

	stateBefore, err := a.SnapshotBlock(id)
	if err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(segmentPathFor(a, stateBefore.SegmentName))
	if err != nil {
		t.Fatal(err)
	}

	// Exact same fragment + chapter: duplicate, must be refused with no writes.
	_, err = a.Append("2026-W48", frames, AppendOptions{})
	assertErrIs(t, err, ErrDuplicateAppend)

	// Same payload under another chapter is still a duplicate payload and must
	// not create a second copy of the same data.
	_, err = a.Append("2026-W49", frames, AppendOptions{})
	assertErrIs(t, err, ErrDuplicateAppend)

	if ids := a.BlockIDs(); len(ids) != 1 || ids[0] != id {
		t.Fatalf("duplicate append must not create blocks, got %v", ids)
	}

	stateAfter, err := a.SnapshotBlock(id)
	if err != nil {
		t.Fatal(err)
	}
	if stateAfter.PayloadHash != stateBefore.PayloadHash || len(stateAfter.Frames) != len(stateBefore.Frames) {
		t.Fatal("sealed block metadata was modified by a rejected duplicate append")
	}
	infoAfter, err := os.Stat(segmentPathFor(a, stateBefore.SegmentName))
	if err != nil {
		t.Fatal(err)
	}
	if infoAfter.Size() != infoBefore.Size() || infoAfter.ModTime() != infoBefore.ModTime() {
		t.Fatal("sealed segment file was rewritten")
	}

	// An explicit id collision is refused as well.
	_, err = a.Append("2026-W50", makeFrames(2), AppendOptions{BlockID: id})
	assertErrIs(t, err, ErrBlockIDCollision)

	// Resume on a sealed block is a rewrite attempt and must fail.
	if _, err := a.Resume(id, frames); !errors.Is(err, ErrBlockSealed) {
		t.Fatalf("expected ErrBlockSealed, got %v", err)
	}

	rep, err := a.Check()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Alarms) != 0 || rep.ReindexedFrames != 0 {
		t.Fatalf("rejected appends must leave a clean archive, got %+v", rep)
	}
}

func TestResumeRejectsMismatchedFragment(t *testing.T) {
	a, dir := newTestArchive(t)
	frames := makeFrames(4)
	if _, err := a.Append("2026-W48", frames, AppendOptions{CrashAfter: 2}); !errors.Is(err, ErrSimulatedCrash) {
		t.Fatalf("expected simulated crash, got %v", err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	wrong := makeFrames(4)
	wrong[0] = []byte("totally-different-frame-bytes")
	_, err = reopened.Resume(1, wrong)
	assertErrIs(t, err, ErrInvalidSegment)

	state, err := reopened.SnapshotBlock(1)
	if err != nil {
		t.Fatal(err)
	}
	if state.Sealed {
		t.Fatal("failed resume must not seal the block")
	}
}
