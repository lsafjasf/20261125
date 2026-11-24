package repack

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// MCPACK is a small self-describing media container used by the repackager.
// All multi-byte integers are big-endian.
//
//	File   := header box*
//	header := magic[6] version u8 flags u16 numBoxes u32 headerCRC u32
//	box    := type[4] language[16] startUs i64 endUs i64
//	          width u32 height u32 payloadLen u32 payload[payloadLen] boxCRC u32
//
// headerCRC covers every preceding header byte; boxCRC covers every
// preceding box byte (header fields and payload).
const (
	magic       = "MCPACK"
	version1    = 1
	headerSize  = 6 + 1 + 2 + 4 + 4
	langField   = 16
	boxHdrSize  = 4 + langField + 8 + 8 + 4 + 4 + 4
	crcSize     = 4
	maxPayload  = 256 * 1024 * 1024
	maxBoxCount = 1 << 20
)

type BoxType [4]byte

var (
	TypeVideo    = BoxType{'V', 'I', 'D', 'E'}
	TypeAudio    = BoxType{'A', 'U', 'D', 'I'}
	TypeSubtitle = BoxType{'S', 'U', 'B', 'T'}
	TypeChapter  = BoxType{'C', 'H', 'A', 'P'}
)

func (t BoxType) String() string { return string(t[:]) }

// Box is one media track fragment or chapter marker inside a pack.
type Box struct {
	Type     BoxType
	Language string
	StartUs  int64
	EndUs    int64
	Width    uint32
	Height   uint32
	Payload  []byte
}

var crcTable = crc32.MakeTable(crc32.Castagnoli)

// ReadPack opens path read-only and verifies every checksum. The caller can
// rely on the returned boxes being byte-exact representations of the file.
func ReadPack(path string) ([]Box, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open input %q: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read input %q: %w", path, err)
	}
	if len(data) < headerSize {
		return nil, fmt.Errorf("input %q: truncated header", path)
	}
	if string(data[:6]) != magic {
		return nil, fmt.Errorf("input %q: bad magic, not an MCPACK file", path)
	}
	if data[6] != version1 {
		return nil, fmt.Errorf("input %q: unsupported version %d", path, data[6])
	}
	if crc32.Checksum(data[:headerSize-crcSize], crcTable) !=
		binary.BigEndian.Uint32(data[headerSize-crcSize:]) {
		return nil, fmt.Errorf("input %q: header checksum mismatch", path)
	}
	numBoxes := binary.BigEndian.Uint32(data[9:13])
	if numBoxes > maxBoxCount {
		return nil, fmt.Errorf("input %q: implausible box count %d", path, numBoxes)
	}

	boxes := make([]Box, 0, numBoxes)
	off := headerSize
	for i := uint32(0); i < numBoxes; i++ {
		if len(data)-off < boxHdrSize {
			return nil, fmt.Errorf("input %q: truncated box header %d", path, i)
		}
		boxStart := off
		var b Box
		copy(b.Type[:], data[off:off+4])
		off += 4
		b.Language = cstring(data[off : off+langField])
		off += langField
		b.StartUs = int64(binary.BigEndian.Uint64(data[off:]))
		off += 8
		b.EndUs = int64(binary.BigEndian.Uint64(data[off:]))
		off += 8
		b.Width = binary.BigEndian.Uint32(data[off:])
		off += 4
		b.Height = binary.BigEndian.Uint32(data[off:])
		off += 4
		payloadLen := binary.BigEndian.Uint32(data[off:])
		off += 4
		if payloadLen > maxPayload {
			return nil, fmt.Errorf("input %q: box %d payload too large", path, i)
		}
		if len(data)-off < int(payloadLen)+crcSize {
			return nil, fmt.Errorf("input %q: truncated box payload %d", path, i)
		}
		b.Payload = append([]byte(nil), data[off:off+int(payloadLen)]...)
		off += int(payloadLen)
		wantCRC := binary.BigEndian.Uint32(data[off:])
		if crc32.Checksum(data[boxStart:off], crcTable) != wantCRC {
			return nil, fmt.Errorf("input %q: box %d checksum mismatch", path, i)
		}
		off += crcSize
		boxes = append(boxes, b)
	}
	if off != len(data) {
		return nil, fmt.Errorf("input %q: %d trailing bytes", path, len(data)-off)
	}
	return boxes, nil
}

// EncodePack serializes boxes into a self-contained, checksummed byte slice.
func EncodePack(boxes []Box) ([]byte, error) {
	var buf bytes.Buffer
	hdr := make([]byte, headerSize)
	copy(hdr[:6], magic)
	hdr[6] = version1
	binary.BigEndian.PutUint32(hdr[9:13], uint32(len(boxes)))
	binary.BigEndian.PutUint32(hdr[13:], crc32.Checksum(hdr[:13], crcTable))
	buf.Write(hdr)

	for i, b := range boxes {
		if len(b.Payload) > maxPayload {
			return nil, fmt.Errorf("box %d: payload too large", i)
		}
		raw := make([]byte, boxHdrSize+len(b.Payload)+crcSize)
		copy(raw[0:4], b.Type[:])
		lang := make([]byte, langField)
		copy(lang, b.Language)
		copy(raw[4:4+langField], lang)
		binary.BigEndian.PutUint64(raw[20:28], uint64(b.StartUs))
		binary.BigEndian.PutUint64(raw[28:36], uint64(b.EndUs))
		binary.BigEndian.PutUint32(raw[36:40], b.Width)
		binary.BigEndian.PutUint32(raw[40:44], b.Height)
		binary.BigEndian.PutUint32(raw[44:48], uint32(len(b.Payload)))
		copy(raw[48:48+len(b.Payload)], b.Payload)
		binary.BigEndian.PutUint32(raw[len(raw)-4:],
			crc32.Checksum(raw[:len(raw)-4], crcTable))
		buf.Write(raw)
	}
	return buf.Bytes(), nil
}

func cstring(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// ErrCorrupt is returned when a post-write verification detects tampering.
var ErrCorrupt = errors.New("output verification failed")
