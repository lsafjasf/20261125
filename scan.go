package matroska

import (
	"errors"
	"fmt"
	"os"
)

const (
	idEBML            uint64 = 0x1A45DFA3
	idSegment         uint64 = 0x18538067
	idSeekHead        uint64 = 0x114D9B74
	idInfo            uint64 = 0x1549A966
	idTimecodeScale   uint64 = 0x2AD7B1
	idTracks          uint64 = 0x1654AE6B
	idTrackEntry      uint64 = 0xAE
	idTrackNumber     uint64 = 0xD7
	idDefaultDuration uint64 = 0x23E383
	idCluster         uint64 = 0x1F43B675
	idClusterTimecode uint64 = 0xE7
	idBlockGroup      uint64 = 0xA0
	idBlock           uint64 = 0xA1
	idSimpleBlock     uint64 = 0xA3
	idBlockDuration   uint64 = 0x9B
	idVoid            uint64 = 0xEC
	idCRC32           uint64 = 0xBF
)

type scanner struct {
	data    []byte
	opts    ScanOptions
	result  *ScanResult
	scale   int64
	tracks  map[uint64]int64
	cluster int64
}

// Scan locates and parses Block/SimpleBlock elements in an in-memory Matroska
// file. Recoverable errors are returned in Issues and scanning continues at the
// next verifiable sync point.
func Scan(data []byte, opts ScanOptions) *ScanResult {
	s := &scanner{
		data:   data,
		opts:   opts,
		result: &ScanResult{},
		scale:  opts.TimestampScale,
		tracks: make(map[uint64]int64),
	}
	if s.scale <= 0 {
		s.scale = defaultTimestampScale
	}
	s.walk(0, int64(len(data)), contextRoot)
	s.flattenFrames()
	return s.result
}

// ScanFile reads path and scans it with Scan.
func ScanFile(path string, opts ScanOptions) (*ScanResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Scan(data, opts), nil
}

type scanContext uint8

const (
	contextRoot scanContext = iota
	contextSegment
	contextCluster
)

func (s *scanner) walk(pos, end int64, ctx scanContext) int64 {
	if end > int64(len(s.data)) {
		end = int64(len(s.data))
	}
	for pos < end {
		h, err := readElementHeader(s.data, int(pos))
		if err != nil {
			s.issueFromError(err, pos, 0, false)
			issueStart := len(s.result.Issues) - 1
			next, ok := s.findSync(pos+1, end)
			if !ok {
				return end
			}
			s.markRecovered(issueStart)
			pos = next
			continue
		}

		if h.unknown {
			next, ok := s.handleUnknownSize(pos, end, h, ctx)
			if ok {
				pos = next
				continue
			}
			issueStart := len(s.result.Issues) - 1
			next, recovered := s.findSync(pos+1, end)
			if !recovered {
				return end
			}
			s.markRecovered(issueStart)
			pos = next
			continue
		}

		dataEnd := int64(h.dataStart) + h.size
		if dataEnd > end {
			s.issue(ErrTruncated, pos, h.id,
				fmt.Sprintf("element declares %d bytes, only %d remain", h.size, end-int64(h.dataStart)), false)
			issueStart := len(s.result.Issues) - 1
			next, ok := s.findSync(pos+1, end)
			if !ok {
				return end
			}
			s.markRecovered(issueStart)
			pos = next
			continue
		}

		switch h.id {
		case idSegment:
			s.walk(int64(h.dataStart), dataEnd, contextSegment)
			pos = dataEnd
		case idEBML, idSeekHead, idVoid, idCRC32:
			pos = dataEnd
		case idInfo:
			s.parseInfo(int64(h.dataStart), dataEnd)
			pos = dataEnd
		case idTracks:
			s.parseTracks(int64(h.dataStart), dataEnd)
			pos = dataEnd
		case idCluster:
			s.parseCluster(int64(h.dataStart), dataEnd)
			pos = dataEnd
		case idBlockGroup:
			s.parseBlockGroup(int64(h.dataStart), dataEnd)
			pos = dataEnd
		case idBlock, idSimpleBlock:
			next, ok := s.parseBlockElement(pos, dataEnd, h.id, end)
			if !ok {
				issueStart := len(s.result.Issues) - 1
				next, ok = s.findSync(pos+1, end)
				if !ok {
					return end
				}
				s.markRecovered(issueStart)
			}
			pos = next
		case idTimecodeScale:
			s.readTimecodeScale(int64(h.dataStart), dataEnd)
			pos = dataEnd
		case idClusterTimecode:
			if value, ok := s.readUintElement(int64(h.dataStart), dataEnd, pos); ok {
				s.cluster = int64(value)
			}
			pos = dataEnd
		default:
			if s.opts.ReportUnknown {
				s.issue(ErrUnknownElement, pos, h.id, "skipped unknown element", true)
			}
			pos = dataEnd
		}
	}
	return end
}

func (s *scanner) handleUnknownSize(pos, end int64, h elementHeader, ctx scanContext) (int64, bool) {
	switch h.id {
	case idSegment:
		return s.walk(int64(h.dataStart), end, contextSegment), true
	case idCluster:
		clusterEnd := s.findClusterEnd(int64(h.dataStart), end)
		s.parseCluster(int64(h.dataStart), clusterEnd)
		return clusterEnd, true
	default:
		s.issue(ErrUnknownElement, pos, h.id, "element has unknown size and cannot be skipped safely", false)
		return 0, false
	}
}

func (s *scanner) parseBlockElement(pos, dataEnd int64, elementID uint64, parentEnd int64) (int64, bool) {
	h, err := readElementHeader(s.data, int(pos))
	if err != nil {
		s.issueFromError(err, pos, elementID, false)
		return pos, false
	}
	block, err := s.parseBlockAt(int64(h.dataStart), dataEnd, pos, 0)
	if err != nil {
		s.issueFromError(err, pos, elementID, false)
		return pos, false
	}
	if dataEnd < parentEnd && !s.validBoundary(dataEnd, parentEnd) {
		s.issue(ErrBlockBoundary, pos, elementID,
			"bytes after declared Block end do not form an EBML element", false)
		return pos, false
	}
	s.result.Blocks = append(s.result.Blocks, *block)
	return dataEnd, true
}

func (s *scanner) parseBlockAt(payloadStart, payloadEnd, elementStart, duration int64) (*Block, error) {
	if duration <= 0 {
		duration = s.opts.DefaultFrameDuration
	}
	trackValue, _, trackErr := readVINT(s.data[int(payloadStart):int(payloadEnd)], 0)
	if trackErr == nil && duration <= 0 {
		if trackDuration := s.tracks[trackValue]; trackDuration > 0 {
			duration = trackDuration
		}
	}
	return ParseBlock(s.data[int(payloadStart):int(payloadEnd)], BlockOptions{
		ElementOffset:        int64(elementStart),
		PayloadOffset:        int64(payloadStart),
		ClusterTimecode:      s.cluster,
		TimestampScale:       s.scale,
		DefaultFrameDuration: duration,
	})
}

func (s *scanner) flattenFrames() {
	for _, block := range s.result.Blocks {
		s.result.Frames = append(s.result.Frames, block.Frames...)
	}
}

func (s *scanner) issue(kind ErrorKind, offset int64, elementID uint64, detail string, recovered bool) {
	s.result.Issues = append(s.result.Issues, Issue{
		Kind: kind, Offset: offset, ElementID: elementID, Detail: detail, Recovered: recovered,
	})
}

func (s *scanner) markRecovered(from int) {
	for i := from; i < len(s.result.Issues); i++ {
		if i >= 0 {
			s.result.Issues[i].Recovered = true
		}
	}
}

func (s *scanner) issueFromError(err error, elementOffset int64, elementID uint64, recovered bool) {
	var pe *ParseError
	if errors.As(err, &pe) {
		s.issue(pe.Kind, pe.Offset, elementID, pe.Detail, recovered)
		return
	}
	kind := ErrInvalidBlock
	var ce *codecError
	if errors.As(err, &ce) {
		kind = ce.kind
	}
	s.issue(kind, elementOffset, elementID, err.Error(), recovered)
}
