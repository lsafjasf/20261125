package mediaidx

import (
	"crypto/sha256"
	"fmt"
	"io"
)

const verifyBufferSize = 128 * 1024

// Verify streams every committed leaf from the media file and reports affected
// frame or sample segments. Media bytes after the committed frontier are an
// uncommitted suffix and prevent Complete from being true.
func (index *Index) Verify() (Report, error) {
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.closed {
		return Report{}, ErrClosed
	}
	dataInfo, err := index.dataFile.Stat()
	if err != nil {
		return Report{}, fmt.Errorf("stat media for verify: %w", err)
	}
	actualSize := dataInfo.Size()
	report := Report{
		Complete:       true,
		Commit:         index.info,
		LeafCount:      uint64(len(index.leaves)),
		DataSize:       index.info.DataSize,
		CommittedData:  index.committedDataSize,
		ActualData:     actualSize,
		HasGap:         index.hasGap,
		FirstProblemAt: ^uint64(0),
	}
	buffer := make([]byte, verifyBufferSize)
	nextByKind := make(map[Kind]uint64, 2)

	for _, leaf := range index.leaves {
		expected := nextByKind[leaf.Locator.Kind]
		if leaf.Locator.Start > expected {
			missing := leaf.Locator.Start - expected
			report.Issues = append(report.Issues, Issue{
				Code:         IssueUnitGap,
				LeafSeq:      leaf.Seq,
				Locator:      Locator{Kind: leaf.Locator.Kind, Start: expected, Count: missing},
				DataOffset:   leaf.DataOffset,
				MissingUnits: missing,
			})
			report.Complete = false
		}
		if end, ok := leaf.Locator.End(); ok && end > expected {
			nextByKind[leaf.Locator.Kind] = end
		}
	}

	for _, leaf := range index.leaves {
		if leaf.end() > actualSize {
			available := actualSize - leaf.DataOffset
			if available < 0 {
				available = 0
			}
			report.Issues = append(report.Issues, Issue{
				Code:         IssueTailTruncated,
				LeafSeq:      leaf.Seq,
				Locator:      leaf.Locator,
				DataOffset:   leaf.DataOffset,
				DataSize:     leaf.DataSize,
				Available:    available,
				MissingBytes: leaf.end() - actualSize,
				Expected:     leaf.Digest,
			})
			report.Complete = false
			break
		}

		actualDigest, err := hashMediaRange(index, leaf.DataOffset, leaf.DataSize, buffer)
		if err != nil {
			report.Issues = append(report.Issues, Issue{
				Code:         IssueTailTruncated,
				LeafSeq:      leaf.Seq,
				Locator:      leaf.Locator,
				DataOffset:   leaf.DataOffset,
				DataSize:     leaf.DataSize,
				Available:    actualSize - leaf.DataOffset,
				MissingBytes: leaf.end() - actualSize,
				Expected:     leaf.Digest,
			})
			report.Complete = false
			break
		}
		report.VerifiedLeaves++
		if actualDigest != leaf.Digest {
			report.Issues = append(report.Issues, Issue{
				Code:       IssueCorruptLeaf,
				LeafSeq:    leaf.Seq,
				Locator:    leaf.Locator,
				DataOffset: leaf.DataOffset,
				DataSize:   leaf.DataSize,
				Available:  leaf.DataSize,
				Expected:   leaf.Digest,
				Actual:     actualDigest,
			})
			report.Complete = false
		}
	}

	if actualSize > index.committedDataSize {
		extra := actualSize - index.committedDataSize
		report.Issues = append(report.Issues, Issue{
			Code:       IssueUncommittedSuffix,
			LeafSeq:    report.LeafCount,
			DataOffset: index.committedDataSize,
			DataSize:   extra,
			Available:  extra,
			ExtraBytes: extra,
		})
		report.Complete = false
	}
	report.FirstProblemAt = 0
	for i, issue := range report.Issues {
		if i == 0 || issue.LeafSeq < report.FirstProblemAt {
			report.FirstProblemAt = issue.LeafSeq
		}
	}
	return report, nil
}

func hashMediaRange(index *Index, offset, size int64, buffer []byte) (Hash, error) {
	section := io.NewSectionReader(index.dataFile, offset, size)
	hasher := sha256.New()
	if _, err := io.CopyBuffer(hasher, section, buffer); err != nil {
		return Hash{}, err
	}
	var digest Hash
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

// VerifyLeaf checks one committed leaf. A nil issue means the leaf data is
// present and matches its digest. This is the O(leaf size) follow-up to
// LocateOffset/LocateRange when only a byte replacement is suspected.
func (index *Index) VerifyLeaf(seq uint64) (*Issue, error) {
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.closed {
		return nil, ErrClosed
	}
	if seq >= uint64(len(index.leaves)) {
		return nil, ErrLeafNotFound
	}
	leaf := index.leaves[seq]
	dataInfo, err := index.dataFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat media for leaf check: %w", err)
	}
	actualSize := dataInfo.Size()
	if leaf.end() > actualSize {
		available := actualSize - leaf.DataOffset
		if available < 0 {
			available = 0
		}
		return &Issue{
			Code:         IssueTailTruncated,
			LeafSeq:      leaf.Seq,
			Locator:      leaf.Locator,
			DataOffset:   leaf.DataOffset,
			DataSize:     leaf.DataSize,
			Available:    available,
			MissingBytes: leaf.end() - actualSize,
			Expected:     leaf.Digest,
		}, nil
	}
	actual, err := hashMediaRange(index, leaf.DataOffset, leaf.DataSize, make([]byte, verifyBufferSize))
	if err != nil {
		return nil, fmt.Errorf("hash media leaf %d: %w", seq, err)
	}
	if actual != leaf.Digest {
		return &Issue{
			Code:       IssueCorruptLeaf,
			LeafSeq:    leaf.Seq,
			Locator:    leaf.Locator,
			DataOffset: leaf.DataOffset,
			DataSize:   leaf.DataSize,
			Available:  leaf.DataSize,
			Expected:   leaf.Digest,
			Actual:     actual,
		}, nil
	}
	return nil, nil
}

// Stats returns a metadata-only snapshot. It never reads or rehashes media.
func (index *Index) Stats() (Stats, error) {
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.closed {
		return Stats{}, ErrClosed
	}
	dataInfo, err := index.dataFile.Stat()
	if err != nil {
		return Stats{}, fmt.Errorf("stat media: %w", err)
	}
	damaged := index.damaged || dataInfo.Size() < index.committedDataSize
	return Stats{
		Commit:      index.info,
		LeafCount:   uint64(len(index.leaves)),
		DataSize:    index.committedDataSize,
		IndexPath:   index.indexPath,
		DataPath:    index.dataPath,
		HasGap:      index.hasGap,
		Damaged:     damaged,
		Uncommitted: index.tx != nil || dataInfo.Size() > index.committedDataSize,
	}, nil
}
