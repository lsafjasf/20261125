package media

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

type mp4Box struct {
	typ         string
	start       int64
	payload     int64
	end         int64
	headerBytes int64
}

func readMP4(r io.ReaderAt, fileSize int64) (*File, error) {
	f := &File{Format: FormatMP4}
	top, err := mp4Boxes(r, 0, fileSize)
	if err != nil {
		return nil, err
	}
	var moov *mp4Box
	for i := range top {
		b := &top[i]
		switch b.typ {
		case "ftyp":
			data, err := readAtFull(r, b.payload, min64(b.end-b.payload, 12))
			if err != nil {
				return nil, err
			}
			if len(data) >= 8 {
				f.Brand = string(data[:4])
			}
		case "moov":
			moov = b
		}
	}
	if moov == nil {
		return nil, errors.New("MP4 has no moov box")
	}
	if err := parseMP4Movie(r, f, *moov); err != nil {
		return nil, err
	}
	return f, nil
}

func mp4Boxes(r io.ReaderAt, start, end int64) ([]mp4Box, error) {
	var out []mp4Box
	for off := start; off < end; {
		if end-off < 8 {
			if end-off == 0 {
				break
			}
			return nil, fmt.Errorf("truncated box header at %d", off)
		}
		var h [16]byte
		if _, err := r.ReadAt(h[:8], off); err != nil {
			return nil, err
		}
		size := int64(binary.BigEndian.Uint32(h[:4]))
		typ := string(h[4:8])
		header := int64(8)
		if size == 1 {
			if end-off < 16 {
				return nil, fmt.Errorf("truncated large box %q", typ)
			}
			if _, err := r.ReadAt(h[:16], off); err != nil {
				return nil, err
			}
			u := binary.BigEndian.Uint64(h[8:16])
			if u > math.MaxInt64 {
				return nil, fmt.Errorf("box %q too large", typ)
			}
			size, header = int64(u), 16
		} else if size == 0 {
			size = end - off
		}
		if size < header || off+size > end {
			return nil, fmt.Errorf("invalid box %q size %d at %d", typ, size, off)
		}
		out = append(out, mp4Box{
			typ: typ, start: off, payload: off + header,
			end: off + size, headerBytes: header,
		})
		off += size
	}
	return out, nil
}

func parseMP4Movie(r io.ReaderAt, f *File, moov mp4Box) error {
	boxes, err := mp4Boxes(r, moov.payload, moov.end)
	if err != nil {
		return fmt.Errorf("moov: %w", err)
	}
	for i := range boxes {
		switch boxes[i].typ {
		case "mvhd":
			ts, dur, err := parseMVHD(r, boxes[i])
			if err != nil {
				return err
			}
			f.Timescale = ts
			f.DeclaredDuration = scaleNanos(dur, ts)
		case "trak":
			t, err := parseMP4Track(r, boxes[i], f.Timescale)
			if err != nil {
				return err
			}
			if t != nil {
				f.Tracks = append(f.Tracks, *t)
			}
		}
	}
	return nil
}

func parseMVHD(r io.ReaderAt, b mp4Box) (uint64, int64, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return 0, 0, err
	}
	if len(data) < 24 {
		return 0, 0, errors.New("mvhd too short")
	}
	switch data[0] {
	case 0:
		return uint64(binary.BigEndian.Uint32(data[12:16])),
			int64(binary.BigEndian.Uint32(data[16:20])), nil
	case 1:
		if len(data) < 36 {
			return 0, 0, errors.New("mvhd v1 too short")
		}
		d := binary.BigEndian.Uint64(data[24:32])
		if d > math.MaxInt64 {
			return 0, 0, errors.New("mvhd duration overflow")
		}
		return uint64(binary.BigEndian.Uint32(data[20:24])), int64(d), nil
	default:
		return 0, 0, fmt.Errorf("unsupported mvhd version %d", data[0])
	}
}

type mp4TrackParts struct {
	tkhd, mdhd, hdlr, stbl *mp4Box
	edts                   *mp4Box
}

func parseMP4Track(r io.ReaderAt, trak mp4Box, movieTimescale uint64) (*Track, error) {
	parts, err := walkMP4Track(r, trak)
	if err != nil {
		return nil, err
	}
	if parts.mdhd == nil || parts.hdlr == nil {
		return nil, errors.New("trak missing mdhd or hdlr")
	}
	timescale, duration, err := parseMDHD(r, *parts.mdhd)
	if err != nil {
		return nil, err
	}
	kind, codec, err := parseHDLR(r, *parts.hdlr)
	if err != nil {
		return nil, err
	}
	t := &Track{
		Kind: kind, Codec: codec, Timescale: timescale,
		Duration: duration, MovieTimescale: movieTimescale,
	}
	if parts.tkhd != nil {
		_ = parseTKHD(r, *parts.tkhd, t)
	}
	if parts.edts != nil {
		edits, err := parseEDTS(r, *parts.edts)
		if err != nil {
			return nil, err
		}
		t.Edits = edits
	}
	if parts.stbl != nil {
		if err := parseSTBL(r, *parts.stbl, t); err != nil {
			return nil, fmt.Errorf("track handler %s: %w", kind, err)
		}
	}
	return t, nil
}

func walkMP4Track(r io.ReaderAt, trak mp4Box) (mp4TrackParts, error) {
	var p mp4TrackParts
	children, err := mp4Boxes(r, trak.payload, trak.end)
	if err != nil {
		return p, err
	}
	for i := range children {
		b := &children[i]
		switch b.typ {
		case "tkhd":
			p.tkhd = b
		case "edts":
			p.edts = b
		case "mdia":
			mdia, err := mp4Boxes(r, b.payload, b.end)
			if err != nil {
				return p, err
			}
			for j := range mdia {
				switch mdia[j].typ {
				case "mdhd":
					p.mdhd = &mdia[j]
				case "hdlr":
					p.hdlr = &mdia[j]
				case "minf":
					minf, err := mp4Boxes(r, mdia[j].payload, mdia[j].end)
					if err != nil {
						return p, err
					}
					for k := range minf {
						if minf[k].typ != "stbl" {
							continue
						}
						p.stbl = &minf[k]
					}
				}
			}
		}
	}
	return p, nil
}

func parseTKHD(r io.ReaderAt, b mp4Box, t *Track) error {
	data, err := readAtFull(r, b.payload, min64(b.end-b.payload, 104))
	if err != nil || len(data) < 8 {
		return err
	}
	var w, h int
	switch data[0] {
	case 0:
		if len(data) >= 84 {
			t.ID = uint64(binary.BigEndian.Uint32(data[12:16]))
			w = int(binary.BigEndian.Uint32(data[76:80]) >> 16)
			h = int(binary.BigEndian.Uint32(data[80:84]) >> 16)
		}
	case 1:
		if len(data) >= 96 {
			t.ID = uint64(binary.BigEndian.Uint32(data[20:24]))
			w = int(binary.BigEndian.Uint32(data[88:92]) >> 16)
			h = int(binary.BigEndian.Uint32(data[92:96]) >> 16)
		}
	}
	t.Width, t.Height = w, h
	return nil
}

func parseMDHD(r io.ReaderAt, b mp4Box) (uint64, uint64, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return 0, 0, err
	}
	if len(data) < 24 {
		return 0, 0, errors.New("mdhd too short")
	}
	switch data[0] {
	case 0:
		return uint64(binary.BigEndian.Uint32(data[12:16])),
			uint64(binary.BigEndian.Uint32(data[16:20])), nil
	case 1:
		if len(data) < 36 {
			return 0, 0, errors.New("mdhd v1 too short")
		}
		return uint64(binary.BigEndian.Uint32(data[20:24])),
			binary.BigEndian.Uint64(data[24:32]), nil
	default:
		return 0, 0, fmt.Errorf("unsupported mdhd version %d", data[0])
	}
}

func parseHDLR(r io.ReaderAt, b mp4Box) (Kind, string, error) {
	data, err := readAtFull(r, b.payload, min64(b.end-b.payload, 12))
	if err != nil {
		return KindOther, "", err
	}
	if len(data) < 12 {
		return KindOther, "", errors.New("hdlr too short")
	}
	switch string(data[8:12]) {
	case "vide":
		return KindVideo, "", nil
	case "soun":
		return KindAudio, "", nil
	case "sbtl", "subt", "text", "clcp":
		return KindSubtitle, "", nil
	default:
		return KindOther, string(data[8:12]), nil
	}
}

func parseEDTS(r io.ReaderAt, b mp4Box) ([]Edit, error) {
	children, err := mp4Boxes(r, b.payload, b.end)
	if err != nil {
		return nil, err
	}
	for i := range children {
		if children[i].typ != "elst" {
			continue
		}
		return parseELST(r, children[i])
	}
	return nil, nil
}

func parseELST(r io.ReaderAt, b mp4Box) ([]Edit, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, errors.New("elst too short")
	}
	version := data[0]
	count := binary.BigEndian.Uint32(data[4:8])
	pos := 8
	need := 12
	if version == 1 {
		need = 20
	}
	if uint64(pos)+uint64(count)*uint64(need) > uint64(len(data)) {
		return nil, errors.New("truncated elst entries")
	}
	edits := make([]Edit, 0, count)
	for i := uint32(0); i < count; i++ {
		var e Edit
		if version == 1 {
			e.SegmentDuration = binary.BigEndian.Uint64(data[pos : pos+8])
			e.MediaTime = int64(binary.BigEndian.Uint64(data[pos+8 : pos+16]))
		} else {
			e.SegmentDuration = uint64(binary.BigEndian.Uint32(data[pos : pos+4]))
			e.MediaTime = int64(int32(binary.BigEndian.Uint32(data[pos+4 : pos+8])))
		}
		rawRate := int32(binary.BigEndian.Uint32(data[pos+need-4 : pos+need]))
		e.MediaRate = float64(rawRate) / 65536
		edits = append(edits, e)
		pos += need
	}
	return edits, nil
}

type mp4SampleTable struct {
	durations []int64
	ctts      []int64
	sizes     []int64
	offsets   []int64
	chunks    []int64
	stsc      []stscEntry
	sync      map[int]bool
	codec     string
}

func parseSTBL(r io.ReaderAt, b mp4Box, t *Track) error {
	children, err := mp4Boxes(r, b.payload, b.end)
	if err != nil {
		return err
	}
	var st *mp4SampleTable
	for i := range children {
		c := &children[i]
		switch c.typ {
		case "stsd":
			codec, err := parseSTSD(r, *c, t)
			if err != nil {
				return err
			}
			t.Codec = codec
		case "stts":
			st, err = ensureTable(st)
			if err == nil {
				st.durations, err = parseSTTS(r, *c)
			}
			if err != nil {
				return err
			}
		case "ctts":
			st, err = ensureTable(st)
			if err == nil {
				st.ctts, err = parseCTTS(r, *c)
			}
			if err != nil {
				return err
			}
		case "stsz":
			st, err = ensureTable(st)
			if err == nil {
				st.sizes, err = parseSTSZ(r, *c)
			}
			if err != nil {
				return err
			}
		case "stz2":
			st, err = ensureTable(st)
			if err == nil {
				st.sizes, err = parseSTZ2(r, *c)
			}
			if err != nil {
				return err
			}
		case "stsc":
			st, err = ensureTable(st)
			if err == nil {
				st.stsc, err = parseSTSC(r, *c)
			}
			if err != nil {
				return err
			}
		case "stco", "co64":
			st, err = ensureTable(st)
			if err == nil {
				st.chunks, err = parseChunkOffsets(r, *c)
			}
			if err != nil {
				return err
			}
		case "stss":
			st, err = ensureTable(st)
			if err == nil {
				st.sync, err = parseSTSS(r, *c)
			}
			if err != nil {
				return err
			}
		}
	}
	if st == nil || len(st.sizes) == 0 {
		return nil
	}
	if len(st.durations) == 0 {
		st.durations = inferDurations(t.Duration, len(st.sizes), t.Timescale)
	}
	if len(st.durations) != len(st.sizes) {
		return fmt.Errorf("stts sample count %d != stsz %d", len(st.durations), len(st.sizes))
	}
	st.offsets, err = mapMP4Chunks(st, len(st.sizes))
	if err != nil {
		return err
	}
	if len(st.offsets) != len(st.sizes) {
		return fmt.Errorf("chunk mapping produced %d offsets for %d samples", len(st.offsets), len(st.sizes))
	}
	t.Samples = make([]Sample, len(st.sizes))
	var dts int64
	for i, size := range st.sizes {
		pts := dts
		if i < len(st.ctts) {
			pts += st.ctts[i]
		}
		t.Samples[i] = Sample{
			Index: i, PTS: pts, Duration: st.durations[i], Size: size,
			Key: st.sync == nil || st.sync[i], Empty: false,
		}
		dts += st.durations[i]
	}
	if err := hashMP4Samples(r, st.offsets, t); err != nil {
		return err
	}
	return nil
}

func parseSTSD(r io.ReaderAt, b mp4Box, t *Track) (string, error) {
	data, err := readAtFull(r, b.payload, min64(b.end-b.payload, 8))
	if err != nil {
		return "", err
	}
	if len(data) < 8 {
		return "", errors.New("stsd too short")
	}
	entries, err := mp4Boxes(r, b.payload+8, b.end)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	codec := strings.TrimRight(entries[0].typ, "\x00 ")
	head, err := readAtFull(r, entries[0].payload, min64(entries[0].end-entries[0].payload, 32))
	if err != nil {
		return codec, err
	}
	if t.Kind == KindVideo && len(head) >= 28 {
		w := int(binary.BigEndian.Uint16(head[24:26]))
		h := int(binary.BigEndian.Uint16(head[26:28]))
		if w > 0 {
			t.Width = w
		}
		if h > 0 {
			t.Height = h
		}
	}
	if t.Kind == KindAudio && len(head) >= 28 {
		t.Channels = int(binary.BigEndian.Uint16(head[16:18]))
		rawRate := binary.BigEndian.Uint32(head[24:28])
		rate := int(rawRate >> 16)
		if rate == 0 {
			rate = int(rawRate)
		}
		t.SampleRate = rate
	}
	return codec, nil
}

func parseSTTS(r io.ReaderAt, b mp4Box) ([]int64, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, errors.New("stts too short")
	}
	count := binary.BigEndian.Uint32(data[4:8])
	if uint64(8)+uint64(count)*8 > uint64(len(data)) {
		return nil, errors.New("truncated stts")
	}
	out := make([]int64, 0)
	pos := 8
	for i := uint32(0); i < count; i++ {
		n := binary.BigEndian.Uint32(data[pos : pos+4])
		d := int64(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
		if n > 100_000_000 {
			return nil, errors.New("unreasonable stts sample count")
		}
		for j := uint32(0); j < n; j++ {
			out = append(out, d)
		}
		pos += 8
	}
	return out, nil
}

func parseCTTS(r io.ReaderAt, b mp4Box) ([]int64, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, errors.New("ctts too short")
	}
	version := data[0]
	count := binary.BigEndian.Uint32(data[4:8])
	if uint64(8)+uint64(count)*8 > uint64(len(data)) {
		return nil, errors.New("truncated ctts")
	}
	out := make([]int64, 0)
	pos := 8
	for i := uint32(0); i < count; i++ {
		n := binary.BigEndian.Uint32(data[pos : pos+4])
		raw := binary.BigEndian.Uint32(data[pos+4 : pos+8])
		var off int64
		if version == 0 {
			off = int64(raw)
		} else {
			off = int64(int32(raw))
		}
		for j := uint32(0); j < n; j++ {
			out = append(out, off)
		}
		pos += 8
	}
	return out, nil
}

func parseSTSZ(r io.ReaderAt, b mp4Box) ([]int64, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 {
		return nil, errors.New("stsz too short")
	}
	fixed := binary.BigEndian.Uint32(data[4:8])
	count := binary.BigEndian.Uint32(data[8:12])
	if fixed != 0 {
		out := make([]int64, count)
		for i := range out {
			out[i] = int64(fixed)
		}
		return out, nil
	}
	if uint64(12)+uint64(count)*4 > uint64(len(data)) {
		return nil, errors.New("truncated stsz")
	}
	out := make([]int64, count)
	for i := range out {
		out[i] = int64(binary.BigEndian.Uint32(data[12+i*4 : 16+i*4]))
	}
	return out, nil
}

func parseSTZ2(r io.ReaderAt, b mp4Box) ([]int64, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 {
		return nil, errors.New("stz2 too short")
	}
	field := data[7]
	count := binary.BigEndian.Uint32(data[8:12])
	out := make([]int64, count)
	switch field {
	case 4:
		if uint64(12)+uint64((count+1)/2) > uint64(len(data)) {
			return nil, errors.New("truncated stz2 4-bit")
		}
		for i := range out {
			v := data[12+i/2]
			if i%2 == 0 {
				out[i] = int64(v >> 4)
			} else {
				out[i] = int64(v & 0x0f)
			}
		}
	case 8:
		if uint64(12)+uint64(count) > uint64(len(data)) {
			return nil, errors.New("truncated stz2 8-bit")
		}
		for i := range out {
			out[i] = int64(data[12+i])
		}
	case 16:
		if uint64(12)+uint64(count)*2 > uint64(len(data)) {
			return nil, errors.New("truncated stz2 16-bit")
		}
		for i := range out {
			out[i] = int64(binary.BigEndian.Uint16(data[12+i*2 : 14+i*2]))
		}
	default:
		return nil, fmt.Errorf("invalid stz2 field size %d", field)
	}
	return out, nil
}

type stscEntry struct {
	FirstChunk      uint32
	SamplesPerChunk uint32
}

func parseSTSC(r io.ReaderAt, b mp4Box) ([]stscEntry, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, errors.New("stsc too short")
	}
	count := binary.BigEndian.Uint32(data[4:8])
	if uint64(8)+uint64(count)*12 > uint64(len(data)) {
		return nil, errors.New("truncated stsc")
	}
	out := make([]stscEntry, count)
	for i := range out {
		pos := 8 + i*12
		out[i] = stscEntry{
			FirstChunk:      binary.BigEndian.Uint32(data[pos : pos+4]),
			SamplesPerChunk: binary.BigEndian.Uint32(data[pos+4 : pos+8]),
		}
	}
	return out, nil
}

func parseChunkOffsets(r io.ReaderAt, b mp4Box) ([]int64, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, errors.New("chunk offset box too short")
	}
	count := binary.BigEndian.Uint32(data[4:8])
	width := uint64(4)
	if b.typ == "co64" {
		width = 8
	}
	if uint64(8)+uint64(count)*width > uint64(len(data)) {
		return nil, errors.New("truncated chunk offsets")
	}
	out := make([]int64, count)
	for i := range out {
		pos := 8 + i*int(width)
		if width == 4 {
			out[i] = int64(binary.BigEndian.Uint32(data[pos : pos+4]))
		} else {
			u := binary.BigEndian.Uint64(data[pos : pos+8])
			if u > math.MaxInt64 {
				return nil, errors.New("co64 offset overflow")
			}
			out[i] = int64(u)
		}
	}
	return out, nil
}

func parseSTSS(r io.ReaderAt, b mp4Box) (map[int]bool, error) {
	data, err := readAtFull(r, b.payload, b.end-b.payload)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, errors.New("stss too short")
	}
	count := binary.BigEndian.Uint32(data[4:8])
	if uint64(8)+uint64(count)*4 > uint64(len(data)) {
		return nil, errors.New("truncated stss")
	}
	out := make(map[int]bool, count)
	for i := uint32(0); i < count; i++ {
		n := binary.BigEndian.Uint32(data[8+i*4 : 12+i*4])
		if n > 0 {
			out[int(n-1)] = true
		}
	}
	return out, nil
}

func mapMP4Chunks(st *mp4SampleTable, sampleCount int) ([]int64, error) {
	if len(st.chunks) == 0 {
		if sampleCount == 0 {
			return nil, nil
		}
		return nil, errors.New("stbl has samples but no stco/co64 chunk offsets")
	}
	if len(st.stsc) == 0 {
		return nil, errors.New("stbl has samples but no stsc")
	}
	out := make([]int64, 0, sampleCount)
	entry := 0
	for chunk := 1; chunk <= len(st.chunks); chunk++ {
		for entry+1 < len(st.stsc) && int(st.stsc[entry+1].FirstChunk) <= chunk {
			entry++
		}
		spc := int(st.stsc[entry].SamplesPerChunk)
		off := st.chunks[chunk-1]
		for j := 0; j < spc && len(out) < sampleCount; j++ {
			out = append(out, off)
			if len(out) > 0 {
				idx := len(out) - 1
				off += st.sizes[idx]
			}
		}
	}
	return out, nil
}

func hashMP4Samples(r io.ReaderAt, offsets []int64, t *Track) error {
	for i := range t.Samples {
		s := &t.Samples[i]
		if s.Size > 0 {
			sec := io.NewSectionReader(r, offsets[i], s.Size)
			h := sha256.New()
			if _, err := io.Copy(h, sec); err != nil {
				return fmt.Errorf("sample %d: %w", i, err)
			}
			sum := h.Sum(nil)
			s.Hash = binary.BigEndian.Uint64(sum[:8])
		}
		if t.Kind == KindSubtitle && s.Size > 0 {
			head, err := readAtFull(r, offsets[i], min64(s.Size, 64))
			if err != nil {
				return err
			}
			emptySubtitleSample(t.Codec, head, s)
		}
	}
	return nil
}

func emptySubtitleSample(codec string, data []byte, s *Sample) {
	switch codec {
	case "tx3g", "text":
		if len(data) >= 2 && binary.BigEndian.Uint16(data[:2]) == 0 {
			s.Empty = true
		}
	default:
		if len(data) == 0 {
			s.Empty = true
		}
	}
}

func ensureTable(st *mp4SampleTable) (*mp4SampleTable, error) {
	if st == nil {
		return &mp4SampleTable{}, nil
	}
	return st, nil
}

func inferDurations(total uint64, count int, timescale uint64) []int64 {
	out := make([]int64, count)
	if count == 0 {
		return out
	}
	d := int64(total) / int64(count)
	if d <= 0 && timescale > 0 {
		d = 1
	}
	for i := range out {
		out[i] = d
	}
	return out
}
