package audit

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Box is one parsed ISO BMFF/QuickTime box header and its optional children.
type Box struct {
	Type         string `json:"type"`
	Start        int64  `json:"start"`
	HeaderSize   int    `json:"header_size"`
	Size         int64  `json:"size"`
	PayloadStart int64  `json:"payload_start"`
	Children     []*Box `json:"children,omitempty"`

	reader io.ReaderAt
}

// End returns the declared first byte after the box.
func (b *Box) End() int64 { return b.Start + b.Size }

// Payload reads the complete box payload. Callers are expected to use it only
// for small metadata boxes; media data boxes are never read by the auditor.
func (b *Box) Payload() ([]byte, error) {
	payloadSize := b.Size - int64(b.HeaderSize)
	if payloadSize < 0 {
		return nil, fmt.Errorf("box %q has negative payload size", b.Type)
	}
	if payloadSize > 16*1024*1024 {
		return nil, fmt.Errorf("box %q payload too large: %d", b.Type, payloadSize)
	}
	buf := make([]byte, payloadSize)
	if _, err := io.ReadFull(io.NewSectionReader(b.reader, b.PayloadStart, payloadSize), buf); err != nil {
		return nil, fmt.Errorf("read %q payload: %w", b.Type, err)
	}
	return buf, nil
}

var containerBoxes = map[string]bool{
	"moov": true,
	"trak": true,
	"edts": true,
	"mdia": true,
	"minf": true,
	"stbl": true,
	"dinf": true,
	"udta": true,
}

// ParseBoxes parses a box sequence occupying [start, end).
func ParseBoxes(r io.ReaderAt, start, end int64) ([]*Box, error) {
	var boxes []*Box
	pos := start

	for pos < end {
		b, next, err := readBox(r, pos, end)
		if err != nil {
			return boxes, err
		}
		if containerBoxes[b.Type] {
			childEnd := b.End()
			if childEnd > end {
				childEnd = end
			}
			children, err := ParseBoxes(r, b.PayloadStart, childEnd)
			if err != nil {
				return boxes, fmt.Errorf("box %q at %d: %w", b.Type, b.Start, err)
			}
			b.Children = children
		}
		boxes = append(boxes, b)
		pos = next
	}
	return boxes, nil
}

func readBox(r io.ReaderAt, start, parentEnd int64) (*Box, int64, error) {
	if parentEnd-start < 8 {
		return nil, start, fmt.Errorf("truncated box header at %d", start)
	}

	header := make([]byte, 16)
	if _, err := io.ReadFull(io.NewSectionReader(r, start, 16), header[:8]); err != nil {
		return nil, start, fmt.Errorf("read box header at %d: %w", start, err)
	}

	size32 := binary.BigEndian.Uint32(header[0:4])
	boxType := fourCC(header[4:8])
	headerSize := int64(8)
	declaredSize := int64(size32)

	if size32 == 1 {
		if _, err := io.ReadFull(io.NewSectionReader(r, start+8, 8), header[8:16]); err != nil {
			return nil, start, fmt.Errorf("read 64-bit size for %q at %d: %w", boxType, start, err)
		}
		declaredSize = int64(binary.BigEndian.Uint64(header[8:16]))
		headerSize = 16
	} else if size32 == 0 {
		declaredSize = parentEnd - start
	}

	if declaredSize < headerSize {
		return nil, start, fmt.Errorf("box %q at %d has size %d below header %d", boxType, start, declaredSize, headerSize)
	}
	if start+declaredSize > parentEnd {
		return nil, start, fmt.Errorf("box %q at %d ends at %d beyond parent end %d", boxType, start, start+declaredSize, parentEnd)
	}

	b := &Box{
		Type:         boxType,
		Start:        start,
		HeaderSize:   int(headerSize),
		Size:         declaredSize,
		PayloadStart: start + headerSize,
		reader:       r,
	}
	return b, start + declaredSize, nil
}

func fourCC(b []byte) string {
	out := make([]byte, 4)
	for i, c := range b {
		if c >= 0x20 && c < 0x7f {
			out[i] = c
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

// FindChild returns the first direct child of the requested type.
func FindChild(boxes []*Box, typ string) *Box {
	for _, b := range boxes {
		if b.Type == typ {
			return b
		}
	}
	return nil
}

// FindAll returns all direct children of the requested type.
func FindAll(boxes []*Box, typ string) []*Box {
	var out []*Box
	for _, b := range boxes {
		if b.Type == typ {
			out = append(out, b)
		}
	}
	return out
}

// Path returns a concise type path for diagnostics.
func (b *Box) Path(parent string) string {
	if parent == "" {
		return "/" + b.Type
	}
	return parent + "/" + b.Type
}
