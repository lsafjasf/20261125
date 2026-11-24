package ffv1arc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestIncrementalReindex proves that a frame index drift on one block recomputes
// only that block's frames while every other block's index stays untouched.
func TestIncrementalReindex(t *testing.T) {
	a, dir := newTestArchive(t)
	id1 := mustAppend(t, a, "2026-W47", makeFrames(2))
	id2 := mustAppend(t, a, "2026-W48", makeFrames(3))

	before1, err := a.SnapshotBlock(id1)
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the manifest's frame index for block 2 only (out-of-band edit).
	manifestFile := filepath.Join(dir, manifestRelPath)
	raw, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatal(err)
	}
	for _, b := range man.Blocks {
		if b.ID == id2 && len(b.Frames) > 0 {
			b.Frames[0].Offset = 999999
			b.Frames[0].Length = 7
		}
	}
	edited, err := json.MarshalIndent(&man, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestFile, edited, 0o644); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := reopened.Check()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.ReindexedBlocks) != 1 || rep.ReindexedBlocks[0] != id2 {
		t.Fatalf("only block %d must be reindexed, got %v", id2, rep.ReindexedBlocks)
	}
	if rep.ReindexedFrames != 3 {
		t.Fatalf("exactly 3 frame indices rebuilt, got %d", rep.ReindexedFrames)
	}

	after1, err := reopened.SnapshotBlock(id1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range before1.Frames {
		if before1.Frames[i] != after1.Frames[i] {
			t.Fatalf("unrelated block %d frame %d was modified", id1, i)
		}
	}

	after2, err := reopened.SnapshotBlock(id2)
	if err != nil {
		t.Fatal(err)
	}
	if after2.Frames[0].Offset == 999999 || after2.Frames[0].Length == 7 {
		t.Fatal("drifted frame index was not repaired from the segment")
	}
	assertNoAlarm(t, rep, AlarmPayloadTampered)
}
