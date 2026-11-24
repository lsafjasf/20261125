package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

type byteDifference struct {
	Relative       int64
	OriginalOffset int64
	CopyOffset     int64
	OriginalLength int64
	CopyLength     int64
	LengthMismatch bool
}

func hashRange(r io.ReaderAt, start, end int64) (string, error) {
	if start < 0 || end < start {
		return "", fmt.Errorf("invalid hash range [%d,%d)", start, end)
	}
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(r, start, end-start)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func buildRawBlocks(r io.ReaderAt, size int64, blockSize int) (string, []RangeHash, error) {
	whole := sha256.New()
	blocks := make([]RangeHash, 0, int((size+int64(blockSize)-1)/int64(blockSize)))
	buf := make([]byte, blockSize)

	for start := int64(0); start < size; start += int64(blockSize) {
		want := int64(blockSize)
		if rest := size - start; rest < want {
			want = rest
		}
		n, err := io.ReadFull(io.NewSectionReader(r, start, want), buf[:want])
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return "", nil, fmt.Errorf("read block at %d: %w", start, err)
		}
		if n == 0 {
			break
		}
		part := buf[:n]
		whole.Write(part)
		blocks = append(blocks, RangeHash{
			Layer:  LayerRawBlock,
			Index:  len(blocks),
			Start:  start,
			End:    start + int64(n),
			Size:   int64(n),
			SHA256: hashBytes(part),
		})
		if int64(n) < want {
			break
		}
	}
	return hex.EncodeToString(whole.Sum(nil)), blocks, nil
}

// firstRangeDifference finds the first differing byte in two corresponding
// ranges. Unequal lengths are reported at the end of the shorter range.
func firstRangeDifference(a io.ReaderAt, aStart, aEnd int64, b io.ReaderAt, bStart, bEnd int64) (*byteDifference, error) {
	aLen := aEnd - aStart
	bLen := bEnd - bStart
	limit := aLen
	if bLen < limit {
		limit = bLen
	}

	const scanSize = 64 << 10
	abuf := make([]byte, scanSize)
	bbuf := make([]byte, scanSize)
	var rel int64

	for rel < limit {
		n := int64(scanSize)
		if rest := limit - rel; rest < n {
			n = rest
		}
		an, err := a.ReadAt(abuf[:n], aStart+rel)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		bn, berr := b.ReadAt(bbuf[:n], bStart+rel)
		if berr != nil && !errors.Is(berr, io.EOF) && !errors.Is(berr, io.ErrUnexpectedEOF) {
			return nil, berr
		}
		m := an
		if bn < m {
			m = bn
		}
		for i := 0; i < m; i++ {
			if abuf[i] != bbuf[i] {
				pos := rel + int64(i)
				return &byteDifference{
					Relative:       pos,
					OriginalOffset: aStart + pos,
					CopyOffset:     bStart + pos,
					OriginalLength: aLen,
					CopyLength:     bLen,
				}, nil
			}
		}
		if int64(m) < n {
			pos := rel + int64(m)
			return &byteDifference{
				Relative:       pos,
				OriginalOffset: aStart + pos,
				CopyOffset:     bStart + pos,
				OriginalLength: aLen,
				CopyLength:     bLen,
				LengthMismatch: true,
			}, nil
		}
		rel += n
	}

	if aLen != bLen {
		return &byteDifference{
			Relative:       limit,
			OriginalOffset: aStart + limit,
			CopyOffset:     bStart + limit,
			OriginalLength: aLen,
			CopyLength:     bLen,
			LengthMismatch: true,
		}, nil
	}
	return nil, nil
}

func firstFileDifference(a, b io.ReaderAt, sizeA, sizeB int64) (*byteDifference, error) {
	return firstRangeDifference(a, 0, sizeA, b, 0, sizeB)
}
