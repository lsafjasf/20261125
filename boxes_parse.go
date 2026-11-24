package mp4diag

import (
	"encoding/binary"
	"fmt"
	"io"
)

func parseSTSD(r io.ReaderAt, box BoxInfo) (string, int) {
	payload, err := readPayload(r, box)
	if err != nil || len(payload) < 16 {
		return "", 4
	}
	entryCount := binary.BigEndian.Uint32(payload[4:8])
	if entryCount < 1 || len(payload) < 16 {
		return "", 4
	}
	entry := payload[8:]
	if len(entry) < 8 {
		return "", 4
	}
	codec := string(entry[4:8])
	nalSize := 4
	if idx := indexBytes(entry, []byte("avcC")); idx >= 0 && idx+9 <= len(entry) {
		nalSize = int(entry[idx+8]&0x03) + 1
	}
	return codec, nalSize
}

func expandSTTS(r io.ReaderAt, box BoxInfo, stblPath string, findings *[]Finding) []uint32 {
	payload, err := readPayload(r, box)
	if err != nil {
		return nil
	}
	br := newByteReader(payload)
	if _, _, err := br.fullBox(); err != nil {
		return nil
	}
	count, err := br.u32()
	if err != nil {
		return nil
	}
	durations := make([]uint32, 0, 16)
	for i := uint32(0); i < count; i++ {
		repeat, err1 := br.u32()
		delta, err2 := br.u32()
		if err1 != nil || err2 != nil {
			*findings = append(*findings, parseFinding(box, "stts_truncated", "stts entry list is truncated"))
			return durations
		}
		for repeat > 0 && len(durations) < maxAnalyzedSamples {
			durations = append(durations, delta)
			repeat--
		}
	}
	return durations
}

func expandCTTS(r io.ReaderAt, box BoxInfo, findings *[]Finding) []int64 {
	payload, err := readPayload(r, box)
	if err != nil {
		return nil
	}
	version := payload[0]
	br := newByteReader(payload)
	_, _, _ = br.fullBox()
	count, err := br.u32()
	if err != nil {
		return nil
	}
	offsets := make([]int64, 0, 16)
	for i := uint32(0); i < count; i++ {
		repeat, err1 := br.u32()
		rawOffset, err2 := br.u32()
		if err1 != nil || err2 != nil {
			*findings = append(*findings, parseFinding(box, "ctts_truncated", "ctts entry list is truncated"))
			return offsets
		}
		offset := int64(rawOffset)
		if version == 1 {
			offset = int64(int32(rawOffset))
		}
		for repeat > 0 && len(offsets) < maxAnalyzedSamples {
			offsets = append(offsets, offset)
			repeat--
		}
	}
	return offsets
}

func parseSTSS(r io.ReaderAt, box BoxInfo) map[int]bool {
	payload, err := readPayload(r, box)
	if err != nil || len(payload) < 8 {
		return nil
	}
	count := binary.BigEndian.Uint32(payload[4:8])
	sync := make(map[int]bool, count)
	for i := uint32(0); i < count && 8+int(i)*4+4 <= len(payload); i++ {
		sample := binary.BigEndian.Uint32(payload[8+i*4 : 12+i*4])
		sync[int(sample)] = true
	}
	return sync
}

func parseSTSC(r io.ReaderAt, box BoxInfo) []stscEntry {
	payload, err := readPayload(r, box)
	if err != nil || len(payload) < 8 {
		return nil
	}
	count := binary.BigEndian.Uint32(payload[4:8])
	entries := make([]stscEntry, 0, count)
	pos := 8
	for i := uint32(0); i < count && pos+12 <= len(payload); i++ {
		entries = append(entries, stscEntry{
			FirstChunk:             binary.BigEndian.Uint32(payload[pos : pos+4]),
			SamplesPerChunk:        binary.BigEndian.Uint32(payload[pos+4 : pos+8]),
			SampleDescriptionIndex: binary.BigEndian.Uint32(payload[pos+8 : pos+12]),
		})
		pos += 12
	}
	return entries
}

func parseSTSZ(r io.ReaderAt, box BoxInfo, stblPath string, findings *[]Finding) ([]uint32, uint32) {
	payload, err := readPayload(r, box)
	if err != nil || len(payload) < 12 {
		*findings = append(*findings, parseFinding(box, "stsz_truncated", "stsz payload is too short"))
		return nil, 0
	}
	uniform := binary.BigEndian.Uint32(payload[4:8])
	count := binary.BigEndian.Uint32(payload[8:12])
	if count > maxAnalyzedSamples {
		*findings = append(*findings, Finding{
			Layer: LayerSampleTable, Severity: SeverityWarning, Code: "sample_count_capped",
			Message: fmt.Sprintf("stsz declares %d samples; analysis capped at %d", count, maxAnalyzedSamples),
			Box:     boxLocator(stblPath+"/stsz", "sample_count", 0), PixelDamage: PixelNotAssessed,
		})
	}
	analyzeCount := minUint32(count, maxAnalyzedSamples)
	if uniform != 0 {
		sizes := make([]uint32, analyzeCount)
		for i := range sizes {
			sizes[i] = uniform
		}
		return sizes, count
	}
	sizes := make([]uint32, 0, analyzeCount)
	for i := uint32(0); i < analyzeCount && 12+int(i)*4+4 <= len(payload); i++ {
		sizes = append(sizes, binary.BigEndian.Uint32(payload[12+i*4:16+i*4]))
	}
	if uint32(len(sizes)) != analyzeCount {
		*findings = append(*findings, parseFinding(box, "stsz_entries_truncated", "stsz entry list is shorter than sample_count"))
	}
	return sizes, count
}

func parseSTCO32(r io.ReaderAt, box BoxInfo) []int64 {
	payload, err := readPayload(r, box)
	if err != nil || len(payload) < 8 {
		return nil
	}
	count := binary.BigEndian.Uint32(payload[4:8])
	out := make([]int64, 0, count)
	for i := uint32(0); i < count && 8+int(i)*4+4 <= len(payload); i++ {
		out = append(out, int64(binary.BigEndian.Uint32(payload[8+i*4:12+i*4])))
	}
	return out
}

func parseCO64(r io.ReaderAt, box BoxInfo) []int64 {
	payload, err := readPayload(r, box)
	if err != nil || len(payload) < 8 {
		return nil
	}
	count := binary.BigEndian.Uint32(payload[4:8])
	out := make([]int64, 0, count)
	for i := uint32(0); i < count && 8+int(i)*8+8 <= len(payload); i++ {
		out = append(out, int64(binary.BigEndian.Uint64(payload[8+i*8:16+i*8])))
	}
	return out
}

func parseNALInfo(sample []byte, lengthSize int) ([]int, string) {
	if lengthSize != 1 && lengthSize != 2 && lengthSize != 4 {
		return nil, "unknown"
	}
	types := make([]int, 0, 4)
	sawNAL := false
	reference := false
	for pos := 0; pos+lengthSize <= len(sample); {
		var n int
		switch lengthSize {
		case 1:
			n = int(sample[pos])
		case 2:
			n = int(binary.BigEndian.Uint16(sample[pos : pos+2]))
		case 4:
			n = int(binary.BigEndian.Uint32(sample[pos : pos+4]))
		}
		pos += lengthSize
		if n <= 0 || pos+n > len(sample) {
			break
		}
		if pos < len(sample) {
			header := sample[pos]
			types = append(types, int(header&0x1f))
			sawNAL = true
			if (header>>5)&0x03 > 0 {
				reference = true
			}
		}
		pos += n
	}
	if !sawNAL {
		return types, "unknown"
	}
	if reference {
		return types, "reference"
	}
	return types, "non_reference"
}

func parseFinding(box BoxInfo, code, message string) Finding {
	return Finding{
		Layer: LayerSampleTable, Severity: SeverityError, Code: code, Message: message,
		Box: &BoxLocator{Path: box.Path, Type: box.Type, Offset: box.Offset}, PixelDamage: PixelNotAssessed,
	}
}

func boxLocator(path, field string, entry int) *BoxLocator {
	typ := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			typ = path[i+1:]
			break
		}
	}
	return &BoxLocator{Path: path, Type: typ, Field: field, Entry: entry}
}

func locatorForPath(boxes []BoxInfo, path, field string, entry int) *BoxLocator {
	loc := boxLocator(path, field, entry)
	if box := boxByPath(boxes, path); box != nil {
		loc.Offset = box.Offset
	}
	return loc
}

func indexBytes(haystack, needle []byte) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return i
		}
	}
	return -1
}

func minUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func annotateMDATMembership(_ []BoxInfo, _ []SampleInfo) {}

func validateMappedBounds(fileSize int64, tables parsedTables, samples []SampleInfo, boxes []BoxInfo, findings *[]Finding) {
	var unavailable []int
	for _, sample := range samples {
		if !sample.Available {
			unavailable = append(unavailable, sample.Number)
		}
	}
	if len(unavailable) == 0 {
		return
	}

	var truncatedMDAT *BoxInfo
	for i := range boxes {
		if boxes[i].Type == "mdat" && !boxes[i].Valid && boxes[i].IssueCode == "box_end_beyond_parent" {
			truncatedMDAT = &boxes[i]
			break
		}
	}
	if truncatedMDAT != nil {
		*findings = append(*findings, Finding{
			Layer: LayerContainer, Severity: SeverityError, Code: "samples_past_truncated_box",
			Message: fmt.Sprintf("%d sample(s) are addressed by stbl but lie beyond the truncated mdat/file boundary", len(unavailable)),
			Box:     &BoxLocator{Path: truncatedMDAT.Path, Type: truncatedMDAT.Type, Offset: truncatedMDAT.Offset},
			Samples: unavailable, Spans: consecutiveSpans(unavailable), PixelDamage: PixelNotAssessed,
		})
		return
	}

	loc := locatorForPath(boxes, tables.stblPath+"/stsz", "sample_size", 0)
	for _, offset := range tables.chunkOffsets {
		if offset < 0 || offset >= fileSize {
			if stco := boxByPath(boxes, tables.stblPath+"/stco"); stco != nil {
				loc = &BoxLocator{Path: stco.Path, Type: stco.Type, Offset: stco.Offset, Field: "chunk_offset"}
			}
			break
		}
	}
	*findings = append(*findings, Finding{
		Layer: LayerSampleTable, Severity: SeverityError, Code: "sample_range_outside_file",
		Message: fmt.Sprintf("%d sample(s) have stsc/stsz/stco-derived ranges outside the file", len(unavailable)),
		Box:     loc, Samples: unavailable, Spans: consecutiveSpans(unavailable), PixelDamage: PixelNotAssessed,
	})
}

func consecutiveSpans(values []int) []Span {
	if len(values) == 0 {
		return nil
	}
	spans := []Span{{First: values[0], Last: values[0]}}
	for _, value := range values[1:] {
		last := &spans[len(spans)-1]
		if value == last.Last+1 {
			last.Last = value
		} else {
			spans = append(spans, Span{First: value, Last: value})
		}
	}
	return spans
}
