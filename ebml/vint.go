package ebml

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"
)

var (
	ErrInvalidVINT = errors.New("invalid EBML VINT")
	ErrTruncated   = errors.New("truncated EBML element")
)

type Header struct {
	ID          uint32
	IDSize      int
	Offset      int64
	SizeVINT    []byte
	Size        uint64
	SizeSize    int
	HeaderSize  int
	DataOffset  int64
	UnknownSize bool
}

func (h Header) ElementEnd(fileSize int64) int64 {
	if h.UnknownSize {
		return fileSize
	}
	end := h.DataOffset + int64(h.Size)
	if end > fileSize {
		return fileSize
	}
	return end
}

func IDWidth(first byte) int {
	if first == 0 {
		return 0
	}
	return bits.LeadingZeros8(first) + 1
}

func ReadVINT(buf []byte) (uint64, int, bool, error) {
	if len(buf) == 0 {
		return 0, 0, false, io.ErrUnexpectedEOF
	}
	width := IDWidth(buf[0])
	if width == 0 || width > 8 || len(buf) < width {
		return 0, 0, false, ErrInvalidVINT
	}
	marker := byte(1) << uint(8-width)
	if buf[0]&marker == 0 {
		return 0, 0, false, fmt.Errorf("%w: missing length marker", ErrInvalidVINT)
	}
	value := uint64(buf[0] & (marker - 1))
	allOnes := value == uint64(marker-1)
	for i := 1; i < width; i++ {
		value = value<<8 | uint64(buf[i])
		if buf[i] != 0xff {
			allOnes = false
		}
	}
	if buf[0] != 0xff {
		rest := buf[0] & (marker - 1)
		if rest != marker-1 {
			allOnes = false
		}
	}
	return value, width, allOnes, nil
}

func ReadHeaderAt(r io.ReaderAt, offset int64) (Header, error) {
	var first [8]byte
	if _, err := r.ReadAt(first[:1], offset); err != nil {
		return Header{}, fmt.Errorf("read element id: %w", err)
	}
	idWidth := IDWidth(first[0])
	if idWidth == 0 || idWidth > 4 {
		return Header{}, fmt.Errorf("%w: element id width %d", ErrInvalidVINT, idWidth)
	}
	if _, err := r.ReadAt(first[:idWidth], offset); err != nil {
		return Header{}, fmt.Errorf("read element id: %w", err)
	}
	id := uint32(0)
	for i := 0; i < idWidth; i++ {
		id = id<<8 | uint32(first[i])
	}

	sizeOffset := offset + int64(idWidth)
	if _, err := r.ReadAt(first[:1], sizeOffset); err != nil {
		return Header{}, fmt.Errorf("read size vint: %w", err)
	}
	sizeWidth := IDWidth(first[0])
	if sizeWidth == 0 || sizeWidth > 8 {
		return Header{}, fmt.Errorf("%w: size vint width %d", ErrInvalidVINT, sizeWidth)
	}
	var sizeBuf [8]byte
	if _, err := r.ReadAt(sizeBuf[:sizeWidth], sizeOffset); err != nil {
		return Header{}, fmt.Errorf("read size vint: %w", err)
	}
	size, _, unknown, err := ReadVINT(sizeBuf[:sizeWidth])
	if err != nil {
		return Header{}, err
	}
	headerSize := idWidth + sizeWidth
	return Header{
		ID:          id,
		IDSize:      idWidth,
		Offset:      offset,
		Size:        size,
		SizeSize:    sizeWidth,
		HeaderSize:  headerSize,
		DataOffset:  offset + int64(headerSize),
		UnknownSize: unknown,
	}, nil
}

func ReadUint(r io.ReaderAt, offset int64, size int) (uint64, error) {
	if size <= 0 || size > 8 {
		return 0, fmt.Errorf("invalid uint size %d", size)
	}
	var buf [8]byte
	start := 8 - size
	n, err := r.ReadAt(buf[start:], offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if n != size {
		return 0, io.ErrUnexpectedEOF
	}
	return binary.BigEndian.Uint64(buf[:]), nil
}

func ReadBytes(r io.ReaderAt, offset int64, size int64) ([]byte, error) {
	if size < 0 || size > 64<<20 {
		return nil, fmt.Errorf("refusing to read %d bytes", size)
	}
	buf := make([]byte, size)
	n, err := r.ReadAt(buf, offset)
	if errors.Is(err, io.EOF) && int64(n) == size {
		return buf[:n], nil
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if int64(n) != size {
		return buf[:n], io.ErrUnexpectedEOF
	}
	return buf, nil
}
