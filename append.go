package ffv1arc

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrSimulatedCrash is returned when a CrashAfter injection point is reached.
// The archive on disk intentionally looks like a real interrupted append.
var ErrSimulatedCrash = errors.New("ffv1arc: simulated crash during append")

// AppendOptions controls a weekly append.
type AppendOptions struct {
	// BlockID selects the manifest block id. Zero means auto-assignment.
	BlockID int
	// CrashAfter, when > 0, simulates a crash after this many frames have been
	// durably written. The partial block stays open and the manifest matches disk.
	CrashAfter int
}

// AppendResult summarizes an append or resume operation.
type AppendResult struct {
	BlockID         int
	Frames          int
	ReindexedFrames int
	Sealed          bool
}

// fragmentDigest computes the sha256 over the exact concatenation of frames.
func fragmentDigest(frames [][]byte) (string, int64) {
	h := sha256.New()
	var size int64
	for _, f := range frames {
		h.Write(f)
		size += int64(len(f))
	}
	return base64Std(h.Sum(nil)), size
}

func (a *Archive) openBlockLocked() *BlockState {
	for _, b := range a.man.Blocks {
		if !b.Sealed {
			return b
		}
	}
	return nil
}

// reindexBlockLocked replaces the frame index of b with the parsed segment's
// view. It returns the number of frame records written and whether the index
// actually changed. Only the supplied block is ever touched.
func (a *Archive) reindexBlockLocked(b *BlockState, ps *parsedSegment) (int, bool) {
	changed := len(b.Frames) != len(ps.frames)
	if !changed {
		for i := range ps.frames {
			if b.Frames[i] != ps.frames[i] {
				changed = true
				break
			}
		}
	}
	b.Frames = cloneFrames(ps.frames)
	b.PayloadHash = ps.payloadHash
	b.PayloadBytes = ps.payloadBytes
	return len(ps.frames), changed
}

func cloneFrames(in []FrameRec) []FrameRec {
	out := make([]FrameRec, len(in))
	copy(out, in)
	return out
}

// Append appends one weekly FFV1 fragment as a new sealed segment.
func (a *Archive) Append(chapter string, frames [][]byte, opts AppendOptions) (*AppendResult, error) {
	if len(frames) == 0 {
		return nil, ErrEmptyFragment
	}
	digest, size := fragmentDigest(frames)

	a.mu.Lock()
	defer a.mu.Unlock()

	if open := a.openBlockLocked(); open != nil {
		return nil, fmt.Errorf("%w: block %d", ErrBlockStillOpen, open.ID)
	}
	for _, b := range a.man.Blocks {
		if b.PayloadHash == digest && b.PayloadBytes == size {
			return nil, fmt.Errorf("%w: block %d holds the same fragment", ErrDuplicateAppend, b.ID)
		}
	}

	id := opts.BlockID
	if id == 0 {
		id = a.nextIDLocked()
	} else if a.findLocked(id) != nil {
		return nil, fmt.Errorf("%w: id %d", ErrBlockIDCollision, id)
	}

	name := segmentName(id, chapter)
	b := &BlockState{
		ID:          id,
		Chapter:     chapter,
		SegmentName: name,
	}
	a.man.Blocks = append(a.man.Blocks, b)

	res := &AppendResult{BlockID: id}
	path := a.segPath(name)
	if err := appendFrames(path, id, chapter, frames, b, opts.CrashAfter, a.saveLocked, &res.ReindexedFrames); err != nil {
		if errors.Is(err, ErrSimulatedCrash) {
			return res, err
		}
		return res, err
	}
	b.Sealed = true
	if err := a.saveLocked(); err != nil {
		return res, err
	}
	res.Frames = len(frames)
	res.Sealed = true
	return res, nil
}

// segmentName builds the on-disk segment file name.
func segmentName(id int, chapter string) string {
	safe := sanitize(chapter)
	return fmt.Sprintf("block-%04d-%s.mkv", id, safe)
}

func sanitize(s string) string {
	var buf bytes.Buffer
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			buf.WriteRune(r)
		default:
			buf.WriteByte('_')
		}
	}
	if buf.Len() == 0 {
		return "segment"
	}
	return buf.String()
}

// appendFrames streams frames into the segment, persisting the manifest after
// every frame so an interruption leaves manifest and disk in agreement.
func appendFrames(path string, id int, chapter string, frames [][]byte,
	b *BlockState, crashAfter int, save func() error, reindexed *int) error {

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := f.Write(segMagic); err != nil {
		return err
	}
	var idBuf [4]byte
	putU32(idBuf[:], uint32(id))
	if _, err := f.Write(idBuf[:]); err != nil {
		return err
	}
	var chLen [2]byte
	putU16(chLen[:], uint16(len(chapter)))
	if _, err := f.Write(chLen[:]); err != nil {
		return err
	}
	if _, err := f.WriteString(chapter); err != nil {
		return err
	}
	for i, payload := range frames {
		offset, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		var head [8]byte
		putU32(head[:4], uint32(i+1))
		putU32(head[4:], uint32(len(payload)))
		if _, err := f.Write(head[:]); err != nil {
			return err
		}
		if _, err := f.Write(payload); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		h.Write(payload)

		b.Frames = append(b.Frames, FrameRec{
			Number: i + 1,
			Offset: offset,
			Length: len(payload),
			Hash:   hashB64(payload),
		})
		b.PayloadHash = base64Std(h.Sum(nil))
		b.PayloadBytes += int64(len(payload))
		if err := save(); err != nil {
			return err
		}
		*reindexed++

		if crashAfter > 0 && i+1 == crashAfter {
			return ErrSimulatedCrash
		}
	}

	if _, err := f.Write(sealMagic); err != nil {
		return err
	}
	if _, err := f.Write(idBuf[:]); err != nil {
		return err
	}
	return f.Sync()
}

// Resume continues an interrupted append with the remaining frames and seals
// the block. The frame index is reconciled from disk first, so only frames of
// the affected block are recomputed.
func (a *Archive) Resume(id int, frames [][]byte) (*AppendResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	b := a.findLocked(id)
	if b == nil {
		return nil, fmt.Errorf("%w: id %d", ErrBlockNotFound, id)
	}
	if b.Sealed {
		return nil, fmt.Errorf("%w: block %d", ErrBlockSealed, id)
	}

	ps, err := parseSegment(a.segPath(b.SegmentName))
	if err != nil {
		return nil, err
	}
	n := len(ps.frames)
	for i := 0; i < n; i++ {
		if hashB64(frames[i]) != ps.frames[i].Hash {
			return nil, fmt.Errorf("%w: frame %d on disk does not match supplied fragment",
				ErrInvalidSegment, i+1)
		}
	}
	a.reindexBlockLocked(b, ps)
	if err := a.saveLocked(); err != nil {
		return nil, err
	}

	have := n
	if have > len(frames) {
		return nil, fmt.Errorf("%w: disk has %d frames, fragment only %d",
			ErrInvalidSegment, have, len(frames))
	}
	if have == len(frames) {
		// All frames durable; only the seal is missing.
		if err := sealSegment(a.segPath(b.SegmentName), id); err != nil {
			return nil, err
		}
		b.Sealed = true
		if err := a.saveLocked(); err != nil {
			return nil, err
		}
		return &AppendResult{BlockID: id, Frames: have, Sealed: true}, nil
	}

	// Rewrite the full segment atomically including the missing frames.
	full := make([][]byte, 0, len(frames))
	full = append(full, frames...)
	path := a.segPath(b.SegmentName)
	if err := writeSegment(path, id, b.Chapter, full); err != nil {
		return nil, err
	}
	ps2, err := parseSegment(path)
	if err != nil {
		return nil, err
	}
	a.reindexBlockLocked(b, ps2)
	b.Sealed = true
	if err := a.saveLocked(); err != nil {
		return nil, err
	}
	return &AppendResult{BlockID: id, Frames: len(frames), Sealed: true}, nil
}

// sealSegment appends the SEAL footer to an otherwise complete segment.
func sealSegment(path string, id int) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(sealMagic); err != nil {
		return err
	}
	var idBuf [4]byte
	putU32(idBuf[:], uint32(id))
	if _, err := f.Write(idBuf[:]); err != nil {
		return err
	}
	return f.Sync()
}

// Rename performs a legitimate archive-managed rename of a sealed segment.
// Frame indices stay valid (payload offsets are unchanged) and no alarm fires.
func (a *Archive) Rename(id int, newName string) error {
	if filepath.Base(newName) != newName || newName == "." || newName == ".." {
		return fmt.Errorf("%w: %q", ErrInvalidName, newName)
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	b := a.findLocked(id)
	if b == nil {
		return fmt.Errorf("%w: id %d", ErrBlockNotFound, id)
	}
	if !b.Sealed {
		return fmt.Errorf("%w: block %d", ErrRenameInProgress, id)
	}

	oldPath := a.segPath(b.SegmentName)
	newPath := a.segPath(newName)
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	previous := b.SegmentName
	b.SegmentName = newName
	if err := a.saveLocked(); err != nil {
		b.SegmentName = previous
		os.Rename(newPath, oldPath)
		return err
	}
	return nil
}
