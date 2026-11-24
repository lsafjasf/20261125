// Package mkv parses the Matroska subset needed to inspect an FFV1 stream:
// tracks, CodecPrivate, clusters, block timestamps, frame payloads and
// container CRC-32 elements.
package mkv

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"ffv1compare/internal/ebml"
)

type Issue struct {
	Offset  int64
	Code    string
	Message string
}

type Track struct {
	Number            uint64
	UID               uint64
	Type              uint64
	CodecID           string
	CodecPrivate      []byte
	CodecPrivateOff   int64
	Width             uint64
	Height            uint64
	DefaultDurationNS uint64
}

type Frame struct {
	Index          int
	TrackNumber    uint64
	Keyframe       bool
	KeyframeSource string
	PTSNS          int64
	DurationNS     int64
	BlockOffset    int64
	SampleOffset   int64
	SampleSize     int
	Sample         []byte
}

type ContainerCRC struct {
	ParentID      string
	ParentOffset  int64
	ElementOffset int64
	Stored        uint32
	Calculated    uint32
	OK            bool
}

type Document struct {
	Path             string
	FileSize         int64
	DocType          string
	DocTypeVersion   uint64
	TimestampScaleNS uint64
	Tracks           []Track
	Frames           []Frame
	ContainerCRCs    []ContainerCRC
	Issues           []Issue
}

type parser struct {
	r       io.ReaderAt
	size    int64
	doc     *Document
	tracks  map[uint64]*Track
	onFrame func(*Document, Frame) error
	err     error
}

// ParseFile opens path and parses its Matroska structure.
func ParseFile(path string) (*Document, error) {
	return ParseFileWithOptions(path, ParseOptions{})
}

// ParseOptions allows callers to consume frames as they are parsed instead of
// retaining every sample in memory.
type ParseOptions struct {
	OnFrame func(doc *Document, frame Frame) error
}

func ParseFileWithOptions(path string, opts ParseOptions) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return ParseWithOptions(f, st.Size(), path, opts)
}

// Parse parses Matroska data from r.
func Parse(r io.ReaderAt, size int64, path string) (*Document, error) {
	return ParseWithOptions(r, size, path, ParseOptions{})
}

func ParseWithOptions(r io.ReaderAt, size int64, path string, opts ParseOptions) (*Document, error) {
	p := &parser{
		r:       r,
		size:    size,
		tracks:  make(map[uint64]*Track),
		onFrame: opts.OnFrame,
		doc: &Document{
			Path:             path,
			FileSize:         size,
			TimestampScaleNS: 1_000_000,
		},
	}
	root, err := ebml.ReadHeaderAt(r, 0, size)
	if err != nil {
		return nil, err
	}
	if root.ID != ebml.IDEBML {
		return nil, fmt.Errorf("not an EBML/Matroska file: first element is %s", ebml.Name(root.ID))
	}
	p.parseEBML(root)

	seg, err := p.findSegment()
	if err != nil {
		return nil, err
	}
	p.parseSegment(seg)
	if p.err != nil {
		return nil, p.err
	}
	return p.doc, nil
}

func (p *parser) addIssue(offset int64, code, format string, args ...any) {
	p.doc.Issues = append(p.doc.Issues, Issue{
		Offset:  offset,
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	})
}

func (p *parser) children(h ebml.Header, fn func(ebml.Header, int64)) {
	end := h.End(p.size)
	off := h.DataOffset
	for off < end {
		ch, err := ebml.ReadHeaderAt(p.r, off, end)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				p.addIssue(off, "element_header", "%v", err)
			}
			return
		}
		chEnd := ch.End(end)
		fn(ch, chEnd)
		if p.err != nil {
			return
		}
		if chEnd <= off {
			p.addIssue(off, "element_stall", "element %s does not advance the parser", ebml.Name(ch.ID))
			return
		}
		off = chEnd
	}
}

func (p *parser) parseEBML(h ebml.Header) {
	p.children(h, func(ch ebml.Header, end int64) {
		switch ch.ID {
		case ebml.IDDocType:
			p.doc.DocType = p.readString(ch, end)
		case ebml.IDDocTypeVersion:
			if v, ok := p.readUint(ch, end); ok {
				p.doc.DocTypeVersion = v
			}
		}
	})
}

func (p *parser) findSegment() (ebml.Header, error) {
	end := p.size
	for off := int64(0); off < end; {
		h, err := ebml.ReadHeaderAt(p.r, off, end)
		if err != nil {
			return ebml.Header{}, err
		}
		if h.ID == ebml.IDSegment {
			return h, nil
		}
		next := h.End(end)
		if next <= off {
			break
		}
		off = next
	}
	return ebml.Header{}, errors.New("Matroska Segment not found")
}

func (p *parser) parseSegment(seg ebml.Header) {
	p.children(seg, func(ch ebml.Header, end int64) {
		switch ch.ID {
		case ebml.IDInfo:
			p.parseInfo(ch)
		case ebml.IDTracks:
			p.parseTracks(ch)
		case ebml.IDCluster:
			p.parseCluster(ch)
		}
	})
}

func (p *parser) parseInfo(h ebml.Header) {
	p.children(h, func(ch ebml.Header, end int64) {
		if ch.ID == ebml.IDTimecodeScale {
			if v, ok := p.readUint(ch, end); ok && v > 0 {
				p.doc.TimestampScaleNS = v
			}
		}
	})
	p.verifyCRC(h)
}

func (p *parser) parseTracks(h ebml.Header) {
	p.children(h, func(ch ebml.Header, end int64) {
		if ch.ID == ebml.IDTrackEntry {
			t := p.parseTrackEntry(ch)
			if t.Number != 0 {
				p.tracks[t.Number] = &t
			}
			p.doc.Tracks = append(p.doc.Tracks, t)
		}
	})
	p.verifyCRC(h)
}

func (p *parser) parseTrackEntry(h ebml.Header) Track {
	var t Track
	p.children(h, func(ch ebml.Header, end int64) {
		switch ch.ID {
		case ebml.IDTrackNumber:
			if v, ok := p.readUint(ch, end); ok {
				t.Number = v
			}
		case ebml.IDTrackUID:
			if v, ok := p.readUint(ch, end); ok {
				t.UID = v
			}
		case ebml.IDTrackType:
			if v, ok := p.readUint(ch, end); ok {
				t.Type = v
			}
		case ebml.IDCodecID:
			t.CodecID = p.readString(ch, end)
		case ebml.IDCodecPrivate:
			t.CodecPrivateOff = ch.DataOffset
			b, _ := ebml.ReadBytes(p.r, ch.DataOffset, int64(ch.Size))
			t.CodecPrivate = b
		case ebml.IDDefaultDuration:
			if v, ok := p.readUint(ch, end); ok {
				t.DefaultDurationNS = v
			}
		case ebml.IDVideo:
			p.parseVideo(ch, &t)
		}
	})
	return t
}

func (p *parser) parseVideo(h ebml.Header, t *Track) {
	p.children(h, func(ch ebml.Header, end int64) {
		switch ch.ID {
		case ebml.IDPixelWidth:
			if v, ok := p.readUint(ch, end); ok {
				t.Width = v
			}
		case ebml.IDPixelHeight:
			if v, ok := p.readUint(ch, end); ok {
				t.Height = v
			}
		}
	})
}

func (p *parser) readUint(h ebml.Header, end int64) (uint64, bool) {
	n := int(h.Size)
	if n <= 0 || n > 8 || h.DataOffset+int64(n) > end {
		return 0, false
	}
	v, err := ebml.ReadUint(p.r, h.DataOffset, n)
	if err != nil {
		return 0, false
	}
	return v, true
}

func (p *parser) readString(h ebml.Header, end int64) string {
	if h.Size == 0 || h.DataOffset+int64(h.Size) > end || h.Size > 1024 {
		return ""
	}
	b, err := ebml.ReadBytes(p.r, h.DataOffset, int64(h.Size))
	if err != nil {
		return ""
	}
	return string(b)
}

func (p *parser) verifyCRC(parent ebml.Header) {
	end := parent.End(p.size)
	p.children(parent, func(ch ebml.Header, chEnd int64) {
		if ch.ID != ebml.IDCRC32 || ch.Size != 4 {
			return
		}
		storedBytes, err := ebml.ReadBytes(p.r, ch.DataOffset, 4)
		if err != nil {
			return
		}
		// Matroska stores the IEEE CRC-32 little-endian.
		stored := uint32(storedBytes[0]) | uint32(storedBytes[1])<<8 |
			uint32(storedBytes[2])<<16 | uint32(storedBytes[3])<<24
		calc, err := p.checksum(chEnd, end)
		if err != nil {
			return
		}
		p.doc.ContainerCRCs = append(p.doc.ContainerCRCs, ContainerCRC{
			ParentID:      ebml.Name(parent.ID),
			ParentOffset:  parent.Offset,
			ElementOffset: ch.Offset,
			Stored:        stored,
			Calculated:    calc,
			OK:            stored == calc,
		})
	})
}

func (p *parser) checksum(start, end int64) (uint32, error) {
	if end < start {
		return 0, fmt.Errorf("invalid checksum range")
	}
	const chunk = 1 << 20
	buf := make([]byte, chunk)
	var h uint32
	for off := start; off < end; {
		n := int64(chunk)
		if end-off < n {
			n = end - off
		}
		read, err := p.r.ReadAt(buf[:n], off)
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		if read > 0 {
			h = crc32.Update(h, crc32.IEEETable, buf[:read])
		}
		off += int64(read)
		if read == 0 {
			break
		}
	}
	return h, nil
}
