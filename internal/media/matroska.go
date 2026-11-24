package media

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	ebmlHeaderID  = 0x1A45DFA3
	segmentID     = 0x18538067
	infoID        = 0x1549A966
	tracksID      = 0x1654AE6B
	clusterID     = 0x1F43B675
	trackEntryID  = 0xAE
	blockGroupID  = 0xA0
	simpleBlockID = 0xA3
	blockID       = 0xA1
)

type ebmlElem struct {
	id           uint32
	payload      int64
	end          int64
	unknownSize  bool
	headerLength int64
}

func readMatroska(r io.ReaderAt, fileSize int64) (*File, error) {
	f := &File{Format: FormatMatroska}
	first, err := readEBML(r, 0, fileSize)
	if err != nil {
		return nil, err
	}
	if first.id != ebmlHeaderID {
		return nil, errors.New("missing EBML header")
	}
	var segment *ebmlElem
	for off := first.end; off < fileSize; {
		e, err := readEBML(r, off, fileSize)
		if err != nil {
			return nil, err
		}
		if e.id == segmentID {
			segment = &e
			break
		}
		off = e.end
	}
	if segment == nil {
		return nil, errors.New("missing Matroska Segment")
	}
	state := &mkvState{
		r: r, f: f, timestampScale: 1_000_000,
		byNumber: make(map[uint64]int),
	}
	if err := state.parseSegment(*segment); err != nil {
		return nil, err
	}
	if f.DeclaredDuration == 0 {
		f.DeclaredDuration = f.MaxPresentedEnd()
	}
	return f, nil
}

func readEBML(r io.ReaderAt, off, limit int64) (ebmlElem, error) {
	var e ebmlElem
	if limit-off < 2 {
		return e, io.ErrUnexpectedEOF
	}
	var b [8]byte
	if _, err := r.ReadAt(b[:1], off); err != nil {
		return e, err
	}
	idLen, err := ebmlLength(b[0])
	if err != nil || idLen > 4 || off+int64(idLen)+1 > limit {
		return e, fmt.Errorf("invalid EBML id at %d", off)
	}
	if _, err := r.ReadAt(b[:idLen], off); err != nil {
		return e, err
	}
	var id uint32
	for i := 0; i < idLen; i++ {
		id = id<<8 | uint32(b[i])
	}
	e.id = id
	sizeOff := off + int64(idLen)
	if _, err := r.ReadAt(b[:1], sizeOff); err != nil {
		return e, err
	}
	sizeLen, err := ebmlLength(b[0])
	if err != nil || sizeLen > 8 || sizeOff+int64(sizeLen) > limit {
		return e, fmt.Errorf("invalid EBML size at %d", sizeOff)
	}
	if _, err := r.ReadAt(b[:sizeLen], sizeOff); err != nil {
		return e, err
	}
	size, allOnes := ebmlSize(b[:sizeLen])
	if allOnes {
		e.unknownSize = true
		e.payload = sizeOff + int64(sizeLen)
		e.end = limit
		e.headerLength = int64(idLen) + int64(sizeLen)
		return e, nil
	}
	if size > uint64(math.MaxInt64) {
		return e, errors.New("EBML element too large")
	}
	e.payload = sizeOff + int64(sizeLen)
	e.end = e.payload + int64(size)
	e.headerLength = int64(idLen) + int64(sizeLen)
	if e.end < e.payload || e.end > limit {
		return e, fmt.Errorf("EBML element 0x%x overruns parent at %d", id, off)
	}
	return e, nil
}

func ebmlLength(first byte) (int, error) {
	if first == 0 {
		return 0, errors.New("zero EBML vint")
	}
	for i := 0; i < 8; i++ {
		if first&(0x80>>i) != 0 {
			return i + 1, nil
		}
	}
	return 0, errors.New("invalid EBML vint")
}

func ebmlSize(buf []byte) (uint64, bool) {
	length := len(buf)
	value := uint64(buf[0]) & ((1 << (8 - length)) - 1)
	allOnes := value == (1<<(8-length))-1
	for _, b := range buf[1:] {
		value = value<<8 | uint64(b)
		if b != 0xff {
			allOnes = false
		}
	}
	return value, allOnes
}

func parseEBMLUint(buf []byte) uint64 {
	var v uint64
	for _, b := range buf {
		v = v<<8 | uint64(b)
	}
	return v
}

func parseEBMLSigned(buf []byte) int64 {
	if len(buf) == 0 {
		return 0
	}
	v := parseEBMLUint(buf)
	bits := uint(len(buf) * 8)
	if bits == 64 {
		return int64(v)
	}
	if v&(1<<(bits-1)) != 0 {
		return int64(v | (^uint64(0) << bits))
	}
	return int64(v)
}

func parseEBMLFloat(buf []byte) float64 {
	switch len(buf) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(buf)))
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(buf))
	default:
		return 0
	}
}

type mkvState struct {
	r              io.ReaderAt
	f              *File
	timestampScale uint64
	byNumber       map[uint64]int
}

func (s *mkvState) parseSegment(seg ebmlElem) error {
	for off := seg.payload; off < seg.end; {
		e, err := readEBML(s.r, off, seg.end)
		if err != nil {
			return fmt.Errorf("segment at %d: %w", off, err)
		}
		switch e.id {
		case infoID:
			if err := s.parseInfo(e); err != nil {
				return err
			}
		case tracksID:
			if err := s.parseTracks(e); err != nil {
				return err
			}
		case clusterID:
			if e.unknownSize {
				return errors.New("unknown-sized Cluster is not supported; remux with known cluster sizes")
			}
			if err := s.parseCluster(e); err != nil {
				return err
			}
		}
		off = e.end
	}
	s.finishDurations()
	return nil
}

func (s *mkvState) parseInfo(info ebmlElem) error {
	for off := info.payload; off < info.end; {
		e, err := readEBML(s.r, off, info.end)
		if err != nil {
			return err
		}
		switch e.id {
		case 0x2AD7B1:
			buf, err := readAtFull(s.r, e.payload, e.end-e.payload)
			if err != nil {
				return err
			}
			if v := parseEBMLUint(buf); v > 0 {
				s.timestampScale = v
			}
		case 0x4489:
			buf, err := readAtFull(s.r, e.payload, e.end-e.payload)
			if err != nil {
				return err
			}
			ticks := parseEBMLFloat(buf)
			if ticks > 0 {
				s.f.DeclaredDuration = int64(ticks * float64(s.timestampScale))
			}
		}
		off = e.end
	}
	s.f.Timescale = 1_000_000_000
	return nil
}

func (s *mkvState) parseTracks(tracks ebmlElem) error {
	for off := tracks.payload; off < tracks.end; {
		e, err := readEBML(s.r, off, tracks.end)
		if err != nil {
			return err
		}
		if e.id == trackEntryID {
			t, err := s.parseTrackEntry(e)
			if err != nil {
				return err
			}
			if t.ID != 0 {
				s.byNumber[t.ID] = len(s.f.Tracks)
			}
			s.f.Tracks = append(s.f.Tracks, *t)
		}
		off = e.end
	}
	return nil
}

func (s *mkvState) parseTrackEntry(entry ebmlElem) (*Track, error) {
	t := &Track{Timescale: 1_000_000_000, TrackTimeScale: 1}
	for off := entry.payload; off < entry.end; {
		e, err := readEBML(s.r, off, entry.end)
		if err != nil {
			return nil, err
		}
		buf, err := readAtFull(s.r, e.payload, e.end-e.payload)
		if err != nil {
			return nil, err
		}
		switch e.id {
		case 0xD7:
			t.ID = parseEBMLUint(buf)
		case 0x83:
			switch parseEBMLUint(buf) {
			case 1:
				t.Kind = KindVideo
			case 2:
				t.Kind = KindAudio
			case 17:
				t.Kind = KindSubtitle
			default:
				t.Kind = KindOther
			}
		case 0x86:
			t.Codec = string(buf)
		case 0x22B59C:
			t.Language = string(buf)
		case 0x536E:
			t.Name = string(buf)
		case 0x23E383:
			t.DefaultDuration = int64(parseEBMLUint(buf))
		case 0x56AA:
			t.CodecDelay = int64(parseEBMLUint(buf))
		case 0x56BB:
			t.SeekPreRoll = int64(parseEBMLUint(buf))
		case 0x5378:
			t.TrackOffset = parseEBMLSigned(buf)
		case 0x23314F:
			if v := parseEBMLFloat(buf); v > 0 {
				t.TrackTimeScale = v
			}
		case 0x63A2:
			sum := sha256.Sum256(buf)
			t.CodecPrivateHash = binary.BigEndian.Uint64(sum[:8])
		case 0xE0:
			if err := s.parseVideo(e, t); err != nil {
				return nil, err
			}
		case 0xE1:
			if err := s.parseAudio(e, t); err != nil {
				return nil, err
			}
		}
		off = e.end
	}
	return t, nil
}

func (s *mkvState) parseVideo(e ebmlElem, t *Track) error {
	for off := e.payload; off < e.end; {
		c, err := readEBML(s.r, off, e.end)
		if err != nil {
			return err
		}
		buf, err := readAtFull(s.r, c.payload, c.end-c.payload)
		if err != nil {
			return err
		}
		switch c.id {
		case 0xB0:
			t.Width = int(parseEBMLUint(buf))
		case 0xBA:
			t.Height = int(parseEBMLUint(buf))
		}
		off = c.end
	}
	return nil
}

func (s *mkvState) parseAudio(e ebmlElem, t *Track) error {
	for off := e.payload; off < e.end; {
		c, err := readEBML(s.r, off, e.end)
		if err != nil {
			return err
		}
		buf, err := readAtFull(s.r, c.payload, c.end-c.payload)
		if err != nil {
			return err
		}
		switch c.id {
		case 0xB5:
			t.SampleRate = int(math.Round(parseEBMLFloat(buf)))
		case 0x9F:
			t.Channels = int(parseEBMLUint(buf))
		}
		off = c.end
	}
	return nil
}

func (s *mkvState) parseCluster(c ebmlElem) error {
	var clusterTS uint64
	for off := c.payload; off < c.end; {
		e, err := readEBML(s.r, off, c.end)
		if err != nil {
			return err
		}
		if e.id == 0xE7 {
			buf, err := readAtFull(s.r, e.payload, e.end-e.payload)
			if err != nil {
				return err
			}
			clusterTS = parseEBMLUint(buf)
		}
		off = e.end
	}
	for off := c.payload; off < c.end; {
		e, err := readEBML(s.r, off, c.end)
		if err != nil {
			return err
		}
		switch e.id {
		case simpleBlockID:
			data, err := readAtFull(s.r, e.payload, e.end-e.payload)
			if err != nil {
				return err
			}
			if err := s.addBlock(clusterTS, data, 0, 0, false); err != nil {
				return err
			}
		case blockGroupID:
			if err := s.parseBlockGroup(clusterTS, e); err != nil {
				return err
			}
		}
		off = e.end
	}
	return nil
}

func (s *mkvState) parseBlockGroup(clusterTS uint64, group ebmlElem) error {
	var data []byte
	var duration int64
	var discard int64
	hasReference := false
	for off := group.payload; off < group.end; {
		e, err := readEBML(s.r, off, group.end)
		if err != nil {
			return err
		}
		switch e.id {
		case blockID:
			data, err = readAtFull(s.r, e.payload, e.end-e.payload)
			if err != nil {
				return err
			}
		case 0x9B:
			buf, err := readAtFull(s.r, e.payload, e.end-e.payload)
			if err != nil {
				return err
			}
			duration = int64(parseEBMLUint(buf))
		case 0x75A2:
			buf, err := readAtFull(s.r, e.payload, e.end-e.payload)
			if err != nil {
				return err
			}
			discard = parseEBMLSigned(buf)
		case 0xFB:
			hasReference = true
		}
		off = e.end
	}
	if data == nil {
		return nil
	}
	return s.addBlock(clusterTS, data, duration, discard, !hasReference)
}

func (s *mkvState) addBlock(clusterTS uint64, data []byte, blockDurationTicks, discardNS int64, key bool) error {
	trackNo, hdr, err := parseMKVBlockHeader(data)
	if err != nil {
		return err
	}
	idx, ok := s.byNumber[trackNo]
	if !ok {
		return nil
	}
	t := &s.f.Tracks[idx]
	frames, err := splitMKVLacing(data, hdr)
	if err != nil {
		return err
	}
	scale := int64(s.timestampScale)
	base := int64(clusterTS)*scale +
		int64(float64(hdr.relative)*t.TrackTimeScale*float64(scale)) + t.TrackOffset
	perFrameDuration := t.DefaultDuration
	if blockDurationTicks > 0 && len(frames) > 0 {
		perFrameDuration = blockDurationTicks * scale / int64(len(frames))
	}
	for i, frame := range frames {
		sum := sha256.Sum256(frame)
		sample := Sample{
			Index: len(t.Samples), PTS: base + int64(i)*max64(perFrameDuration, 0),
			Duration: perFrameDuration, Size: int64(len(frame)),
			Hash: binary.BigEndian.Uint64(sum[:8]), Key: key,
			Empty: t.Kind == KindSubtitle && len(frame) == 0,
		}
		if i == len(frames)-1 {
			sample.DiscardPadding = discardNS
		}
		t.Samples = append(t.Samples, sample)
	}
	return nil
}

type mkvBlockHeader struct {
	trackNumber uint64
	relative    int16
	flags       byte
	dataOffset  int
}

func parseMKVBlockHeader(data []byte) (uint64, mkvBlockHeader, error) {
	var h mkvBlockHeader
	if len(data) < 4 {
		return 0, h, errors.New("truncated Matroska block")
	}
	l, err := ebmlLength(data[0])
	if err != nil || l > 8 || l+3 > len(data) {
		return 0, h, errors.New("invalid Matroska block track number")
	}
	track, _ := ebmlSize(data[:l])
	h.trackNumber = track
	h.relative = int16(binary.BigEndian.Uint16(data[l : l+2]))
	h.flags = data[l+2]
	h.dataOffset = l + 3
	return track, h, nil
}

func splitMKVLacing(data []byte, h mkvBlockHeader) ([][]byte, error) {
	body := data[h.dataOffset:]
	lacing := (h.flags >> 1) & 0x03
	if lacing == 0 {
		return [][]byte{body}, nil
	}
	if len(body) < 1 {
		return nil, errors.New("truncated lacing header")
	}
	count := int(body[0]) + 1
	body = body[1:]
	if count <= 0 {
		return nil, errors.New("invalid lacing frame count")
	}
	sizes := make([]int, count)
	switch lacing {
	case 0x02:
		if len(body)%count != 0 {
			return nil, errors.New("fixed lacing payload not divisible")
		}
		for i := range sizes {
			sizes[i] = len(body) / count
		}
	case 0x01:
		total := 0
		for i := 0; i < count-1; i++ {
			n := 0
			for {
				if len(body) == 0 {
					return nil, errors.New("truncated Xiph lacing")
				}
				v := int(body[0])
				body = body[1:]
				n += v
				if v != 255 {
					break
				}
			}
			sizes[i] = n
			total += n
		}
		if total > len(body) {
			return nil, errors.New("Xiph lacing exceeds payload")
		}
		sizes[count-1] = len(body) - total
	case 0x03:
		if len(body) == 0 {
			return nil, errors.New("truncated EBML lacing")
		}
		first, n := readVintValue(body)
		if n == 0 || first > uint64(len(body)-n) {
			return nil, errors.New("invalid EBML lacing first size")
		}
		sizes[0] = int(first)
		body = body[n:]
		total := sizes[0]
		prev := int64(first)
		for i := 1; i < count-1; i++ {
			delta, n := readSignedVint(body)
			if n == 0 {
				return nil, errors.New("truncated EBML lacing delta")
			}
			body = body[n:]
			prev += delta
			if prev < 0 || prev > int64(len(body)) {
				return nil, errors.New("EBML lacing size out of range")
			}
			sizes[i] = int(prev)
			total += sizes[i]
		}
		if total > len(body) {
			return nil, errors.New("EBML lacing exceeds payload")
		}
		sizes[count-1] = len(body) - total
	default:
		return nil, errors.New("reserved lacing mode")
	}
	out := make([][]byte, 0, count)
	pos := 0
	for _, n := range sizes {
		if n < 0 || pos+n > len(body) {
			return nil, errors.New("laced frame overruns block")
		}
		out = append(out, body[pos:pos+n])
		pos += n
	}
	return out, nil
}

func readVintValue(buf []byte) (uint64, int) {
	if len(buf) == 0 {
		return 0, 0
	}
	l, err := ebmlLength(buf[0])
	if err != nil || l > len(buf) {
		return 0, 0
	}
	v, _ := ebmlSize(buf[:l])
	return v, l
}

func readSignedVint(buf []byte) (int64, int) {
	if len(buf) == 0 {
		return 0, 0
	}
	l, err := ebmlLength(buf[0])
	if err != nil || l > 8 || l > len(buf) {
		return 0, 0
	}
	v := uint64(buf[0]) & ((1 << uint(8-l)) - 1)
	for i := 1; i < l; i++ {
		v = v<<8 | uint64(buf[i])
	}
	bias := (int64(1) << (7*uint(l) - 1)) - 1
	return int64(v) - bias, l
}

func (s *mkvState) finishDurations() {
	for i := range s.f.Tracks {
		t := &s.f.Tracks[i]
		var prev int64
		for j := range t.Samples {
			smp := &t.Samples[j]
			if smp.Duration > 0 {
				prev = smp.Duration
				continue
			}
			switch {
			case j+1 < len(t.Samples) && t.Samples[j+1].PTS > smp.PTS:
				smp.Duration = t.Samples[j+1].PTS - smp.PTS
			case t.DefaultDuration > 0:
				smp.Duration = t.DefaultDuration
			case prev > 0:
				smp.Duration = prev
			}
			if smp.Duration > 0 {
				prev = smp.Duration
			}
		}
		span := t.RawSpan()
		if span.End > span.Start {
			t.Duration = uint64(span.End - span.Start)
		}
	}
}
