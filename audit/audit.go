package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"ffv1mkaudit/ebml"
)

type Options struct {
	ExpectedSlices int
	Expectations   *Expectations
	MaxFrameBytes  int64
	HashFile       bool
	FFV1CRC        string
}

type parser struct {
	r             io.ReaderAt
	size          int64
	opts          Options
	report        Report
	tracks        map[uint64]TrackInfo
	seg           ebml.Header
	hasSeg        bool
	frameSeq      int
	blockSeq      int
	clusterOffset int64
}

func AnalyzePath(path string, opts Options) (*Report, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	report, err := AnalyzeReaderAt(file, info.Size(), opts)
	if err != nil {
		return nil, err
	}
	report.Path = path
	if opts.HashFile {
		hash := sha256.New()
		if _, err := io.Copy(hash, io.NewSectionReader(file, 0, info.Size())); err != nil {
			report.addIssue(Issue{Severity: SeverityWarning, Code: "file_hash_failed", Message: err.Error()})
		} else {
			report.FileSHA256 = hex.EncodeToString(hash.Sum(nil))
		}
	}
	return report, nil
}

func AnalyzeReaderAt(r io.ReaderAt, size int64, opts Options) (*Report, error) {
	if opts.MaxFrameBytes <= 0 {
		opts.MaxFrameBytes = 512 << 20
	}
	p := &parser{
		r:      r,
		size:   size,
		opts:   opts,
		tracks: map[uint64]TrackInfo{},
		report: Report{
			SchemaVersion:   SchemaVersion,
			FileSize:        size,
			TimecodeScaleNS: 1_000_000,
			Frames:          []FrameReport{},
			FrameHashes:     []FrameHash{},
		},
	}
	if err := p.parse(); err != nil {
		return nil, err
	}
	p.refineFFV1Modes()
	p.inferSliceConsistency()
	p.applyExpectations()
	p.finalize()
	return &p.report, nil
}

func (r *Report) addIssue(issue Issue) {
	r.Issues = append(r.Issues, issue)
}

func (p *parser) addIssue(issue Issue) {
	p.report.addIssue(issue)
}

func (p *parser) readHeader(offset int64) (ebml.Header, bool) {
	h, err := ebml.ReadHeaderAt(p.r, offset)
	if err != nil {
		p.addIssue(Issue{
			Severity: SeverityError,
			Code:     "ebml_header_unreadable",
			Message:  err.Error(),
			Offset:   offset,
		})
		return ebml.Header{}, false
	}
	return h, true
}

func (p *parser) elementEnd(h ebml.Header) int64 {
	return p.elementEndWithin(h, p.size)
}

func (p *parser) elementEndWithin(h ebml.Header, bound int64) int64 {
	if h.UnknownSize {
		return bound
	}
	end := h.DataOffset + int64(h.Size)
	if end > bound {
		p.addIssue(Issue{
			Severity:  SeverityError,
			Code:      "ebml_element_truncated",
			Message:   "declared element extends past its available boundary",
			Offset:    h.Offset,
			EndOffset: end,
		})
		return bound
	}
	return end
}

func (p *parser) readUint(h ebml.Header) (uint64, bool) {
	end := p.elementEnd(h)
	if h.DataOffset >= end {
		return 0, false
	}
	value, err := ebml.ReadUint(p.r, h.DataOffset, int(h.Size))
	if err != nil {
		p.addIssue(Issue{Severity: SeverityError, Code: "ebml_uint_unreadable", Message: err.Error(), Offset: h.Offset})
		return 0, false
	}
	return value, true
}

func (p *parser) readString(h ebml.Header) string {
	size := int64(h.Size)
	if size <= 0 || size > 1<<20 {
		return ""
	}
	buf, err := ebml.ReadBytes(p.r, h.DataOffset, size)
	if err != nil {
		p.addIssue(Issue{Severity: SeverityWarning, Code: "ebml_string_unreadable", Message: err.Error(), Offset: h.Offset})
	}
	return string(buf)
}

func (p *parser) readBytes(h ebml.Header) []byte {
	size := int64(h.Size)
	if size <= 0 {
		return nil
	}
	buf, err := ebml.ReadBytes(p.r, h.DataOffset, size)
	if err != nil {
		p.addIssue(Issue{Severity: SeverityWarning, Code: "ebml_bytes_short_read", Message: err.Error(), Offset: h.Offset})
	}
	return buf
}

func (p *parser) parse() error {
	root, err := ebml.ReadHeaderAt(p.r, 0)
	if err != nil {
		return err
	}
	if root.ID != ebml.IDEBML {
		return &FatalFormatError{Message: "missing EBML header", Offset: 0}
	}
	p.parseEBMLHeader(root)

	offset := root.ElementEnd(p.size)
	for offset < p.size {
		h, ok := p.readHeader(offset)
		if !ok {
			break
		}
		if h.ID == ebml.IDSegment {
			p.seg = h
			p.hasSeg = true
			p.parseSegment(h)
			offset = h.ElementEnd(p.size)
			continue
		}
		offset = h.ElementEnd(p.size)
		if !h.UnknownSize && int64(h.Size) < 0 {
			break
		}
	}
	if !p.hasSeg {
		return &FatalFormatError{Message: "Matroska Segment element not found"}
	}
	p.collectUnknowns()
	return nil
}

type FatalFormatError struct {
	Message string
	Offset  int64
}

func (e *FatalFormatError) Error() string { return e.Message }
