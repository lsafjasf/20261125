package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Box is a parsed ISO-BMFF box header plus its payload.
type Box struct {
	Type   string
	Size   uint64 // total size including header
	Offset int64  // offset of the box header in the file
	Data   []byte // payload (header excluded)
}

// containerTypes are boxes whose payload is a sequence of child boxes.
var containerTypes = map[string]bool{
	"moov": true, "trak": true, "mdia": true, "minf": true,
	"stbl": true, "edts": true, "dinf": true, "udta": true,
	"moof": true, "traf": true, "mvex": true,
}

// ReadBox reads one box at the current position of r.
// end is the absolute file offset limiting this box's extent.
func ReadBox(r io.ReadSeeker, end int64) (*Box, error) {
	start, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	if start+8 > end {
		return nil, io.EOF
	}
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	size := uint64(binary.BigEndian.Uint32(hdr[0:4]))
	typ := string(hdr[4:8])
	headerSize := uint64(8)
	if size == 1 {
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return nil, err
		}
		size = binary.BigEndian.Uint64(ext[:])
		headerSize = 16
	} else if size == 0 {
		size = uint64(end - start)
	}
	if size < headerSize || start+int64(size) > end {
		return nil, fmt.Errorf("invalid box %q size %d at offset %d", typ, size, start)
	}
	payload := make([]byte, size-headerSize)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &Box{Type: typ, Size: size, Offset: start, Data: payload}, nil
}

// Children parses the payload of a container box into child boxes.
func (b *Box) Children() ([]*Box, error) {
	var out []*Box
	data := b.Data
	for len(data) >= 8 {
		size := uint64(binary.BigEndian.Uint32(data[0:4]))
		headerSize := uint64(8)
		if size == 1 {
			if len(data) < 16 {
				return nil, fmt.Errorf("truncated extended box header in %q", b.Type)
			}
			size = binary.BigEndian.Uint64(data[8:16])
			headerSize = 16
		} else if size == 0 {
			size = uint64(len(data))
		}
		if size < headerSize || size > uint64(len(data)) {
			return nil, fmt.Errorf("invalid child box %q size %d in %q",
				string(data[4:8]), size, b.Type)
		}
		out = append(out, &Box{
			Type: string(data[4:8]),
			Size: size,
			Data: data[headerSize:size],
		})
		data = data[size:]
	}
	return out, nil
}

// FindChildren returns all direct children with the given type.
func FindChildren(b *Box, typ string) []*Box {
	kids, err := b.Children()
	if err != nil {
		return nil
	}
	var out []*Box
	for _, k := range kids {
		if k.Type == typ {
			out = append(out, k)
		}
	}
	return out
}

// FindChild returns the first direct child with the given type, or nil.
func FindChild(b *Box, typ string) *Box {
	kids := FindChildren(b, typ)
	if len(kids) == 0 {
		return nil
	}
	return kids[0]
}
