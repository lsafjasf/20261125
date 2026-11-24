package mp4diag

import (
	"encoding/binary"
	"fmt"
	"io"
)

type byteReader struct {
	b   []byte
	pos int
}

func newByteReader(b []byte) *byteReader { return &byteReader{b: b} }

func (r *byteReader) left() int { return len(r.b) - r.pos }

func (r *byteReader) take(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.b) {
		return nil, io.ErrUnexpectedEOF
	}
	v := r.b[r.pos : r.pos+n]
	r.pos += n
	return v, nil
}

func (r *byteReader) u8() (uint8, error) {
	v, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return v[0], nil
}

func (r *byteReader) u16() (uint16, error) {
	v, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(v), nil
}

func (r *byteReader) u24() (uint32, error) {
	v, err := r.take(3)
	if err != nil {
		return 0, err
	}
	return uint32(v[0])<<16 | uint32(v[1])<<8 | uint32(v[2]), nil
}

func (r *byteReader) u32() (uint32, error) {
	v, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(v), nil
}

func (r *byteReader) u64() (uint64, error) {
	v, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(v), nil
}

func (r *byteReader) fullBox() (uint8, uint32, error) {
	version, err := r.u8()
	if err != nil {
		return 0, 0, err
	}
	flags, err := r.u24()
	return version, flags, err
}

func readPayload(r io.ReaderAt, box BoxInfo) ([]byte, error) {
	start := box.Offset + box.HeaderSize
	size := box.DeclaredEnd - start
	if size < 0 || size > maxInt64(0, box.ActualEnd-start) && box.IssueCode != "" {
		size = box.ActualEnd - start
	}
	if size < 0 {
		return nil, fmt.Errorf("payload for %s has negative size", box.Path)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(io.NewSectionReader(r, start, size), buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func boxByType(boxes []BoxInfo, typ string) *BoxInfo {
	for i := range boxes {
		if boxes[i].Type == typ && boxes[i].Valid {
			return &boxes[i]
		}
	}
	return nil
}

func boxesUnderPath(boxes []BoxInfo, prefix string) []BoxInfo {
	var out []BoxInfo
	for _, box := range boxes {
		if len(box.Path) >= len(prefix) && box.Path[:len(prefix)] == prefix {
			out = append(out, box)
		}
	}
	return out
}
