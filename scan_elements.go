package matroska

import "math"

func (s *scanner) parseInfo(start, end int64) {
	for pos := start; pos < end; {
		h, err := readElementHeader(s.data, int(pos))
		if err != nil {
			s.issueFromError(err, pos, 0, false)
			return
		}
		if h.unknown {
			s.issue(ErrUnknownElement, pos, h.id, "unknown-sized element in Info", false)
			return
		}
		dataEnd := int64(h.dataStart) + h.size
		if dataEnd > end {
			s.issue(ErrTruncated, pos, h.id, "Info child overruns its parent", false)
			return
		}
		switch h.id {
		case idTimecodeScale:
			if value, ok := s.readUintElement(int64(h.dataStart), dataEnd, pos); ok &&
				s.opts.TimestampScale <= 0 {
				if value == 0 || value > math.MaxInt64 {
					s.issue(ErrInvalidElement, pos, h.id, "TimecodeScale must be a positive int64", true)
				} else {
					s.scale = int64(value)
				}
			}
		default:
			if s.opts.ReportUnknown {
				s.issue(ErrUnknownElement, pos, h.id, "skipped Info child", true)
			}
		}
		pos = dataEnd
	}
}

func (s *scanner) parseTracks(start, end int64) {
	for pos := start; pos < end; {
		h, err := readElementHeader(s.data, int(pos))
		if err != nil {
			s.issueFromError(err, pos, 0, false)
			return
		}
		if h.unknown {
			s.issue(ErrUnknownElement, pos, h.id, "unknown-sized element in Tracks", false)
			return
		}
		dataEnd := int64(h.dataStart) + h.size
		if dataEnd > end {
			s.issue(ErrTruncated, pos, h.id, "Tracks child overruns its parent", false)
			return
		}
		if h.id == idTrackEntry {
			s.parseTrackEntry(int64(h.dataStart), dataEnd)
		} else if s.opts.ReportUnknown {
			s.issue(ErrUnknownElement, pos, h.id, "skipped Tracks child", true)
		}
		pos = dataEnd
	}
}

func (s *scanner) parseTrackEntry(start, end int64) {
	var track, duration uint64
	for pos := start; pos < end; {
		h, err := readElementHeader(s.data, int(pos))
		if err != nil {
			s.issueFromError(err, pos, 0, false)
			break
		}
		if h.unknown {
			s.issue(ErrUnknownElement, pos, h.id, "unknown-sized element in TrackEntry", false)
			break
		}
		dataEnd := int64(h.dataStart) + h.size
		if dataEnd > end {
			s.issue(ErrTruncated, pos, h.id, "TrackEntry child overruns its parent", false)
			break
		}
		switch h.id {
		case idTrackNumber:
			if value, ok := s.readUintElement(int64(h.dataStart), dataEnd, pos); ok {
				track = value
			}
		case idDefaultDuration:
			if value, ok := s.readUintElement(int64(h.dataStart), dataEnd, pos); ok && value <= math.MaxInt64 {
				duration = value
			}
		}
		pos = dataEnd
	}
	if track > 0 {
		s.tracks[track] = int64(duration)
	}
}

func (s *scanner) readUintElement(start, end, elementPos int64) (uint64, bool) {
	if start < 0 || end > int64(len(s.data)) || start >= end || end-start > 8 {
		s.issue(ErrInvalidElement, elementPos, 0, "unsigned element must contain 1 to 8 bytes", true)
		return 0, false
	}
	return readUint(s.data[int(start):int(end)]), true
}

func (s *scanner) readTimecodeScale(start, end int64) {
	value, ok := s.readUintElement(start, end, start)
	if !ok || s.opts.TimestampScale > 0 {
		return
	}
	if value == 0 || value > math.MaxInt64 {
		s.issue(ErrInvalidElement, start, idTimecodeScale, "TimecodeScale must be a positive int64", true)
		return
	}
	s.scale = int64(value)
}

func (s *scanner) parseCluster(start, end int64) {
	previous := s.cluster
	s.cluster = 0
	if timecode, ok := s.findClusterTimecode(start, end); ok {
		s.cluster = timecode
	}
	s.walk(start, end, contextCluster)
	s.cluster = previous
}

func (s *scanner) findClusterTimecode(start, end int64) (int64, bool) {
	for pos := start; pos < end; {
		h, err := readElementHeader(s.data, int(pos))
		if err != nil || h.unknown {
			return 0, false
		}
		dataEnd := int64(h.dataStart) + h.size
		if dataEnd > end {
			return 0, false
		}
		if h.id == idClusterTimecode {
			value, ok := s.readUintElement(int64(h.dataStart), dataEnd, pos)
			if !ok || value > math.MaxInt64 {
				return 0, false
			}
			return int64(value), true
		}
		pos = dataEnd
	}
	return 0, false
}

func (s *scanner) parseBlockGroup(start, end int64) {
	var blockStart, blockEnd, blockPos int64
	blockStart, blockEnd, blockPos = -1, -1, -1
	var blockDuration uint64
	for pos := start; pos < end; {
		h, err := readElementHeader(s.data, int(pos))
		if err != nil {
			s.issueFromError(err, pos, 0, false)
			s.recoverInside(pos+1, end)
			return
		}
		if h.unknown {
			s.issue(ErrUnknownElement, pos, h.id, "unknown-sized element in BlockGroup", false)
			s.recoverInside(pos+1, end)
			return
		}
		dataEnd := int64(h.dataStart) + h.size
		if dataEnd > end {
			s.issue(ErrTruncated, pos, h.id, "BlockGroup child overruns its parent", false)
			s.recoverInside(pos+1, end)
			return
		}
		switch h.id {
		case idBlock:
			if blockStart < 0 {
				blockStart, blockEnd, blockPos = int64(h.dataStart), dataEnd, pos
			}
		case idBlockDuration:
			if value, ok := s.readUintElement(int64(h.dataStart), dataEnd, pos); ok {
				blockDuration = value
			}
		default:
			if s.opts.ReportUnknown {
				s.issue(ErrUnknownElement, pos, h.id, "skipped BlockGroup child", true)
			}
		}
		pos = dataEnd
	}
	if blockStart < 0 {
		s.issue(ErrInvalidElement, start, idBlockGroup, "BlockGroup has no Block", true)
		return
	}

	block, err := s.parseBlockAt(blockStart, blockEnd, blockPos, 0)
	if err != nil {
		s.issueFromError(err, blockPos, idBlock, false)
		s.recoverInside(blockPos+1, end)
		return
	}
	if blockDuration > 0 && s.opts.DefaultFrameDuration <= 0 &&
		s.tracks[block.TrackNumber] <= 0 && block.FrameCount > 1 {
		if blockDuration > uint64(math.MaxInt64/s.scale) {
			s.issue(ErrInvalidElement, blockPos, idBlockDuration, "BlockDuration overflows int64", true)
		} else {
			totalNanos := int64(blockDuration) * s.scale
			perFrame := totalNanos / int64(block.FrameCount)
			if perFrame > 0 {
				block, err = s.parseBlockAt(blockStart, blockEnd, blockPos, perFrame)
				if err != nil {
					s.issueFromError(err, blockPos, idBlock, false)
					return
				}
			}
		}
	}
	if blockEnd < end && !s.validBoundary(blockEnd, end) {
		s.issue(ErrBlockBoundary, blockPos, idBlock,
			"bytes after declared Block end do not form an EBML element", false)
		s.recoverInside(blockEnd+1, end)
		return
	}
	s.result.Blocks = append(s.result.Blocks, *block)
}

func (s *scanner) recoverInside(from, end int64) {
	issueStart := len(s.result.Issues) - 1
	next, ok := s.findSync(from, end)
	if !ok {
		return
	}
	s.markRecovered(issueStart)
	s.walk(next, end, contextCluster)
}

func (s *scanner) validBoundary(at, end int64) bool {
	if at == end {
		return true
	}
	if at < 0 || at >= end {
		return false
	}
	_, err := readElementHeader(s.data, int(at))
	return err == nil
}

func (s *scanner) findClusterEnd(start, parentEnd int64) int64 {
	for i := start; i+4 <= parentEnd; i++ {
		if s.data[i] != 0x1F {
			continue
		}
		h, err := readElementHeader(s.data, int(i))
		if err != nil || h.id != idCluster {
			continue
		}
		return i
	}
	return parentEnd
}

func (s *scanner) findSync(from, limit int64) (int64, bool) {
	if from < 0 {
		from = 0
	}
	for i := from; i < limit; i++ {
		first := s.data[i]
		if first != 0x1F && first != 0xA0 && first != 0xA1 && first != 0xA3 {
			continue
		}
		h, err := readElementHeader(s.data, int(i))
		if err != nil || h.dataStart > int(limit) {
			continue
		}
		if h.unknown {
			if h.id == idCluster {
				return i, true
			}
			continue
		}
		dataEnd := int64(h.dataStart) + h.size
		if dataEnd > limit {
			continue
		}
		switch h.id {
		case idCluster, idBlockGroup:
			return i, true
		case idBlock, idSimpleBlock:
			block, err := s.parseBlockAt(int64(h.dataStart), dataEnd, i, 0)
			if err != nil || len(block.Frames) == 0 {
				continue
			}
			if len(s.tracks) > 0 {
				if _, known := s.tracks[block.TrackNumber]; !known {
					continue
				}
			}
			if dataEnd < limit && !s.validBoundary(dataEnd, limit) {
				continue
			}
			return i, true
		}
	}
	return 0, false
}
