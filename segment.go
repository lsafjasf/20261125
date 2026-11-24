package ffv1arc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
)

// On-disk segment layout (all integers big-endian):
//
//	"FFV1ARC1"                     magic, 8 bytes
//	blockID              uint32
//	chapterLen           uint16
//	chapter              chapterLen bytes
//	frameNo  uint32 | frameLen uint32 | frameLen bytes   repeated per frame
//	"SEAL" | blockID uint32        footer, present only once a block is sealed
var (
	segMagic  = []byte("FFV1ARC1")
	sealMagic = []byte("SEAL")
)

// parsedSegment is the decoded view of a segment file.
type parsedSegment struct {
	blockID      int
	chapter      string
	frames       []FrameRec
	sealed       bool
	payloadBytes int64
	payloadHash  string
}

// writeSegment writes a complete, sealed segment for block b containing frames.
func writeSegment(path string, blockID int, chapter string, frames [][]byte) error {
	var buf bytes.Buffer
	buf.Write(segMagic)
	var idBytes [4]byte
	putU32(idBytes[:], uint32(blockID))
	buf.Write(idBytes[:])
	if len(chapter) > 0xffff {
		return fmt.Errorf("%w: chapter too long", ErrInvalidSegment)
	}
	var chLen [2]byte
	putU16(chLen[:], uint16(len(chapter)))
	buf.Write(chLen[:])
	buf.WriteString(chapter)

	for i, payload := range frames {
		if len(payload) > 0xffffffff {
			return fmt.Errorf("%w: frame too large", ErrInvalidSegment)
		}
		var head [8]byte
		putU32(head[:4], uint32(i+1))
		putU32(head[4:], uint32(len(payload)))
		buf.Write(head[:])
		buf.Write(payload)
	}

	buf.Write(sealMagic)
	buf.Write(idBytes[:])

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// parseSegment decodes a segment file. An interrupted (truncated, footerless)
// segment is returned with sealed=false and no error; structural corruption
// produces ErrInvalidSegment.
func parseSegment(path string) (*parsedSegment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < len(segMagic) || !bytes.Equal(data[:len(segMagic)], segMagic) {
		return nil, fmt.Errorf("%w: bad magic", ErrInvalidSegment)
	}
	r := bytes.NewReader(data[len(segMagic):])

	var idBuf [4]byte
	if _, err := io.ReadFull(r, idBuf[:]); err != nil {
		return nil, fmt.Errorf("%w: truncated header", ErrInvalidSegment)
	}
	blockID := int(binary.BigEndian.Uint32(idBuf[:]))

	var chLenBuf [2]byte
	if _, err := io.ReadFull(r, chLenBuf[:]); err != nil {
		return nil, fmt.Errorf("%w: truncated chapter length", ErrInvalidSegment)
	}
	chapter := make([]byte, binary.BigEndian.Uint16(chLenBuf[:]))
	if _, err := io.ReadFull(r, chapter); err != nil {
		return nil, fmt.Errorf("%w: truncated chapter", ErrInvalidSegment)
	}

	ps := &parsedSegment{blockID: blockID, chapter: string(chapter)}
	ph := sha256.New()

	// Frames run until either the SEAL footer appears or the file ends.
	frameBuf := make([]byte, 0, 4096)
	for {
		var head [8]byte
		if _, err := io.ReadFull(r, head[:]); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("%w: truncated frame header", ErrInvalidSegment)
			}
			return nil, err
		}
		frameNo := binary.BigEndian.Uint32(head[:4])
		frameLen := binary.BigEndian.Uint32(head[4:])
		if bytes.Equal(head[:4], sealMagic) && r.Len() == 0 && int(frameLen) == blockID {
			ps.sealed = true
			break
		}
		if int(frameNo) != len(ps.frames)+1 {
			return nil, fmt.Errorf("%w: frame numbering gap", ErrInvalidSegment)
		}
		if cap(frameBuf) < int(frameLen) {
			frameBuf = make([]byte, frameLen)
		} else {
			frameBuf = frameBuf[:frameLen]
		}
		offset, _ := r.Seek(0, io.SeekCurrent)
		if _, err := io.ReadFull(r, frameBuf); err != nil {
			// Truncated frame payload: interrupted append, stop here.
			break
		}
		payload := make([]byte, frameLen)
		copy(payload, frameBuf)
		ph.Write(payload)
		ps.frames = append(ps.frames, FrameRec{
			Number: int(frameNo),
			Offset: offset,
			Length: int(frameLen),
			Hash:   hashB64(payload),
		})
		ps.payloadBytes += int64(frameLen)
	}

	ps.payloadHash = base64Hash(ph)
	return ps, nil
}

func base64Hash(h hash.Hash) string {
	return base64Std(h.Sum(nil))
}
