package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const maxBoxDepth = 16

type box struct {
	typ        string
	start      int64
	payloadPos int64
	end        int64
}

func (b box) payloadSize() int64 {
	return b.end - b.payloadPos
}

func readBoxes(r io.ReaderAt, start, end int64, depth int, visit func(box) error) error {
	if depth > maxBoxDepth {
		return fmt.Errorf("box nesting exceeds %d", maxBoxDepth)
	}
	for pos := start; pos < end; {
		b, next, err := readBox(r, pos, end)
		if err != nil {
			return fmt.Errorf("box at offset %d: %w", pos, err)
		}
		if err := visit(b); err != nil {
			return err
		}
		pos = next
	}
	return nil
}

func readBox(r io.ReaderAt, pos, parentEnd int64) (box, int64, error) {
	var hdr [8]byte
	if parentEnd-pos < int64(len(hdr)) {
		return box{}, 0, io.ErrUnexpectedEOF
	}
	if _, err := r.ReadAt(hdr[:], pos); err != nil {
		return box{}, 0, err
	}

	size := uint64(binary.BigEndian.Uint32(hdr[0:4]))
	typ := string(hdr[4:8])
	headerSize := int64(8)

	switch size {
	case 0:
		size = uint64(parentEnd - pos)
	case 1:
		var large [8]byte
		if parentEnd-pos < 16 {
			return box{}, 0, io.ErrUnexpectedEOF
		}
		if _, err := r.ReadAt(large[:], pos+8); err != nil {
			return box{}, 0, err
		}
		size = binary.BigEndian.Uint64(large[:])
		headerSize = 16
	}
	if size < uint64(headerSize) {
		return box{}, 0, fmt.Errorf("invalid size %d for %q", size, typ)
	}
	if size > uint64(parentEnd-pos) {
		return box{}, 0, fmt.Errorf("size %d overruns parent for %q", size, typ)
	}
	boxEnd := pos + int64(size)
	return box{
		typ:        typ,
		start:      pos,
		payloadPos: pos + headerSize,
		end:        boxEnd,
	}, boxEnd, nil
}

func readPayload(r io.ReaderAt, b box, max int64) ([]byte, error) {
	size := b.payloadSize()
	if size < 0 || size > max {
		return nil, fmt.Errorf("%q payload is %d bytes (limit %d)", b.typ, size, max)
	}
	data := make([]byte, size)
	if size == 0 {
		return data, nil
	}
	if _, err := r.ReadAt(data, b.payloadPos); err != nil {
		return nil, err
	}
	return data, nil
}

func readPayloadPrefix(r io.ReaderAt, b box, limit int64) ([]byte, error) {
	size := b.payloadSize()
	if size < 0 {
		return nil, fmt.Errorf("%q has a negative payload size", b.typ)
	}
	if size > limit {
		size = limit
	}
	data := make([]byte, size)
	if size == 0 {
		return data, nil
	}
	if _, err := r.ReadAt(data, b.payloadPos); err != nil {
		return nil, err
	}
	return data, nil
}

type payloadReader struct {
	data []byte
	pos  int
}

func newPayloadReader(data []byte) *payloadReader {
	return &payloadReader{data: data}
}

func (p *payloadReader) remaining() int {
	return len(p.data) - p.pos
}

func (p *payloadReader) take(n int) ([]byte, error) {
	if n < 0 || p.remaining() < n {
		return nil, io.ErrUnexpectedEOF
	}
	v := p.data[p.pos : p.pos+n]
	p.pos += n
	return v, nil
}

func (p *payloadReader) uint8() (uint8, error) {
	v, err := p.take(1)
	if err != nil {
		return 0, err
	}
	return v[0], nil
}

func (p *payloadReader) uint16() (uint16, error) {
	v, err := p.take(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(v), nil
}

func (p *payloadReader) uint24() (uint32, error) {
	v, err := p.take(3)
	if err != nil {
		return 0, err
	}
	return uint32(v[0])<<16 | uint32(v[1])<<8 | uint32(v[2]), nil
}

func (p *payloadReader) uint32() (uint32, error) {
	v, err := p.take(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(v), nil
}

func (p *payloadReader) uint64() (uint64, error) {
	v, err := p.take(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(v), nil
}

func (p *payloadReader) skip(n int) error {
	_, err := p.take(n)
	return err
}

func (p *payloadReader) fourCC() (string, error) {
	v, err := p.take(4)
	if err != nil {
		return "", err
	}
	return string(v), nil
}

func fullBox(r *payloadReader) (version uint8, flags uint32, err error) {
	v, err := r.uint8()
	if err != nil {
		return 0, 0, err
	}
	f, err := r.uint24()
	return v, f, err
}

func checkedAdd(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, fmt.Errorf("timestamp overflow: %d + %d", a, b)
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, fmt.Errorf("timestamp underflow: %d + %d", a, b)
	}
	return a + b, nil
}
