package mp4diag

import (
	"encoding/binary"
	"fmt"
	"io"
)

const maxAnalyzedSamples = 1_000_000

type stscEntry struct {
	FirstChunk             uint32
	SamplesPerChunk        uint32
	SampleDescriptionIndex uint32
}

type parsedTables struct {
	trackID       uint32
	codec         string
	nalLengthSize int
	durations     []uint32
	ctsOffsets    []int64
	syncSamples   map[int]bool
	stsc          []stscEntry
	chunkOffsets  []int64
	sizes         []uint32
	count         uint32
	stblPath      string
}

func buildSamples(r io.ReaderAt, size int64, boxes []BoxInfo, findings *[]Finding) []SampleInfo {
	stblPath, trakPath, trackID := findVideoSTBL(r, boxes, findings)
	if stblPath == "" {
		*findings = append(*findings, Finding{
			Layer: LayerSampleTable, Severity: SeverityError, Code: "video_sample_table_not_found",
			Message: "no vide handler with an stbl box was found",
		})
		return nil
	}
	tables := parseSTBL(r, boxes, stblPath, findings)
	tables.trackID = trackID
	if tables.count == 0 {
		return nil
	}

	samples := make([]SampleInfo, 0, minInt(int(tables.count), maxAnalyzedSamples))
	sampleNo := 0
	for chunkNo0, chunkOffset := range tables.chunkOffsets {
		samplesPerChunk := samplesPerChunkFor(tables.stsc, chunkNo0+1)
		cursor := chunkOffset
		for withinChunk := uint32(0); withinChunk < samplesPerChunk && sampleNo < int(tables.count); withinChunk++ {
			sampleNo++
			sz := uint32(0)
			if sampleNo-1 < len(tables.sizes) {
				sz = tables.sizes[sampleNo-1]
			}
			info := SampleInfo{
				Number: sampleNo, TrackID: tables.trackID, Chunk: chunkNo0 + 1,
				Offset: cursor, Size: sz, Sync: tables.syncSamples == nil || tables.syncSamples[sampleNo],
				NALLengthSize: tables.nalLengthSize,
			}
			info.Available = sz > 0 && cursor >= 0 && cursor+int64(sz) <= size
			if cursor < 0 || cursor+int64(sz) > size {
				info.Available = false
			}
			if sampleNo-1 < len(tables.durations) {
				info.DTS = cumulativeDTS(tables.durations, sampleNo-1)
			}
			if sampleNo-1 < len(tables.ctsOffsets) {
				info.CTSOffset = tables.ctsOffsets[sampleNo-1]
			}
			info.PTS = info.DTS + info.CTSOffset
			if info.Available && tables.nalLengthSize > 0 && (tables.codec == "avc1" || tables.codec == "avc3") {
				payload, err := io.ReadAll(io.NewSectionReader(r, info.Offset, int64(info.Size)))
				if err == nil {
					info.NALTypes, info.ReferenceStatus = parseNALInfo(payload, tables.nalLengthSize)
				}
			}
			samples = append(samples, info)
			cursor += int64(sz)
		}
	}

	if sampleNo != int(tables.count) {
		loc := locatorForPath(boxes, stblPath+"/stsc", "entries", 0)
		*findings = append(*findings, Finding{
			Layer: LayerSampleTable, Severity: SeverityError, Code: "stsc_sample_count_mismatch",
			Message: fmt.Sprintf("stsc/chunk offsets address %d samples but stsz declares %d", sampleNo, tables.count),
			Box:     loc, Samples: []int{sampleNo + 1}, PixelDamage: PixelNotAssessed,
		})
	}
	assignDisplayOrder(samples)
	annotateMDATMembership(boxes, samples)
	validateMappedBounds(size, tables, samples, boxes, findings)
	_ = trakPath
	return samples
}

func findVideoSTBL(r io.ReaderAt, boxes []BoxInfo, findings *[]Finding) (string, string, uint32) {
	for _, hdlr := range boxes {
		if hdlr.Type != "hdlr" || !hdlr.Valid {
			continue
		}
		payload, err := readPayload(r, hdlr)
		if err != nil || len(payload) < 12 {
			continue
		}
		handler := string(payload[8:12])
		if handler != "vide" {
			continue
		}
		mdiaPath := parentPath(hdlr.Path)
		trakPath := parentPath(mdiaPath)
		stblPath := mdiaPath + "/minf/stbl"
		if boxByPath(boxes, stblPath) != nil {
			return stblPath, trakPath, parseTrackID(r, boxes, trakPath)
		}
	}
	return "", "", 0
}

func parseTrackID(r io.ReaderAt, boxes []BoxInfo, trakPath string) uint32 {
	tkhd := boxByPath(boxes, trakPath+"/tkhd")
	if tkhd == nil {
		return 1
	}
	payload, err := readPayload(r, *tkhd)
	if err != nil || len(payload) < 20 {
		return 1
	}
	version := payload[0]
	if version == 1 && len(payload) >= 28 {
		return binary.BigEndian.Uint32(payload[20:24])
	}
	return binary.BigEndian.Uint32(payload[12:16])
}

func parseSTBL(r io.ReaderAt, boxes []BoxInfo, stblPath string, findings *[]Finding) parsedTables {
	t := parsedTables{stblPath: stblPath, nalLengthSize: 4}
	if stsd := boxByPath(boxes, stblPath+"/stsd"); stsd != nil {
		codec, nalSize := parseSTSD(r, *stsd)
		t.codec, t.nalLengthSize = codec, nalSize
	}
	if stts := boxByPath(boxes, stblPath+"/stts"); stts != nil {
		t.durations = expandSTTS(r, *stts, stblPath, findings)
	}
	if ctts := boxByPath(boxes, stblPath+"/ctts"); ctts != nil {
		t.ctsOffsets = expandCTTS(r, *ctts, findings)
	}
	if stss := boxByPath(boxes, stblPath+"/stss"); stss != nil {
		t.syncSamples = parseSTSS(r, *stss)
	}
	if stsc := boxByPath(boxes, stblPath+"/stsc"); stsc != nil {
		t.stsc = parseSTSC(r, *stsc)
	}
	if stco := boxByPath(boxes, stblPath+"/stco"); stco != nil {
		t.chunkOffsets = parseSTCO32(r, *stco)
	} else if co64 := boxByPath(boxes, stblPath+"/co64"); co64 != nil {
		t.chunkOffsets = parseCO64(r, *co64)
	}
	if stsz := boxByPath(boxes, stblPath+"/stsz"); stsz != nil {
		t.sizes, t.count = parseSTSZ(r, *stsz, stblPath, findings)
	}
	return t
}

func boxByPath(boxes []BoxInfo, path string) *BoxInfo {
	for i := range boxes {
		if boxes[i].Path == path {
			return &boxes[i]
		}
	}
	return nil
}

func parentPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func samplesPerChunkFor(entries []stscEntry, chunk int) uint32 {
	result := uint32(0)
	for _, entry := range entries {
		if int(entry.FirstChunk) <= chunk {
			result = entry.SamplesPerChunk
		} else {
			break
		}
	}
	return result
}

func cumulativeDTS(durations []uint32, index int) int64 {
	var total int64
	for i := 0; i < index && i < len(durations); i++ {
		total += int64(durations[i])
	}
	return total
}

func assignDisplayOrder(samples []SampleInfo) {
	order := make([]int, len(samples))
	for i := range order {
		order[i] = i
	}
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && samples[order[j-1]].PTS > samples[order[j]].PTS; j-- {
			order[j-1], order[j] = order[j], order[j-1]
		}
	}
	for displayPos, sampleIndex := range order {
		samples[sampleIndex].DisplayOrder = displayPos + 1
	}
}
