package mediaidx

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
)

// Tx is an atomic append transaction. Leaves become durable only when Commit
// returns successfully.
type Tx struct {
	index      *Index
	leaves     []Leaf
	dataSize   int64
	mmr        mmrState
	nextByKind map[Kind]uint64
	hasGap     bool
	finished   bool
}

// Len returns the number of staged leaves.
func (tx *Tx) Len() int {
	tx.index.mu.Lock()
	defer tx.index.mu.Unlock()
	return len(tx.leaves)
}

// Append adds a leaf using an explicit media coordinate.
func (tx *Tx) Append(locator Locator, data []byte) (*Leaf, error) {
	tx.index.mu.Lock()
	defer tx.index.mu.Unlock()
	return tx.appendLocked(locator, data)
}

// AppendFrame appends one video frame at the next frame number.
func (tx *Tx) AppendFrame(data []byte) (*Leaf, error) {
	tx.index.mu.Lock()
	defer tx.index.mu.Unlock()
	return tx.appendLocked(Locator{Kind: Frame, Start: tx.nextByKind[Frame], Count: 1}, data)
}

// AppendFrameAt appends one video frame at an explicit frame number.
func (tx *Tx) AppendFrameAt(frame uint64, data []byte) (*Leaf, error) {
	return tx.Append(Locator{Kind: Frame, Start: frame, Count: 1}, data)
}

// AppendSamples appends an audio sample segment at the next sample position.
func (tx *Tx) AppendSamples(count uint64, data []byte) (*Leaf, error) {
	tx.index.mu.Lock()
	defer tx.index.mu.Unlock()
	return tx.appendLocked(Locator{Kind: Sample, Start: tx.nextByKind[Sample], Count: count}, data)
}

// AppendSamplesAt appends an audio sample segment at an explicit position.
func (tx *Tx) AppendSamplesAt(start, count uint64, data []byte) (*Leaf, error) {
	return tx.Append(Locator{Kind: Sample, Start: start, Count: count}, data)
}

func (tx *Tx) appendLocked(locator Locator, data []byte) (*Leaf, error) {
	if err := tx.activeLocked(); err != nil {
		return nil, err
	}
	if !locator.Kind.valid() || locator.Count == 0 {
		return nil, fmt.Errorf("%w: %+v", ErrInvalidLocator, locator)
	}
	if locator.Kind == Frame && locator.Count != 1 {
		return nil, fmt.Errorf("%w: a frame leaf must cover exactly one frame", ErrInvalidLocator)
	}
	endUnit, ok := locator.End()
	if !ok {
		return nil, fmt.Errorf("%w: unit range overflow", ErrInvalidLocator)
	}
	next := tx.nextByKind[locator.Kind]
	if locator.Start < next {
		return nil, fmt.Errorf("%w: %s starts at %d before next position %d", ErrInvalidLocator, locator.Kind, locator.Start, next)
	}
	if locator.Start > next {
		tx.hasGap = true
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty leaf", ErrInvalidData)
	}
	if len(data) > math.MaxInt64 || tx.dataSize > math.MaxInt64-int64(len(data)) {
		return nil, fmt.Errorf("%w: media size overflow", ErrInvalidData)
	}

	digest := sha256.Sum256(data)
	leaf := Leaf{
		Seq:        uint64(len(tx.index.leaves) + len(tx.leaves)),
		Locator:    locator,
		DataOffset: tx.dataSize,
		DataSize:   int64(len(data)),
		Digest:     Hash(digest),
	}
	if err := writeAllAt(tx.index.dataFile, data, tx.dataSize); err != nil {
		cleanupErr := tx.index.failCommitLocked(tx)
		return nil, joinFailure(fmt.Errorf("write media leaf: %w", err), cleanupErr)
	}

	tx.dataSize += int64(len(data))
	tx.leaves = append(tx.leaves, leaf)
	tx.mmr.appendLeaf(leaf.Digest)
	tx.nextByKind[locator.Kind] = endUnit
	return &leaf, nil
}

func (tx *Tx) activeLocked() error {
	if tx.finished || tx.index.tx != tx {
		return ErrNoTransaction
	}
	if tx.index.closed {
		return ErrClosed
	}
	return nil
}

// Commit makes every staged leaf visible in one durable journal commit.
func (tx *Tx) Commit() (CommitInfo, error) {
	index := tx.index
	index.mu.Lock()
	defer index.mu.Unlock()
	if err := tx.activeLocked(); err != nil {
		return CommitInfo{}, err
	}
	if len(tx.leaves) == 0 {
		return CommitInfo{}, ErrEmptyTransaction
	}
	if !index.noSync {
		if err := index.dataFile.Sync(); err != nil {
			cleanupErr := index.failCommitLocked(tx)
			return CommitInfo{}, joinFailure(fmt.Errorf("sync media before commit: %w", err), cleanupErr)
		}
	}

	if _, err := index.indexFile.Seek(index.committedIndexSize, io.SeekStart); err != nil {
		cleanupErr := index.failCommitLocked(tx)
		return CommitInfo{}, joinFailure(fmt.Errorf("seek index before commit: %w", err), cleanupErr)
	}
	writer := bufio.NewWriterSize(index.indexFile, 64*1024)
	var journalBytes int64
	for _, leaf := range tx.leaves {
		record := sealRecord(recordTypeLeaf, encodeLeafPayload(leaf))
		written, err := writer.Write(record)
		if err != nil {
			cleanupErr := index.failCommitLocked(tx)
			return CommitInfo{}, joinFailure(fmt.Errorf("write leaf record: %w", err), cleanupErr)
		}
		journalBytes += int64(written)
	}

	commit := commitPayload{
		Number:    index.info.Number + 1,
		LeafCount: uint64(len(index.leaves) + len(tx.leaves)),
		DataSize:  tx.dataSize,
		Previous:  index.info.ID,
		Root:      tx.mmr.root(),
		Peaks:     tx.mmr.peakHashes(),
	}
	payload, err := encodeCommitPayload(commit)
	if err != nil {
		cleanupErr := index.failCommitLocked(tx)
		return CommitInfo{}, joinFailure(err, cleanupErr)
	}
	commitRecord := sealRecord(recordTypeCommit, payload)
	if _, err := writer.Write(commitRecord); err != nil {
		cleanupErr := index.failCommitLocked(tx)
		return CommitInfo{}, joinFailure(fmt.Errorf("write commit record: %w", err), cleanupErr)
	}
	if err := writer.Flush(); err != nil {
		cleanupErr := index.failCommitLocked(tx)
		return CommitInfo{}, joinFailure(fmt.Errorf("flush index: %w", err), cleanupErr)
	}
	if !index.noSync {
		if err := index.indexFile.Sync(); err != nil {
			cleanupErr := index.failCommitLocked(tx)
			return CommitInfo{}, joinFailure(fmt.Errorf("sync index: %w", err), cleanupErr)
		}
	}

	journalBytes += int64(len(commitRecord))
	index.leaves = append(index.leaves, tx.leaves...)
	index.mmr = tx.mmr.clone()
	index.info = CommitInfo{
		Number:    commit.Number,
		LeafCount: commit.LeafCount,
		DataSize:  commit.DataSize,
		Root:      commit.Root,
		ID:        recordID(commitRecord[:len(commitRecord)-recordTrailerSize]),
	}
	index.committedIndexSize += journalBytes
	index.committedDataSize = tx.dataSize
	index.nextByKind = make(map[Kind]uint64, len(tx.nextByKind))
	for kind, value := range tx.nextByKind {
		index.nextByKind[kind] = value
	}
	index.hasGap = index.hasGap || tx.hasGap
	index.tx = nil
	tx.finished = true
	return index.info, nil
}

// Abort discards every staged leaf and any media bytes written by the
// transaction.
func (tx *Tx) Abort() error {
	index := tx.index
	index.mu.Lock()
	defer index.mu.Unlock()
	if tx.finished || index.tx != tx {
		return ErrNoTransaction
	}
	return index.abortLocked(tx)
}

func (index *Index) abortLocked(tx *Tx) error {
	index.tx = nil
	tx.finished = true
	indexError := index.indexFile.Truncate(index.committedIndexSize)
	dataError := index.dataFile.Truncate(index.committedDataSize)
	_, seekIndexErr := index.indexFile.Seek(index.committedIndexSize, io.SeekStart)
	_, seekDataErr := index.dataFile.Seek(index.committedDataSize, io.SeekStart)
	if indexError != nil {
		return fmt.Errorf("truncate index during abort: %w", indexError)
	}
	if dataError != nil {
		return fmt.Errorf("truncate media during abort: %w", dataError)
	}
	if seekIndexErr != nil {
		return seekIndexErr
	}
	return seekDataErr
}

func (index *Index) failCommitLocked(tx *Tx) error {
	index.tx = nil
	tx.finished = true
	indexError := index.indexFile.Truncate(index.committedIndexSize)
	dataError := index.dataFile.Truncate(index.committedDataSize)
	_, seekIndexErr := index.indexFile.Seek(index.committedIndexSize, io.SeekStart)
	_, seekDataErr := index.dataFile.Seek(index.committedDataSize, io.SeekStart)
	var failure error
	if indexError != nil {
		index.damaged = true
		failure = fmt.Errorf("rollback index: %w", indexError)
	} else if dataError != nil {
		index.damaged = true
		failure = fmt.Errorf("rollback media: %w", dataError)
	} else if seekIndexErr != nil {
		failure = seekIndexErr
	} else {
		failure = seekDataErr
	}
	return failure
}

func joinFailure(primary, cleanup error) error {
	if cleanup == nil {
		return primary
	}
	return fmt.Errorf("%w; rollback failed: %w", primary, cleanup)
}

func writeAllAt(file io.WriterAt, data []byte, offset int64) error {
	for len(data) > 0 {
		written, err := file.WriteAt(data, offset)
		if written > 0 {
			data = data[written:]
			offset += int64(written)
		}
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
