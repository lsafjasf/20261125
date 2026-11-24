package mediaidx

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

type journalSnapshot struct {
	leaves       []Leaf
	mmr          mmrState
	info         CommitInfo
	committedEnd int64
	hasCommit    bool
	tornTail     bool
}

func loadJournal(file *os.File) (journalSnapshot, error) {
	info, err := file.Stat()
	if err != nil {
		return journalSnapshot{}, err
	}
	size := info.Size()
	if size < journalHeaderSize {
		return journalSnapshot{committedEnd: 0, tornTail: true}, nil
	}

	header := make([]byte, journalHeaderSize)
	if _, err := file.ReadAt(header, 0); err != nil {
		return journalSnapshot{}, fmt.Errorf("read index header: %w", err)
	}
	if err := decodeJournalHeader(header); err != nil {
		return journalSnapshot{}, err
	}

	snapshot := journalSnapshot{committedEnd: journalHeaderSize}
	allLeaves := make([]Leaf, 0)
	nextByKind := make(map[Kind]uint64, 2)
	var lastCommitID Hash
	position := int64(journalHeaderSize)

	for position < size {
		remaining := size - position
		if remaining < recordHeaderSize+recordTrailerSize {
			snapshot.tornTail = true
			break
		}

		header := make([]byte, recordHeaderSize)
		if _, err := file.ReadAt(header, position); err != nil {
			return journalSnapshot{}, fmt.Errorf("read record header: %w", err)
		}
		if !validRecordMagic(header) {
			return journalSnapshot{}, fmt.Errorf("%w at offset %d: bad record magic", errInvalidJournal, position)
		}
		payloadSize := int64(binary.LittleEndian.Uint32(header[5:9]))
		if payloadSize > maxRecordPayload {
			return journalSnapshot{}, fmt.Errorf("%w at offset %d: record too large", errInvalidJournal, position)
		}
		recordSize := int64(recordHeaderSize) + payloadSize + recordTrailerSize
		if recordSize > remaining {
			snapshot.tornTail = true
			break
		}

		record := make([]byte, recordSize)
		if _, err := file.ReadAt(record, position); err != nil && err != io.EOF {
			return journalSnapshot{}, fmt.Errorf("read record: %w", err)
		}
		payloadEnd := recordHeaderSize + int(payloadSize)
		expectedCRC := binary.LittleEndian.Uint32(record[payloadEnd:])
		actualCRC := crc32.Checksum(record[4:payloadEnd], crcTable)
		if expectedCRC != actualCRC {
			if position+recordSize == size {
				snapshot.tornTail = true
				break
			}
			return journalSnapshot{}, fmt.Errorf("%w at offset %d: record checksum mismatch", errInvalidJournal, position)
		}

		payload := record[recordHeaderSize:payloadEnd]
		body := record[:payloadEnd]
		switch record[4] {
		case recordTypeLeaf:
			leaf, err := decodeLeafPayload(payload)
			if err != nil {
				return journalSnapshot{}, fmt.Errorf("decode leaf at offset %d: %w", position, err)
			}
			if leaf.Seq != uint64(len(allLeaves)) {
				return journalSnapshot{}, fmt.Errorf("%w: leaf sequence %d, want %d", errInvalidJournal, leaf.Seq, len(allLeaves))
			}
			if err := validateLeafPlacement(allLeaves, leaf, nextByKind); err != nil {
				return journalSnapshot{}, err
			}
			allLeaves = append(allLeaves, leaf)
		case recordTypeCommit:
			commit, err := decodeCommitPayload(payload)
			if err != nil {
				return journalSnapshot{}, fmt.Errorf("decode commit at offset %d: %w", position, err)
			}
			if err := validateCommit(commit, allLeaves, snapshot, lastCommitID); err != nil {
				return journalSnapshot{}, err
			}
			state, err := mmrFromPeaks(commit.LeafCount, commit.Peaks)
			if err != nil {
				return journalSnapshot{}, fmt.Errorf("%w: %v", errInvalidJournal, err)
			}
			if state.root() != commit.Root {
				return journalSnapshot{}, fmt.Errorf("%w: commit root does not match peaks", errInvalidJournal)
			}
			lastCommitID = sha256.Sum256(body)
			snapshot.mmr = state
			snapshot.info = CommitInfo{
				Number:    commit.Number,
				LeafCount: commit.LeafCount,
				DataSize:  commit.DataSize,
				Root:      commit.Root,
				ID:        lastCommitID,
			}
			snapshot.committedEnd = position + recordSize
			snapshot.hasCommit = true
		default:
			return journalSnapshot{}, fmt.Errorf("%w: unknown record type %d", errInvalidJournal, record[4])
		}
		position += recordSize
	}

	if snapshot.hasCommit {
		snapshot.leaves = append([]Leaf(nil), allLeaves[:snapshot.info.LeafCount]...)
	} else {
		snapshot.leaves = nil
	}
	if snapshot.committedEnd < size {
		snapshot.tornTail = true
	}
	return snapshot, nil
}

func validateLeafPlacement(leaves []Leaf, next Leaf, nextByKind map[Kind]uint64) error {
	if next.DataOffset < 0 || next.DataSize <= 0 || next.end() < next.DataOffset {
		return fmt.Errorf("%w: invalid byte range", errInvalidJournal)
	}
	endUnit, ok := next.Locator.End()
	if !ok {
		return fmt.Errorf("%w: unit range overflow", errInvalidJournal)
	}
	if next.Locator.Start < nextByKind[next.Locator.Kind] {
		return fmt.Errorf("%w: overlapping or reordered %s locator", errInvalidJournal, next.Locator.Kind)
	}
	nextByKind[next.Locator.Kind] = endUnit
	if len(leaves) > 0 {
		previous := leaves[len(leaves)-1]
		if next.DataOffset != previous.end() {
			return fmt.Errorf("%w: non-contiguous data offsets", errInvalidJournal)
		}
	} else if next.DataOffset != 0 {
		return fmt.Errorf("%w: first data offset is %d", errInvalidJournal, next.DataOffset)
	}
	return nil
}

func validateCommit(commit commitPayload, leaves []Leaf, snapshot journalSnapshot, lastCommitID Hash) error {
	wantNumber := snapshot.info.Number + 1
	if commit.Number != wantNumber {
		return fmt.Errorf("%w: commit number %d, want %d", errInvalidJournal, commit.Number, wantNumber)
	}
	if commit.LeafCount != uint64(len(leaves)) {
		return fmt.Errorf("%w: commit covers %d leaves, found %d", errInvalidJournal, commit.LeafCount, len(leaves))
	}
	if commit.LeafCount == 0 {
		return fmt.Errorf("%w: empty commit", errInvalidJournal)
	}
	if commit.Previous != lastCommitID {
		return fmt.Errorf("%w: broken commit chain at commit %d", errInvalidJournal, commit.Number)
	}
	if expected := leaves[len(leaves)-1].end(); commit.DataSize != expected {
		return fmt.Errorf("%w: commit data size %d, want %d", errInvalidJournal, commit.DataSize, expected)
	}
	return nil
}
