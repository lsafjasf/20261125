package main

import (
	"fmt"
	"io"
	"sort"
)

func expandMP4Samples(r io.ReaderAt, fileSize int64, track TrackInfo, sizes []uint32, offsets []int64, runs []stscRun) ([]SampleHash, []RangeHash, error) {
	samples := make([]SampleHash, 0, len(sizes))
	chunks := make([]RangeHash, 0, len(offsets))
	if len(sizes) > 0 && (len(offsets) == 0 || len(runs) == 0) {
		return samples, chunks, fmt.Errorf("track %d has samples but no chunk mapping", track.TrackID)
	}

	sampleCursor := 0
	runIndex := 0
	for chunkZero := 0; chunkZero < len(offsets); chunkZero++ {
		chunkNumber := chunkZero + 1
		for runIndex+1 < len(runs) && runs[runIndex+1].FirstChunk <= chunkNumber {
			runIndex++
		}
		if runIndex >= len(runs) || runs[runIndex].FirstChunk > chunkNumber {
			return samples, chunks, fmt.Errorf("track %d has no stsc entry for chunk %d", track.TrackID, chunkNumber)
		}

		chunkStart := offsets[chunkZero]
		if chunkStart < 0 || chunkStart > fileSize {
			return samples, chunks, fmt.Errorf("track %d chunk %d has invalid offset %d", track.TrackID, chunkNumber, chunkStart)
		}
		cursor := chunkStart
		added := 0

		for added < runs[runIndex].SamplesPerChunk {
			if sampleCursor >= len(sizes) {
				return samples, chunks, fmt.Errorf("track %d stsc expands beyond stsz count", track.TrackID)
			}
			size := int64(sizes[sampleCursor])
			end := cursor + size
			if end < cursor || end > fileSize {
				return samples, chunks, fmt.Errorf("track %d sample %d range [%d,%d) exceeds file", track.TrackID, sampleCursor+1, cursor, end)
			}
			sum, err := hashRange(r, cursor, end)
			if err != nil {
				return samples, chunks, err
			}
			samples = append(samples, SampleHash{
				Key:        fmt.Sprintf("track/%d/sample/%d", track.TrackID, sampleCursor+1),
				TrackID:    track.TrackID,
				TrackKind:  track.Kind,
				Index:      sampleCursor + 1,
				ChunkIndex: chunkNumber,
				Offset:     cursor,
				Size:       size,
				SHA256:     sum,
				Sync:       track.Kind != "video",
			})
			cursor = end
			sampleCursor++
			added++
		}

		chunkHash, err := hashRange(r, chunkStart, cursor)
		if err != nil {
			return samples, chunks, err
		}
		chunks = append(chunks, RangeHash{
			Layer:  LayerMP4Chunk,
			Key:    fmt.Sprintf("track/%d/chunk/%d", track.TrackID, chunkNumber),
			Index:  chunkNumber,
			Start:  chunkStart,
			End:    cursor,
			Size:   cursor - chunkStart,
			SHA256: chunkHash,
		})
	}

	if sampleCursor != len(sizes) {
		return samples, chunks, fmt.Errorf("track %d mapped %d samples but stsz contains %d", track.TrackID, sampleCursor, len(sizes))
	}
	return samples, chunks, nil
}

func applySTTS(r io.ReaderAt, stbl *boxNode, trackID uint32, samples []SampleHash, warnings []string) {
	box := stbl.child("stts")
	if box == nil {
		return
	}
	data, err := readFullBoxPayload(r, box)
	if err != nil || len(data) < 4 {
		warnings = append(warnings, fmt.Sprintf("track %d: invalid stts", trackID))
		return
	}
	entries := int(beUint32(data, 0))
	pos := 4
	sampleIndex := 0
	var dts int64
	for e := 0; e < entries; e++ {
		if pos+8 > len(data) {
			warnings = append(warnings, fmt.Sprintf("track %d: truncated stts", trackID))
			return
		}
		count := int(beUint32(data, pos))
		delta := beUint32(data, pos+4)
		pos += 8
		for j := 0; j < count && sampleIndex < len(samples); j++ {
			samples[sampleIndex].DTS = dts
			samples[sampleIndex].PTS = dts
			samples[sampleIndex].Duration = delta
			dts += int64(delta)
			sampleIndex++
		}
	}
	if sampleIndex != len(samples) {
		warnings = append(warnings, fmt.Sprintf("track %d: stts covers %d of %d samples", trackID, sampleIndex, len(samples)))
	}
}

func applyCTTS(r io.ReaderAt, stbl *boxNode, samples []SampleHash, warnings []string) {
	box := stbl.child("ctts")
	if box == nil {
		return
	}
	data, err := readFullBoxPayload(r, box)
	if err != nil || len(data) < 4 {
		return
	}
	entries := int(beUint32(data, 0))
	pos := 4
	sampleIndex := 0
	for e := 0; e < entries; e++ {
		if pos+8 > len(data) {
			warnings = append(warnings, "truncated ctts")
			return
		}
		count := int(beUint32(data, pos))
		offset := int32(beUint32(data, pos+4))
		pos += 8
		for j := 0; j < count && sampleIndex < len(samples); j++ {
			samples[sampleIndex].PTS = samples[sampleIndex].DTS + int64(offset)
			sampleIndex++
		}
	}
}

func applySTSS(r io.ReaderAt, stbl *boxNode, samples []SampleHash) {
	box := stbl.child("stss")
	if box == nil {
		for i := range samples {
			if samples[i].TrackKind == "video" {
				samples[i].Sync = true
			}
		}
		return
	}
	data, err := readFullBoxPayload(r, box)
	if err != nil || len(data) < 4 {
		return
	}
	for i := range samples {
		samples[i].Sync = false
	}
	count := int(beUint32(data, 0))
	for i := 0; i < count; i++ {
		off := 4 + i*4
		if off+4 > len(data) {
			return
		}
		n := int(beUint32(data, off))
		if n >= 1 && n <= len(samples) {
			samples[n-1].Sync = true
		}
	}
}

func assignDisplayOrder(samples []SampleHash) {
	order := make([]int, len(samples))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		x, y := samples[order[a]], samples[order[b]]
		if x.PTS != y.PTS {
			return x.PTS < y.PTS
		}
		if x.DTS != y.DTS {
			return x.DTS < y.DTS
		}
		return x.Index < y.Index
	})
	for display, sampleZero := range order {
		samples[sampleZero].DisplayOrder = display + 1
	}
}
