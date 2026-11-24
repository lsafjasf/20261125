package mediaidx

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openTestIndex(t *testing.T, options Options) (*Index, string, string) {
	t.Helper()
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "media.idx")
	dataPath := filepath.Join(dir, "media.bin")
	options.DataPath = dataPath
	index, err := OpenWithOptions(indexPath, options)
	if err != nil {
		t.Fatalf("OpenWithOptions: %v", err)
	}
	return index, indexPath, dataPath
}

func appendFrames(t *testing.T, index *Index, frames ...[]byte) CommitInfo {
	t.Helper()
	tx, err := index.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, frame := range frames {
		if _, err := tx.AppendFrame(frame); err != nil {
			t.Fatalf("AppendFrame: %v", err)
		}
	}
	info, err := tx.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return info
}

func TestAppendReopenAndVerify(t *testing.T) {
	index, indexPath, dataPath := openTestIndex(t, Options{})
	first := appendFrames(t, index,
		[]byte("frame-zero"),
		[]byte("frame-one"),
		[]byte("frame-two"),
	)
	if first.LeafCount != 3 || first.Number != 1 {
		t.Fatalf("unexpected first commit: %+v", first)
	}
	if err := index.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenWithOptions(indexPath, Options{DataPath: dataPath})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if got := reopened.CommitInfo(); got != first {
		t.Fatalf("commit not recovered: got %+v want %+v", got, first)
	}
	report, err := reopened.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !report.Complete || report.VerifiedLeaves != 3 {
		t.Fatalf("unexpected report: %+v", report)
	}

	second := appendFrames(t, reopened, []byte("frame-three"), []byte("frame-four"))
	if second.Number != 2 || second.LeafCount != 5 {
		t.Fatalf("unexpected second commit: %+v", second)
	}
	if second.Root == first.Root {
		t.Fatal("root did not change")
	}
	leaves := reopened.Leaves()
	if leaves[0].Digest != Hash(sha256.Sum256([]byte("frame-zero"))) {
		t.Fatalf("old leaf changed: %+v", leaves[0])
	}
	if leaves[4].Locator != (Locator{Kind: Frame, Start: 4, Count: 1}) {
		t.Fatalf("new leaf locator mismatch: %+v", leaves[4])
	}

	report, err = reopened.Verify()
	if err != nil || !report.Complete {
		t.Fatalf("verify after append: report=%+v err=%v", report, err)
	}
	stats, err := reopened.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Uncommitted || stats.LeafCount != 5 || stats.DataSize != second.DataSize {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestPersistedFrontierReconstructsRoot(t *testing.T) {
	index, indexPath, dataPath := openTestIndex(t, Options{NoSync: true})
	frames := [][]byte{[]byte("a"), []byte("bb"), []byte("ccc"), []byte("dddd"), []byte("eeeee")}
	info := appendFrames(t, index, frames...)
	leaves := index.Leaves()
	if err := index.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var expected mmrState
	for _, leaf := range leaves {
		expected.appendLeaf(leaf.Digest)
	}
	if info.Root != expected.root() {
		t.Fatalf("commit root %x != independently rebuilt root %x", info.Root, expected.root())
	}
	reopened, err := OpenWithOptions(indexPath, Options{DataPath: dataPath, NoSync: true})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if reopened.mmr.count != uint64(len(frames)) {
		t.Fatalf("restored MMR count = %d", reopened.mmr.count)
	}
	if reopened.mmr.root() != info.Root {
		t.Fatalf("restored frontier root mismatch")
	}
	if reopened.mmr.appended != 0 {
		t.Fatalf("open rehashed %d leaves", reopened.mmr.appended)
	}
	tx, err := reopened.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.AppendFrame([]byte("new-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.AppendFrame([]byte("new-2")); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if reopened.mmr.appended != 2 {
		t.Fatalf("incremental append hashed %d leaves, want 2", reopened.mmr.appended)
	}
}

func TestTailTruncationLocatesFrame(t *testing.T) {
	index, indexPath, dataPath := openTestIndex(t, Options{})
	appendFrames(t, index,
		bytes.Repeat([]byte("a"), 10),
		bytes.Repeat([]byte("b"), 10),
		bytes.Repeat([]byte("c"), 10),
	)
	if err := index.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	file, err := os.OpenFile(dataPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(27); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWithOptions(indexPath, Options{DataPath: dataPath})
	if err != nil {
		t.Fatalf("reopen truncated media: %v", err)
	}
	defer reopened.Close()
	if _, err := reopened.Begin(); !errors.Is(err, ErrDamaged) {
		t.Fatalf("Begin error = %v, want ErrDamaged", err)
	}
	report, err := reopened.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Complete || len(report.Issues) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	issue := report.Issues[0]
	if issue.Code != IssueTailTruncated || issue.Locator != (Locator{Kind: Frame, Start: 2, Count: 1}) {
		t.Fatalf("unexpected issue: %+v", issue)
	}
	if issue.Available != 7 || issue.MissingBytes != 3 {
		t.Fatalf("unexpected truncation detail: %+v", issue)
	}
}

func TestLocalReplacementLocatesSampleSegment(t *testing.T) {
	index, indexPath, dataPath := openTestIndex(t, Options{})
	tx, err := index.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.AppendSamples(100, bytes.Repeat([]byte{1}, 20)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.AppendSamples(200, bytes.Repeat([]byte{2}, 20)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.AppendSamples(300, bytes.Repeat([]byte{3}, 20)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(dataPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte{9, 9, 9}, 25); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWithOptions(indexPath, Options{DataPath: dataPath})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	report, err := reopened.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Complete || len(report.Issues) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	issue := report.Issues[0]
	if issue.Code != IssueCorruptLeaf || issue.Locator != (Locator{Kind: Sample, Start: 100, Count: 200}) {
		t.Fatalf("unexpected issue: %+v", issue)
	}
	if issue.DataOffset != 20 || issue.DataSize != 20 {
		t.Fatalf("unexpected byte range: %+v", issue)
	}
	if single, err := reopened.VerifyLeaf(0); err != nil || single != nil {
		t.Fatalf("healthy leaf check: issue=%+v err=%v", single, err)
	}
	if single, err := reopened.VerifyLeaf(1); err != nil || single == nil || single.Locator != issue.Locator {
		t.Fatalf("damaged leaf check: issue=%+v err=%v", single, err)
	}

	located, ok := reopened.LocateOffset(25)
	if !ok || located.Seq != 1 {
		t.Fatalf("LocateOffset = %+v, %v", located, ok)
	}
	leaves, err := reopened.LocateRange(19, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaves) != 2 || leaves[0].Seq != 0 || leaves[1].Seq != 1 {
		t.Fatalf("LocateRange = %+v", leaves)
	}
	unit, ok := reopened.LocateUnit(Sample, 250)
	if !ok || unit.Seq != 1 {
		t.Fatalf("LocateUnit = %+v, %v", unit, ok)
	}
}

func TestUnitGapIsReported(t *testing.T) {
	index, _, _ := openTestIndex(t, Options{})
	tx, err := index.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.AppendFrame([]byte("frame-0")); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.AppendFrameAt(3, []byte("frame-3")); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	defer index.Close()

	report, err := index.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || !report.HasGap || len(report.Issues) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	issue := report.Issues[0]
	if issue.Code != IssueUnitGap ||
		issue.Locator != (Locator{Kind: Frame, Start: 1, Count: 2}) ||
		issue.MissingUnits != 2 {
		t.Fatalf("unexpected gap: %+v", issue)
	}
	stats, err := index.Stats()
	if err != nil || !stats.HasGap {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
}
