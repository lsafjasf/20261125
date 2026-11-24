package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

type channelFingerprint struct {
	name       string
	nonZero    int
	min        int64
	max        int64
	sumSquares float64
	first      int64
	hash       string
}

type chunkEvidence struct {
	id     string
	offset int
	size   int
}

type evidence struct {
	fileSize       int
	riffSize       int
	riffSizeOK     bool
	chunks         []chunkEvidence
	format         formatInfo
	declaredNames  []string
	frameCount     int
	blockAlign     int
	bytesPerSample int
	fingerprints   []channelFingerprint
	dataHash       string
	fileHash       string
	cmapConsistent bool
	maskConsistent bool
}

func collectEvidence(w *wavFile) evidence {
	channels := int(w.format.channels)
	bytesPerSample := int(w.format.bitsPerSample) / 8
	fingerprints := make([]channelFingerprint, channels)
	names := w.declaredNames()
	for i := range fingerprints {
		fingerprints[i] = channelFingerprint{
			name: names[i],
			min:  1<<63 - 1,
			max:  -1 << 63,
			hash: hashChannel(w, i),
		}
	}

	frames := w.frameCount()
	dataStart := w.data.offset + 8
	for frame := 0; frame < frames; frame++ {
		for channel := 0; channel < channels; channel++ {
			offset := dataStart + (frame*channels+channel)*bytesPerSample
			value := readPCM(w.raw[offset : offset+bytesPerSample])
			fp := &fingerprints[channel]
			if value != 0 {
				fp.nonZero++
			}
			if value < fp.min {
				fp.min = value
			}
			if value > fp.max {
				fp.max = value
			}
			floatValue := float64(value)
			fp.sumSquares += floatValue * floatValue
			if frame == 0 {
				fp.first = value
			}
		}
	}

	chunks := make([]chunkEvidence, 0, len(w.chunks))
	for _, c := range w.chunks {
		chunks = append(chunks, chunkEvidence{id: c.id, offset: c.offset, size: c.size})
	}

	declared := w.declaredNames()
	return evidence{
		fileSize:       len(w.raw),
		riffSize:       w.riffSize,
		riffSizeOK:     w.riffSize == len(w.raw)-8,
		chunks:         chunks,
		format:         w.format,
		declaredNames:  declared,
		frameCount:     frames,
		blockAlign:     w.blockAlign(),
		bytesPerSample: bytesPerSample,
		fingerprints:   fingerprints,
		dataHash:       hashBytes(w.data.data),
		fileHash:       hashBytes(w.raw),
		cmapConsistent: w.cmap != nil && len(w.cmap.names) == channels,
		maskConsistent: !w.format.hasMask || len(maskNames(w.format.mask)) == channels,
	}
}

func readPCM(value []byte) int64 {
	switch len(value) {
	case 1:
		return int64(value[0]) - 128
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(value)))
	case 3:
		sample := uint32(value[0]) | uint32(value[1])<<8 | uint32(value[2])<<16
		if sample&0x800000 != 0 {
			sample |= 0xff000000
		}
		return int64(int32(sample))
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(value)))
	default:
		panic(fmt.Sprintf("unsupported sample width %d", len(value)))
	}
}

func hashChannel(w *wavFile, channel int) string {
	bytesPerSample := int(w.format.bitsPerSample) / 8
	channels := int(w.format.channels)
	dataStart := w.data.offset + 8
	hasher := sha256.New()
	for frame := 0; frame < w.frameCount(); frame++ {
		start := dataStart + (frame*channels+channel)*bytesPerSample
		hasher.Write(w.raw[start : start+bytesPerSample])
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
