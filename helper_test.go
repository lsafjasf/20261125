package ffv1arc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func newTestArchive(t *testing.T) (*Archive, string) {
	t.Helper()
	dir := t.TempDir()
	a, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return a, dir
}

func makeFrames(n int) [][]byte {
	frames := make([][]byte, n)
	for i := range frames {
		frames[i] = []byte(fmt.Sprintf("frame-%04d-payload", i+1))
	}
	return frames
}

func mustAppend(t *testing.T, a *Archive, chapter string, frames [][]byte) int {
	t.Helper()
	res, err := a.Append(chapter, frames, AppendOptions{})
	if err != nil {
		t.Fatalf("Append(%q): %v", chapter, err)
	}
	if !res.Sealed || res.Frames != len(frames) {
		t.Fatalf("unexpected append result: %+v", res)
	}
	return res.BlockID
}

func assertAlarm(t *testing.T, rep *CheckReport, kind AlarmKind, blockID int) {
	t.Helper()
	for _, al := range rep.Alarms {
		if al.Kind == kind && al.BlockID == blockID {
			return
		}
	}
	t.Fatalf("expected alarm %s for block %d, got alarms: %v", kind, blockID, rep.Alarms)
}

func assertNoAlarm(t *testing.T, rep *CheckReport, kind AlarmKind) {
	t.Helper()
	for _, al := range rep.Alarms {
		if al.Kind == kind {
			t.Fatalf("did not expect alarm %s, got %v", kind, rep.Alarms)
		}
	}
}

// flipFrameByte flips one byte inside a frame payload on disk without touching
// frame headers or the footer, simulating localized payload tampering.
func flipFrameByte(t *testing.T, path string, frameIndex, offsetInFrame int) {
	t.Helper()
	ps, err := parseSegment(path)
	if err != nil {
		t.Fatalf("parseSegment before tamper: %v", err)
	}
	if frameIndex >= len(ps.frames) {
		t.Fatalf("frame index %d out of range", frameIndex)
	}
	fr := ps.frames[frameIndex]
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open for tamper: %v", err)
	}
	defer f.Close()
	buf := make([]byte, 1)
	if _, err := f.ReadAt(buf, fr.Offset+int64(offsetInFrame)); err != nil {
		t.Fatalf("read byte: %v", err)
	}
	buf[0] ^= 0xff
	if _, err := f.WriteAt(buf, fr.Offset+int64(offsetInFrame)); err != nil {
		t.Fatalf("flip byte: %v", err)
	}
}

func segmentPathFor(a *Archive, name string) string {
	return filepath.Join(a.root, segDir, name)
}

func assertErrIs(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("expected error %v, got %v", target, err)
	}
}
