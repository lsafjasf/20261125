// Package bmff implements minimal ISO-BMFF (MP4/MOV) box parsing and building.
//
// The package is intentionally dependency free and only contains the pieces
// required to inspect and rebuild sample tables (stbl children).
package bmff

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Common errors returned by the package.
var (
	ErrShortHeader = errors.New("bmff: truncated box header")
	ErrBadSize     = errors.New("bmff: invalid box size")
)

// Box is a located box header inside a seekable file.
type Box struct {
	Type       string
	Start      int64 // absolute offset of the size field
	Size       int64 // total box size including header; -1 means "to EOF"
	HeaderSize int   // 8 or 16
	FileSize   int64 // size of the enclosing file (used for size==0 boxes)
}

// PayloadStart returns the absolute offset of the first payload byte.
func (b Box) PayloadStart() int64 { return b.Start + int64(b.HeaderSize) }

// End returns the absolute offset immediately after the box.
func (b Box) End() int64 {
	if b.Size < 0 {
		return b.FileSize
	}
	return b.Start + b.Size
}

// PayloadLength returns the payload length in bytes.
func (b Box) PayloadLength() int64 { return b.End() - b.PayloadStart() }

// ReadBoxHeader reads the box header at absolute offset off.
// fileSize is used to resolve size==0 ("box extends to EOF").
func ReadBoxHeader(ra io.ReaderAt, off, fileSize int64) (Box, error) {
	var raw [16]byte
	if _, err := ra.ReadAt(raw[:8], off); err != nil {
		return Box{}, fmt.Errorf("%w at %d: %v", ErrShortHeader, off, err)
	}
	size32 := binary.BigEndian.Uint32(raw[0:4])
	b := Box{Type: string(raw[4:8]), Start: off, HeaderSize: 8, FileSize: fileSize}
	switch size32 {
	case 0:
		b.Size = -1
		if fileSize > 0 && off > fileSize {
			return b, fmt.Errorf("%w: box starts past EOF", ErrBadSize)
		}
	case 1:
		if _, err := ra.ReadAt(raw[8:16], off+8); err != nil {
			return b, fmt.Errorf("%w: largesize at %d: %v", ErrShortHeader, off, err)
		}
		b.Size = int64(binary.BigEndian.Uint64(raw[8:16]))
		b.HeaderSize = 16
		if b.Size < 16 {
			return b, fmt.Errorf("%w: largesize %d", ErrBadSize, b.Size)
		}
	default:
		b.Size = int64(size32)
		if b.Size < 8 {
			return b, fmt.Errorf("%w: size %d", ErrBadSize, b.Size)
		}
	}
	return b, nil
}

// Validate reports whether the box fits inside its enclosing range.
func (b Box) Validate(rangeEnd int64) error {
	end := b.End()
	if end > rangeEnd {
		return fmt.Errorf("%w: box %q at %d ends at %d beyond %d",
			ErrBadSize, b.Type, b.Start, end, rangeEnd)
	}
	return nil
}

// IterateChildren invokes fn for every direct child box of parent.
// Children are read through ra; fn may return a sentinel error to stop.
func IterateChildren(ra io.ReaderAt, parent Box, fn func(child Box) error) error {
	start := parent.PayloadStart()
	end := parent.End()
	return IterateRange(ra, start, end, fn)
}

// IterateRange walks top level boxes in the absolute range [start,end).
func IterateRange(ra io.ReaderAt, start, end int64, fn func(child Box) error) error {
	pos := start
	for pos+8 <= end {
		b, err := ReadBoxHeader(ra, pos, end)
		if err != nil {
			return err
		}
		realEnd := b.End()
		if realEnd > end {
			// Truncated child: surface it but stop iteration.
			return fmt.Errorf("%w: box %q at %d needs %d bytes, range ends %d",
				ErrBadSize, b.Type, pos, b.Size, end)
		}
		if err := fn(b); err != nil {
			return err
		}
		if b.Size < 0 {
			return nil
		}
		pos = realEnd
	}
	return nil
}

// FindChild returns the first direct child with the given fourcc type.
func FindChild(ra io.ReaderAt, parent Box, want string) (Box, bool, error) {
	var found Box
	ok := false
	err := IterateChildren(ra, parent, func(c Box) error {
		if c.Type == want {
			found = c
			ok = true
			return errStop
		}
		return nil
	})
	if errors.Is(err, errStop) {
		err = nil
	}
	return found, ok, err
}

var errStop = errors.New("stop")

// ReadBoxBytes reads the full box bytes (header + payload).
func ReadBoxBytes(ra io.ReaderAt, b Box) ([]byte, error) {
	n := b.Size
	if n < 0 {
		n = b.FileSize - b.Start
	}
	buf := make([]byte, n)
	if _, err := ra.ReadAt(buf, b.Start); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf, nil
}

// ReadPayload reads exactly the payload of b into memory.
func ReadPayload(ra io.ReaderAt, b Box) ([]byte, error) {
	n := b.PayloadLength()
	buf := make([]byte, n)
	if _, err := ra.ReadAt(buf, b.PayloadStart()); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf, nil
}

// U32 reads a big endian uint32 from buf at offset.
func U32(buf []byte, off int) uint32 { return binary.BigEndian.Uint32(buf[off : off+4]) }

// U16 reads a big endian uint16 from buf at offset.
func U16(buf []byte, off int) uint16 { return binary.BigEndian.Uint16(buf[off : off+2]) }

// PutU32 appends a big endian uint32.
func PutU32(buf []byte, v uint32) { binary.BigEndian.PutUint32(buf, v) }

// BuildBox serializes a normal 32 bit size box.
func BuildBox(boxType string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], boxType)
	copy(out[8:], payload)
	return out
}

// BuildFullBox serializes a ISO full box (version + 3 flag bytes).
func BuildFullBox(boxType string, version byte, flags uint32, payload []byte) []byte {
	full := make([]byte, 4+len(payload))
	full[0] = version
	full[1] = byte(flags >> 16)
	full[2] = byte(flags >> 8)
	full[3] = byte(flags)
	copy(full[4:], payload)
	return BuildBox(boxType, full)
}
