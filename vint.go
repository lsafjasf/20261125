package matroska

import (
	"fmt"
	"math"
)

type codecError struct {
	kind   ErrorKind
	detail string
}

func (e *codecError) Error() string { return e.detail }
func (e *codecError) Unwrap() error { return e.kind }

func codecErr(kind ErrorKind, format string, args ...any) error {
	return &codecError{kind: kind, detail: fmt.Sprintf(format, args...)}
}

func vintWidth(first byte) (int, error) {
	if first == 0 {
		return 0, codecErr(ErrInvalidVINT, "zero VINT prefix")
	}
	width := 1
	for first&0x80 == 0 {
		first <<= 1
		width++
	}
	if width > 8 {
		return 0, codecErr(ErrInvalidVINT, "VINT is longer than 8 bytes")
	}
	return width, nil
}

// readVINT decodes an EBML variable-size integer and removes the marker bits.
func readVINT(data []byte, pos int) (value uint64, width int, err error) {
	if pos < 0 || pos >= len(data) {
		return 0, 0, codecErr(ErrTruncated, "missing VINT")
	}
	width, err = vintWidth(data[pos])
	if err != nil {
		return 0, 0, err
	}
	if pos+width > len(data) {
		return 0, 0, codecErr(ErrTruncated, "VINT needs %d bytes, have %d", width, len(data)-pos)
	}
	mask := byte((1 << (8 - width)) - 1)
	value = uint64(data[pos] & mask)
	for i := 1; i < width; i++ {
		value = value<<8 | uint64(data[pos+i])
	}
	return value, width, nil
}

func readElementID(data []byte, pos int) (id uint64, width int, err error) {
	if pos < 0 || pos >= len(data) {
		return 0, 0, codecErr(ErrTruncated, "missing element ID")
	}
	width, err = vintWidth(data[pos])
	if err != nil {
		return 0, 0, err
	}
	if width > 4 {
		return 0, 0, codecErr(ErrInvalidVINT, "element ID is longer than 4 bytes")
	}
	if pos+width > len(data) {
		return 0, 0, codecErr(ErrTruncated, "element ID needs %d bytes, have %d", width, len(data)-pos)
	}
	for i := 0; i < width; i++ {
		id = id<<8 | uint64(data[pos+i])
	}
	return id, width, nil
}

func vintUnknown(value uint64, width int) bool {
	return value == (uint64(1)<<(7*width))-1
}

// readSignedLacingVINT decodes the biased signed integer used by EBML lacing.
func readSignedLacingVINT(data []byte, pos int) (int64, int, error) {
	raw, width, err := readVINT(data, pos)
	if err != nil {
		return 0, 0, err
	}
	bias := (int64(1) << (7*width - 1)) - 1
	return int64(raw) - bias, width, nil
}

type elementHeader struct {
	id         uint64
	size       int64
	unknown    bool
	headerSize int
	dataStart  int
}

func readElementHeader(data []byte, pos int) (elementHeader, error) {
	id, idWidth, err := readElementID(data, pos)
	if err != nil {
		return elementHeader{}, err
	}
	sizeValue, sizeWidth, err := readVINT(data, pos+idWidth)
	if err != nil {
		return elementHeader{}, err
	}
	size := int64(sizeValue)
	unknown := vintUnknown(sizeValue, sizeWidth)
	if sizeValue > math.MaxInt64 {
		return elementHeader{}, codecErr(ErrInvalidVINT, "element size does not fit int64")
	}
	return elementHeader{
		id:         id,
		size:       size,
		unknown:    unknown,
		headerSize: idWidth + sizeWidth,
		dataStart:  pos + idWidth + sizeWidth,
	}, nil
}

func readUint(data []byte) uint64 {
	var value uint64
	for _, b := range data {
		value = value<<8 | uint64(b)
	}
	return value
}
