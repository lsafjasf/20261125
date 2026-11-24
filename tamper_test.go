package ffv1arc

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLocalTampering covers the localized-tamper class with three distinct
// alarm channels: payload edits, out-of-band renames and chapter edits.
func TestLocalTampering(t *testing.T) {
	t.Run("PayloadByteFlip", func(t *testing.T) {
		a, _ := newTestArchive(t)
		id := mustAppend(t, a, "2026-W48", makeFrames(3))
		state, err := a.SnapshotBlock(id)
		if err != nil {
			t.Fatal(err)
		}
		path := segmentPathFor(a, state.SegmentName)
		origHash := state.Frames[1].Hash

		flipFrameByte(t, path, 1, len("frame-0002-payload")-1)

		rep, err := a.Check()
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		assertAlarm(t, rep, AlarmPayloadTampered, id)

		// Tampered hash must not be silently re-baselined into the index.
		after, err := a.SnapshotBlock(id)
		if err != nil {
			t.Fatal(err)
		}
		if after.Frames[1].Hash != origHash {
			t.Fatal("checker absorbed tampered payload into the frame index")
		}
		if rep.HasAlarm(AlarmNameChanged, AlarmChapterChanged) {
			t.Fatalf("payload edit must not trigger rename/chapter alarms: %v", rep.Alarms)
		}

		// A second check keeps alarming: the tamper remains visible.
		rep2, err := a.Check()
		if err != nil {
			t.Fatal(err)
		}
		assertAlarm(t, rep2, AlarmPayloadTampered, id)
	})

	t.Run("OutOfBandRename", func(t *testing.T) {
		a, _ := newTestArchive(t)
		id := mustAppend(t, a, "2026-W48", makeFrames(2))
		state, err := a.SnapshotBlock(id)
		if err != nil {
			t.Fatal(err)
		}
		oldPath := segmentPathFor(a, state.SegmentName)
		newName := "block-0001-RENAMED.mkv"
		if err := os.Rename(oldPath, filepath.Join(a.root, segDir, newName)); err != nil {
			t.Fatal(err)
		}

		rep, err := a.Check()
		if err != nil {
			t.Fatal(err)
		}
		assertAlarm(t, rep, AlarmNameChanged, id)
		if rep.HasAlarm(AlarmPayloadTampered, AlarmChapterChanged) {
			t.Fatalf("rename must not look like payload/chapter tamper: %v", rep.Alarms)
		}
		// Payload is intact, so no frame indices need rebuilding.
		if rep.ReindexedFrames != 0 {
			t.Fatalf("rename of intact payload must not recompute frames, got %d", rep.ReindexedFrames)
		}
	})

	t.Run("ChapterEdited", func(t *testing.T) {
		a, dir := newTestArchive(t)
		id := mustAppend(t, a, "2026-W48", makeFrames(2))
		// Edit the chapter directly in the manifest (out-of-band metadata edit).
		manifestFile := filepath.Join(dir, manifestRelPath)
		data, err := os.ReadFile(manifestFile)
		if err != nil {
			t.Fatal(err)
		}
		edited := bytesReplace(data, []byte("2026-W48"), []byte("2099-W48"))
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
		assertAlarm(t, rep, AlarmChapterChanged, id)
		if rep.HasAlarm(AlarmPayloadTampered, AlarmNameChanged) {
			t.Fatalf("chapter edit must not look like payload/rename tamper: %v", rep.Alarms)
		}
	})

	t.Run("SegmentHeaderChapterEdited", func(t *testing.T) {
		a, _ := newTestArchive(t)
		id := mustAppend(t, a, "2026-W48", makeFrames(2))
		state, err := a.SnapshotBlock(id)
		if err != nil {
			t.Fatal(err)
		}
		path := segmentPathFor(a, state.SegmentName)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		edited := bytesReplace(data, []byte("2026-W48"), []byte("2099-W48"))
		if err := os.WriteFile(path, edited, 0o644); err != nil {
			t.Fatal(err)
		}

		rep, err := a.Check()
		if err != nil {
			t.Fatal(err)
		}
		assertAlarm(t, rep, AlarmChapterChanged, id)
		assertNoAlarm(t, rep, AlarmNameChanged)
	})
}

func bytesReplace(haystack, old, new []byte) []byte {
	out := make([]byte, 0, len(haystack))
	for i := 0; i < len(haystack); {
		if i+len(old) <= len(haystack) && string(haystack[i:i+len(old)]) == string(old) {
			out = append(out, new...)
			i += len(old)
			continue
		}
		out = append(out, haystack[i])
		i++
	}
	return out
}
