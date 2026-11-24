package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	maxSamples    = 10_000_000
	maxTableBytes = 64 << 20
)

type issue struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type track struct {
	ID            uint32
	Codec         string
	Timescale     uint32
	MediaDuration uint64
	SampleCount   int
	Durations     []uint32
	Offsets       []int64
	OffsetPresent []bool
	Sync          []bool
	CTTSVersion   uint8
	CTTSPresent   bool
	STSSPresent   bool
	Issues        []issue
}

type movie struct {
	Fragmented bool
	Tracks     []*track
	Issues     []issue
}

type sttsEntry struct {
	count uint32
	delta uint32
}

type cttsEntry struct {
	count  uint32
	offset int64
}

type sampleTables struct {
	codec         string
	stsdEntries   uint32
	stts          []sttsEntry
	ctts          []cttsEntry
	cttsVersion   uint8
	cttsPresent   bool
	stss          []uint32
	stssPresent   bool
	sampleCount   uint32
	sampleCountOK bool
}

func parseFile(path string) (*movie, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}
	return parseMovie(f, info.Size())
}

func parseMovie(r io.ReaderAt, size int64) (*movie, error) {
	m := &movie{}
	seenMoov := false
	err := readBoxes(r, 0, size, 0, func(b box) error {
		switch b.typ {
		case "moov":
			seenMoov = true
			return parseMoov(r, b, m)
		case "moof":
			m.Fragmented = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !seenMoov {
		return nil, errors.New("no moov box found")
	}
	if m.Fragmented {
		m.Issues = append(m.Issues, issue{
			Code:   "fragmented_mp4",
			Detail: "moof boxes are present; timing from moof/traf/trun is not read",
		})
	}
	return m, nil
}

func parseMoov(r io.ReaderAt, b box, m *movie) error {
	return readBoxes(r, b.payloadPos, b.end, 1, func(child box) error {
		switch child.typ {
		case "trak":
			tr, err := parseTrack(r, child)
			if err != nil {
				return err
			}
			if tr != nil {
				m.Tracks = append(m.Tracks, tr)
			}
		case "mvex":
			m.Fragmented = true
		}
		return nil
	})
}

func parseTrack(r io.ReaderAt, b box) (*track, error) {
	tr := &track{}
	tables := &sampleTables{}
	handler := ""

	err := readBoxes(r, b.payloadPos, b.end, 2, func(child box) error {
		switch child.typ {
		case "tkhd":
			id, err := parseTrackID(r, child)
			if err != nil {
				return err
			}
			tr.ID = id
		case "mdia":
			var err error
			handler, err = parseMdia(r, child, tr, tables)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if handler != "vide" {
		return nil, nil
	}
	return buildTrack(tr, tables), nil
}

func parseMdia(r io.ReaderAt, b box, tr *track, tables *sampleTables) (string, error) {
	handler := ""
	err := readBoxes(r, b.payloadPos, b.end, 3, func(child box) error {
		switch child.typ {
		case "mdhd":
			return parseMDHD(r, child, tr)
		case "hdlr":
			var err error
			handler, err = parseHandler(r, child)
			return err
		case "minf":
			return readBoxes(r, child.payloadPos, child.end, 4, func(grandchild box) error {
				if grandchild.typ != "stbl" {
					return nil
				}
				return parseStbl(r, grandchild, tables)
			})
		}
		return nil
	})
	return handler, err
}

func parseTrackID(r io.ReaderAt, b box) (uint32, error) {
	data, err := readPayloadPrefix(r, b, 32)
	if err != nil {
		return 0, err
	}
	p := newPayloadReader(data)
	version, _, err := fullBox(p)
	if err != nil {
		return 0, err
	}
	skip := 8
	if version == 1 {
		skip = 16
	}
	if err := p.skip(skip); err != nil {
		return 0, err
	}
	return p.uint32()
}

func parseMDHD(r io.ReaderAt, b box, tr *track) error {
	data, err := readPayloadPrefix(r, b, 64)
	if err != nil {
		return err
	}
	p := newPayloadReader(data)
	version, _, err := fullBox(p)
	if err != nil {
		return err
	}
	if version == 1 {
		if err := p.skip(16); err != nil {
			return err
		}
		ts, err := p.uint32()
		if err != nil {
			return err
		}
		dur, err := p.uint64()
		if err != nil {
			return err
		}
		tr.Timescale, tr.MediaDuration = ts, dur
		return nil
	}
	if err := p.skip(8); err != nil {
		return err
	}
	ts, err := p.uint32()
	if err != nil {
		return err
	}
	dur, err := p.uint32()
	if err != nil {
		return err
	}
	tr.Timescale, tr.MediaDuration = ts, uint64(dur)
	return nil
}

func parseHandler(r io.ReaderAt, b box) (string, error) {
	data, err := readPayloadPrefix(r, b, 16)
	if err != nil {
		return "", err
	}
	p := newPayloadReader(data)
	if _, _, err := fullBox(p); err != nil {
		return "", err
	}
	if err := p.skip(4); err != nil {
		return "", err
	}
	return p.fourCC()
}

func parseStbl(r io.ReaderAt, b box, tables *sampleTables) error {
	return readBoxes(r, b.payloadPos, b.end, 5, func(child box) error {
		switch child.typ {
		case "stsd":
			return parseSTSD(r, child, tables)
		case "stts":
			return parseSTTS(r, child, tables)
		case "ctts":
			return parseCTTS(r, child, tables)
		case "stss":
			return parseSTSS(r, child, tables)
		case "stsz":
			return parseSTSZ(r, child, tables)
		case "stz2":
			return parseSTZ2(r, child, tables)
		}
		return nil
	})
}

func parseSTSD(r io.ReaderAt, b box, tables *sampleTables) error {
	data, err := readPayloadPrefix(r, b, 64)
	if err != nil {
		return err
	}
	p := newPayloadReader(data)
	if _, _, err := fullBox(p); err != nil {
		return err
	}
	count, err := p.uint32()
	if err != nil {
		return err
	}
	tables.stsdEntries = count
	if count == 0 {
		return nil
	}
	if err := p.skip(4); err != nil {
		return err
	}
	codec, err := p.fourCC()
	if err != nil {
		return err
	}
	tables.codec = codec
	return nil
}

func parseSTTS(r io.ReaderAt, b box, tables *sampleTables) error {
	data, err := readPayload(r, b, maxTableBytes)
	if err != nil {
		return err
	}
	p := newPayloadReader(data)
	if _, _, err := fullBox(p); err != nil {
		return err
	}
	count, err := p.uint32()
	if err != nil {
		return err
	}
	if uint64(count) > uint64(p.remaining()/8) {
		return io.ErrUnexpectedEOF
	}
	if count > maxSamples {
		return fmt.Errorf("stts entry count %d exceeds limit %d", count, maxSamples)
	}
	entries := make([]sttsEntry, count)
	for i := range entries {
		entries[i].count, err = p.uint32()
		if err != nil {
			return err
		}
		entries[i].delta, err = p.uint32()
		if err != nil {
			return err
		}
	}
	tables.stts = entries
	return nil
}

func parseCTTS(r io.ReaderAt, b box, tables *sampleTables) error {
	data, err := readPayload(r, b, maxTableBytes)
	if err != nil {
		return err
	}
	p := newPayloadReader(data)
	version, _, err := fullBox(p)
	if err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("unsupported ctts version %d", version)
	}
	count, err := p.uint32()
	if err != nil {
		return err
	}
	if uint64(count) > uint64(p.remaining()/8) {
		return io.ErrUnexpectedEOF
	}
	if count > maxSamples {
		return fmt.Errorf("ctts entry count %d exceeds limit %d", count, maxSamples)
	}
	entries := make([]cttsEntry, count)
	for i := range entries {
		entries[i].count, err = p.uint32()
		if err != nil {
			return err
		}
		raw, err := p.uint32()
		if err != nil {
			return err
		}
		if version == 0 {
			entries[i].offset = int64(raw)
		} else {
			entries[i].offset = int64(int32(raw))
		}
	}
	tables.ctts = entries
	tables.cttsVersion = version
	tables.cttsPresent = true
	return nil
}

func parseSTSS(r io.ReaderAt, b box, tables *sampleTables) error {
	data, err := readPayload(r, b, maxTableBytes)
	if err != nil {
		return err
	}
	p := newPayloadReader(data)
	if _, _, err := fullBox(p); err != nil {
		return err
	}
	count, err := p.uint32()
	if err != nil {
		return err
	}
	if uint64(count) > uint64(p.remaining()/4) {
		return io.ErrUnexpectedEOF
	}
	if count > maxSamples {
		return fmt.Errorf("stss entry count %d exceeds limit %d", count, maxSamples)
	}
	entries := make([]uint32, count)
	for i := range entries {
		entries[i], err = p.uint32()
		if err != nil {
			return err
		}
	}
	tables.stss = entries
	tables.stssPresent = true
	return nil
}

func parseSTSZ(r io.ReaderAt, b box, tables *sampleTables) error {
	data, err := readPayload(r, b, maxTableBytes)
	if err != nil {
		return err
	}
	p := newPayloadReader(data)
	if _, _, err := fullBox(p); err != nil {
		return err
	}
	sampleSize, err := p.uint32()
	if err != nil {
		return err
	}
	count, err := p.uint32()
	if err != nil {
		return err
	}
	if sampleSize == 0 && uint64(count) > uint64(p.remaining()/4) {
		return io.ErrUnexpectedEOF
	}
	tables.sampleCount = count
	tables.sampleCountOK = true
	return nil
}

func parseSTZ2(r io.ReaderAt, b box, tables *sampleTables) error {
	data, err := readPayload(r, b, maxTableBytes)
	if err != nil {
		return err
	}
	p := newPayloadReader(data)
	if _, _, err := fullBox(p); err != nil {
		return err
	}
	if err := p.skip(3); err != nil {
		return err
	}
	fieldSize, err := p.uint8()
	if err != nil {
		return err
	}
	count, err := p.uint32()
	if err != nil {
		return err
	}
	bits := uint64(count) * uint64(fieldSize)
	if fieldSize != 4 && fieldSize != 8 && fieldSize != 16 {
		return fmt.Errorf("invalid stz2 field size %d", fieldSize)
	}
	if bits > uint64(p.remaining())*8 || bits > maxTableBytes*8 {
		return io.ErrUnexpectedEOF
	}
	tables.sampleCount = count
	tables.sampleCountOK = true
	return nil
}

func buildTrack(tr *track, tables *sampleTables) *track {
	tr.Codec = tables.codec
	tr.CTTSPresent = tables.cttsPresent
	tr.CTTSVersion = tables.cttsVersion
	tr.STSSPresent = tables.stssPresent

	for _, e := range tables.stts {
		if e.count == 0 {
			continue
		}
		if uint64(len(tr.Durations))+uint64(e.count) > maxSamples {
			tr.Issues = append(tr.Issues, issue{
				Code:   "too_many_samples",
				Detail: fmt.Sprintf("stts expands beyond %d samples", maxSamples),
			})
			break
		}
		for range e.count {
			tr.Durations = append(tr.Durations, e.delta)
		}
	}
	tr.SampleCount = len(tr.Durations)

	if tables.sampleCountOK && tables.sampleCount != uint32(tr.SampleCount) {
		tr.Issues = append(tr.Issues, issue{
			Code: "sample_count_mismatch",
			Detail: fmt.Sprintf(
				"stsz/stz2 declares %d samples, stts expands to %d",
				tables.sampleCount, tr.SampleCount,
			),
		})
	}
	if len(tr.Durations) == 0 {
		tr.Issues = append(tr.Issues, issue{
			Code:   "no_samples",
			Detail: "no samples could be expanded from stts",
		})
	}

	tr.Offsets = make([]int64, tr.SampleCount)
	tr.OffsetPresent = make([]bool, tr.SampleCount)
	if tables.cttsPresent {
		pos := 0
		for _, e := range tables.ctts {
			for range e.count {
				if pos >= tr.SampleCount {
					break
				}
				tr.Offsets[pos] = e.offset
				tr.OffsetPresent[pos] = true
				pos++
			}
			if pos >= tr.SampleCount {
				break
			}
		}
		if pos != tr.SampleCount {
			tr.Issues = append(tr.Issues, issue{
				Code: "ctts_count_mismatch",
				Detail: fmt.Sprintf(
					"ctts covers %d samples, stts expands to %d",
					pos, tr.SampleCount,
				),
			})
		}
	} else {
		for i := range tr.OffsetPresent {
			tr.OffsetPresent[i] = true
		}
	}

	tr.Sync = make([]bool, tr.SampleCount)
	if !tables.stssPresent {
		for i := range tr.Sync {
			tr.Sync[i] = true
		}
	} else {
		prev := uint32(0)
		for _, sample := range tables.stss {
			if sample == 0 || int64(sample) > int64(tr.SampleCount) {
				tr.Issues = append(tr.Issues, issue{
					Code:   "stss_sample_out_of_range",
					Detail: fmt.Sprintf("stss references sample %d outside 1..%d", sample, tr.SampleCount),
				})
				continue
			}
			if sample <= prev {
				tr.Issues = append(tr.Issues, issue{
					Code:   "stss_not_strictly_increasing",
					Detail: fmt.Sprintf("stss sample %d follows %d", sample, prev),
				})
			}
			prev = sample
			tr.Sync[sample-1] = true
		}
	}

	if tr.Timescale == 0 {
		tr.Issues = append(tr.Issues, issue{
			Code:   "missing_timescale",
			Detail: "mdhd timescale is zero; time values remain in raw ticks",
		})
	}
	return tr
}

func (t *track) seconds(ticks int64) float64 {
	if t.Timescale == 0 {
		return 0
	}
	return float64(ticks) / float64(t.Timescale)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
