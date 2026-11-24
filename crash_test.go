package mediaidx

import (
	"crypto/sha256"
	"errors"
	"os"
	"testing"
)

func simulateCrash(index *Index) error {
	indexError := index.indexFile.Close()
	dataError := index.dataFile.Close()
	index.closed = true
	if indexError != nil {
		return indexError
	}
	return dataError
}

func TestInterruptedTransactionDoesNotAdvanceCommit(t *testing.T) {
	index, indexPath, dataPath := openTestIndex(t, Options{})
	first := appendFrames(t, index, []byte("committed"))

	tx, err := index.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := tx.AppendFrame([]byte("interrupted-frame")); err != nil {
		t.Fatalf("AppendFrame: %v", err)
	}
	if _, err := tx.AppendSamples(480, []byte("interrupted-samples")); err != nil {
		t.Fatalf("AppendSamples: %v", err)
	}
	stats, err := index.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Uncommitted {
		t.Fatal("active transaction should be visible as uncommitted")
	}
	report, err := index.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete {
		t.Fatal("media with an uncommitted suffix must not verify complete")
	}
	if last := report.Issues[len(report.Issues)-1]; last.Code != IssueUncommittedSuffix {
		t.Fatalf("last issue = %+v", last)
	}
	if err := simulateCrash(index); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWithOptions(indexPath, Options{DataPath: dataPath})
	if err != nil {
		t.Fatalf("reopen after crash: %v", err)
	}
	defer reopened.Close()
	if got := reopened.CommitInfo(); got != first {
		t.Fatalf("commit advanced across crash: got %+v want %+v", got, first)
	}
	if got := reopened.Leaves(); len(got) != 1 || got[0].Locator != (Locator{Kind: Frame, Start: 0, Count: 1}) {
		t.Fatalf("uncommitted leaves visible: %+v", got)
	}
	report, err = reopened.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || report.LeafCount != 1 {
		t.Fatalf("recovered report: %+v", report)
	}
}

func TestTornCommitRecordIsDiscarded(t *testing.T) {
	index, indexPath, dataPath := openTestIndex(t, Options{})
	first := appendFrames(t, index, []byte("committed"))
	committedEnd, err := index.indexFile.Seek(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}

	uncommitted := []byte("partial-daily-segment")
	dataFile, err := os.OpenFile(dataPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataFile.WriteAt(uncommitted, first.DataSize); err != nil {
		t.Fatal(err)
	}
	if err := dataFile.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := dataFile.Close(); err != nil {
		t.Fatal(err)
	}

	digest := sha256.Sum256(uncommitted)
	leaf := Leaf{
		Seq:        1,
		Locator:    Locator{Kind: Frame, Start: 1, Count: 1},
		DataOffset: first.DataSize,
		DataSize:   int64(len(uncommitted)),
		Digest:     Hash(digest),
	}
	leafRecord := sealRecord(recordTypeLeaf, encodeLeafPayload(leaf))
	indexFile, err := os.OpenFile(indexPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	partialCommit := []byte{'M', 'D', 'R', '1', recordTypeCommit, 0, 0}
	if _, err := indexFile.WriteAt(append(leafRecord, partialCommit...), committedEnd); err != nil {
		t.Fatal(err)
	}
	if err := indexFile.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := indexFile.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWithOptions(indexPath, Options{DataPath: dataPath})
	if err != nil {
		t.Fatalf("reopen with torn commit: %v", err)
	}
	defer reopened.Close()
	if got := reopened.CommitInfo(); got != first {
		t.Fatalf("torn commit became visible: got %+v want %+v", got, first)
	}
	stats, err := reopened.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.DataSize != first.DataSize || stats.Uncommitted || stats.LeafCount != 1 {
		t.Fatalf("recovery did not roll back: %+v", stats)
	}
}

func TestTornLeafRecordIsDiscarded(t *testing.T) {
	index, indexPath, dataPath := openTestIndex(t, Options{})
	first := appendFrames(t, index, []byte("base"))
	committedEnd, err := index.indexFile.Seek(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}

	dataFile, err := os.OpenFile(dataPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataFile.WriteAt([]byte("tail"), first.DataSize); err != nil {
		t.Fatal(err)
	}
	if err := dataFile.Close(); err != nil {
		t.Fatal(err)
	}
	indexFile, err := os.OpenFile(indexPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := indexFile.WriteAt([]byte{'M', 'D'}, committedEnd); err != nil {
		t.Fatal(err)
	}
	if err := indexFile.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWithOptions(indexPath, Options{DataPath: dataPath})
	if err != nil {
		t.Fatalf("reopen with torn leaf: %v", err)
	}
	defer reopened.Close()
	if got := reopened.CommitInfo(); got != first {
		t.Fatalf("commit changed: %+v", got)
	}
	report, err := reopened.Verify()
	if err != nil || !report.Complete {
		t.Fatalf("verify after recovery: report=%+v err=%v", report, err)
	}
}

func TestAbortRollsBackDataAndLeaves(t *testing.T) {
	index, _, dataPath := openTestIndex(t, Options{})
	first := appendFrames(t, index, []byte("base"))
	tx, err := index.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.AppendFrame([]byte("discard-me")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if got := index.CommitInfo(); got != first {
		t.Fatalf("abort changed commit: %+v", got)
	}
	info, err := os.Stat(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != first.DataSize {
		t.Fatalf("abort left data: size=%d want=%d", info.Size(), first.DataSize)
	}
	if _, err := tx.Commit(); err != ErrNoTransaction {
		t.Fatalf("Commit after Abort = %v", err)
	}

	second := appendFrames(t, index, []byte("after-abort"))
	if second.Number != 2 || second.DataSize != first.DataSize+int64(len("after-abort")) {
		t.Fatalf("append after abort: %+v", second)
	}
	report, err := index.Verify()
	if err != nil || !report.Complete {
		t.Fatalf("verify after abort: report=%+v err=%v", report, err)
	}
}

func TestCorruptCommittedRecordFailsOpen(t *testing.T) {
	index, indexPath, dataPath := openTestIndex(t, Options{})
	appendFrames(t, index, []byte("frame-0"))
	appendFrames(t, index, []byte("frame-1"))
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(indexPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var value [1]byte
	if _, err := file.ReadAt(value[:], journalHeaderSize+20); err != nil {
		t.Fatal(err)
	}
	value[0] ^= 0xff
	if _, err := file.WriteAt(value[:], journalHeaderSize+20); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = OpenWithOptions(indexPath, Options{DataPath: dataPath})
	if !errors.Is(err, ErrCorruptIndex) {
		t.Fatalf("Open error = %v, want ErrCorruptIndex", err)
	}
}
