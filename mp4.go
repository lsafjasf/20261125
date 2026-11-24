package main

import (
	"fmt"
	"io"
	"sort"
)

const (
	maxTableBytes = 64 << 20
	maxSamples    = 10_000_000
)

type parsedTrack struct {
	info    TrackInfo
	samples []SampleHash
	chunks  []RangeHash
}

func buildMP4Manifest(m *Manifest, r io.ReaderAt, size int64, opts Options) error {
	roots, boxes, err := buildBoxTree(r, size)
	if err != nil {
		return err
	}
	m.Boxes = boxes

	trakNodes := findAllBoxes(roots, "trak")
	for _, trak := range trakNodes {
		track, warnings, err := parseMP4Track(r, size, trak, opts)
		for _, w := range warnings {
			m.Warnings = append(m.Warnings, w)
		}
		if err != nil {
			m.Warnings = append(m.Warnings, err.Error())
			continue
		}
		m.Tracks = append(m.Tracks, track.info)
		m.Samples = append(m.Samples, track.samples...)
		m.Chunks = append(m.Chunks, track.chunks...)
		m.AudioIntervals = append(m.AudioIntervals, buildMP4AudioIntervals(r, track, opts)...)
	}

	sort.SliceStable(m.Chunks, func(i, j int) bool {
		if m.Chunks[i].Start == m.Chunks[j].Start {
			return m.Chunks[i].Key < m.Chunks[j].Key
		}
		return m.Chunks[i].Start < m.Chunks[j].Start
	})
	return nil
}

func readFullBoxPayload(r io.ReaderAt, box *boxNode) ([]byte, error) {
	start := box.HeaderEnd + 4 // skip version + flags
	if start > box.End {
		return nil, fmt.Errorf("box %s is shorter than full-box header", box.Path)
	}
	n := box.End - start
	if n > maxTableBytes {
		return nil, fmt.Errorf("table box %s too large: %d bytes", box.Path, n)
	}
	buf := make([]byte, n)
	if n == 0 {
		return buf, nil
	}
	if _, err := r.ReadAt(buf, start); err != nil && err != io.EOF {
		return nil, fmt.Errorf("read box %s: %w", box.Path, err)
	}
	return buf, nil
}

func parseMP4Track(r io.ReaderAt, fileSize int64, trak *boxNode, opts Options) (*parsedTrack, []string, error) {
	var warnings []string
	info := TrackInfo{Kind: "unknown"}

	if tkhd := trak.child("tkhd"); tkhd != nil {
		if data, err := readFullBoxPayload(r, tkhd); err == nil && len(data) >= 4 {
			version := byte(0)
			if raw, err := readBoxPayload(r, tkhd); err == nil && len(raw) > 0 {
				version = raw[0]
			}
			if version == 1 && len(data) >= 20 {
				info.TrackID = beUint32(data, 16)
			} else if len(data) >= 12 {
				info.TrackID = beUint32(data, 8)
			}
		}
	}

	mdia := trak.child("mdia")
	if mdia != nil {
		if hdlr := mdia.child("hdlr"); hdlr != nil {
			if data, err := readFullBoxPayload(r, hdlr); err == nil && len(data) >= 8 {
				info.Handler = string(data[4:8])
				switch info.Handler {
				case "vide":
					info.Kind = "video"
				case "soun":
					info.Kind = "audio"
				case "hint":
					info.Kind = "hint"
				case "meta":
					info.Kind = "metadata"
				}
			}
		}
		if mdhd := mdia.child("mdhd"); mdhd != nil {
			if raw, err := readBoxPayload(r, mdhd); err == nil && len(raw) >= 1 {
				version := raw[0]
				data, derr := readFullBoxPayload(r, mdhd)
				if derr == nil {
					if version == 1 && len(data) >= 24 {
						info.Timescale = beUint32(data, 16)
						info.Duration = beUint64(data, 20)
					} else if version == 0 && len(data) >= 16 {
						info.Timescale = beUint32(data, 8)
						info.Duration = uint64(beUint32(data, 12))
					}
				}
			}
		}
	}
	if info.TrackID == 0 {
		return nil, warnings, fmt.Errorf("track at %s has no usable tkhd track ID", trak.Path)
	}

	stbl := trak.findFirstDescendant("stbl")
	if stbl == nil {
		return nil, warnings, fmt.Errorf("track %d has no stbl", info.TrackID)
	}

	sizes, uniformSize, declaredCount, err := parseSTSZ(r, stbl)
	if err != nil {
		return nil, warnings, err
	}
	count := declaredCount
	if uniformSize > 0 {
		if count > maxSamples {
			return nil, warnings, fmt.Errorf("track %d sample count %d exceeds limit", info.TrackID, count)
		}
		sizes = make([]uint32, count)
		for i := range sizes {
			sizes[i] = uniformSize
		}
	}
	if count == 0 {
		warnings = append(warnings, fmt.Sprintf("track %d has zero media samples", info.TrackID))
	}

	offsets, err := parseChunkOffsets(r, stbl)
	if err != nil {
		return nil, warnings, err
	}
	runs, err := parseSTSC(r, stbl)
	if err != nil {
		return nil, warnings, err
	}

	samples, chunks, err := expandMP4Samples(r, fileSize, info, sizes, offsets, runs)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	if len(samples) != count {
		warnings = append(warnings, fmt.Sprintf("track %d stsz count %d but stsc/stco expanded %d", info.TrackID, count, len(samples)))
	}

	applySTTS(r, stbl, info.TrackID, samples, warnings)
	applyCTTS(r, stbl, samples, warnings)
	applySTSS(r, stbl, samples)
	assignDisplayOrder(samples)

	info.SampleCount = len(samples)
	info.ChunkCount = len(chunks)
	return &parsedTrack{info: info, samples: samples, chunks: chunks}, warnings, nil
}

func readBoxPayload(r io.ReaderAt, box *boxNode) ([]byte, error) {
	n := box.End - box.HeaderEnd
	if n > maxTableBytes {
		return nil, fmt.Errorf("box %s too large", box.Path)
	}
	buf := make([]byte, n)
	if n == 0 {
		return buf, nil
	}
	if _, err := r.ReadAt(buf, box.HeaderEnd); err != nil && err != io.EOF {
		return nil, err
	}
	return buf, nil
}

func (n *boxNode) findFirstDescendant(typ string) *boxNode {
	all := n.all(typ)
	if len(all) == 0 {
		return nil
	}
	return all[0]
}
