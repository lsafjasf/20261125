package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

type repairMode string

const (
	modeSamples     repairMode = "samples"
	modeDeclaration repairMode = "declaration"
)

type repairRequest struct {
	mode    repairMode
	mapping []int
	names   []string
}

type repairResult struct {
	output      []byte
	mode        repairMode
	mapping     []int
	beforeData  string
	afterData   string
	beforeFile  string
	afterFile   string
	dataChanged bool
	fmtChanged  bool
	cmapChanged bool
	newNames    []string
	firstBefore []int64
	firstAfter  []int64
}

func repairWAV(w *wavFile, request repairRequest) (*repairResult, error) {
	channels := int(w.format.channels)
	if len(request.mapping) != channels {
		return nil, errors.New("mapping length does not match channel count")
	}
	if err := validatePermutation(request.mapping, channels); err != nil {
		return nil, err
	}
	if request.mode == modeDeclaration && len(request.names) != channels {
		return nil, errors.New("declaration names length does not match channel count")
	}

	before := collectEvidence(w)
	result := &repairResult{
		mode:        request.mode,
		mapping:     append([]int(nil), request.mapping...),
		beforeData:  before.dataHash,
		beforeFile:  before.fileHash,
		firstBefore: firstSamples(w),
	}

	var output []byte
	var err error
	switch request.mode {
	case modeSamples:
		output, err = reorderSamples(w, request.mapping)
	case modeDeclaration:
		if !isIdentity(request.mapping) {
			return nil, errors.New("declaration mode must not reorder samples; use --names to describe the existing sample order")
		}
		output, err = updateDeclaration(w, request.names)
	default:
		return nil, fmt.Errorf("unknown repair mode %q", request.mode)
	}
	if err != nil {
		return nil, err
	}

	afterReader := bytes.NewReader(output)
	afterWAV, err := parseWAV(afterReader)
	if err != nil {
		return nil, fmt.Errorf("repaired output failed validation: %w", err)
	}
	after := collectEvidence(afterWAV)
	result.output = output
	result.afterData = after.dataHash
	result.afterFile = after.fileHash
	result.dataChanged = result.afterData != result.beforeData
	result.newNames = after.declaredNames
	result.firstAfter = firstSamples(afterWAV)
	result.fmtChanged = !bytes.Equal(w.fmtChunk().data, afterWAV.fmtChunk().data)
	result.cmapChanged = declarationPayloadChanged(w, afterWAV)

	if request.mode == modeSamples && !result.dataChanged {
		return nil, errors.New("samples mode requested but PCM payload did not change")
	}
	if request.mode == modeDeclaration && result.dataChanged {
		return nil, errors.New("declaration mode must not change PCM payload")
	}
	if result.fmtChanged {
		return nil, errors.New("repair must not change fmt payload")
	}
	if request.mode == modeSamples && result.cmapChanged {
		return nil, errors.New("samples mode must not change channel declaration")
	}
	if request.mode == modeSamples && !nonDataChunksEqual(w, afterWAV) {
		return nil, errors.New("samples mode must not change non-data chunks")
	}
	if request.mode == modeDeclaration && !result.cmapChanged {
		return nil, errors.New("declaration mode did not change channel declaration")
	}
	if request.mode == modeDeclaration && !declarationOnlyChanged(w, afterWAV) {
		return nil, errors.New("declaration mode changed a chunk other than cmap")
	}
	return result, nil
}

func nonDataChunksEqual(before, after *wavFile) bool {
	if len(before.chunks) != len(after.chunks) {
		return false
	}
	for i := range before.chunks {
		left := before.chunks[i]
		right := after.chunks[i]
		if left.id != right.id || left.offset != right.offset || left.size != right.size ||
			(left.id != dataID && !bytes.Equal(left.data, right.data)) {
			return false
		}
	}
	return true
}

func declarationOnlyChanged(before, after *wavFile) bool {
	if before.cmap == nil {
		if len(after.chunks) != len(before.chunks)+1 {
			return false
		}
	} else if len(before.chunks) != len(after.chunks) {
		return false
	}
	beforeIndex := 0
	for i := range after.chunks {
		right := after.chunks[i]
		if before.cmap == nil && right.id == cmapID {
			continue
		}
		if beforeIndex >= len(before.chunks) {
			return false
		}
		left := before.chunks[beforeIndex]
		beforeIndex++
		if left.id != right.id {
			return false
		}
		if left.id == dataID || left.id == cmapID {
			continue
		}
		if left.offset != right.offset || left.size != right.size || !bytes.Equal(left.data, right.data) {
			return false
		}
	}
	return true
}

func validatePermutation(mapping []int, channels int) error {
	seen := make([]bool, channels)
	for _, value := range mapping {
		if value < 0 || value >= channels {
			return fmt.Errorf("channel position %d is outside 1..%d", value+1, channels)
		}
		if seen[value] {
			return fmt.Errorf("source position %d is repeated", value+1)
		}
		seen[value] = true
	}
	return nil
}

func reorderSamples(w *wavFile, mapping []int) ([]byte, error) {
	if isIdentity(mapping) {
		return nil, errors.New("mapping is identity; no repair needed")
	}
	channels := int(w.format.channels)
	bytesPerSample := int(w.format.bitsPerSample) / 8
	frameSize := channels * bytesPerSample
	frames := w.frameCount()
	output := append([]byte(nil), w.raw...)
	dataStart := w.data.offset + 8
	temp := make([]byte, frameSize)
	for frame := 0; frame < frames; frame++ {
		frameStart := dataStart + frame*frameSize
		for outputChannel, sourceChannel := range mapping {
			sourceStart := frameStart + sourceChannel*bytesPerSample
			copy(temp[outputChannel*bytesPerSample:(outputChannel+1)*bytesPerSample],
				w.raw[sourceStart:sourceStart+bytesPerSample])
		}
		copy(output[frameStart:frameStart+frameSize], temp)
	}
	return output, nil
}

func updateDeclaration(w *wavFile, names []string) ([]byte, error) {
	if w.cmap != nil && equalStrings(w.cmap.names, names) {
		return nil, errors.New("declaration already matches requested names")
	}
	if len(names) > 65535 {
		return nil, errors.New("too many channel names")
	}
	payload := make([]byte, 4+16*len(names))
	binary.LittleEndian.PutUint32(payload[0:4], uint32(len(names)))
	for i, name := range names {
		if len(name) > 16 {
			return nil, fmt.Errorf("channel name %q exceeds 16 bytes", name)
		}
		copy(payload[4+i*16:4+(i+1)*16], name)
	}

	var prefix []byte
	var suffix []byte
	if w.cmap == nil {
		insertAt := w.data.offset
		prefix = append([]byte(nil), w.raw[:insertAt]...)
		suffix = append([]byte(nil), w.raw[insertAt:]...)
	} else {
		start := w.cmap.offset
		end := start + 8 + w.cmap.size + w.cmap.size%2
		prefix = append([]byte(nil), w.raw[:start]...)
		suffix = append([]byte(nil), w.raw[end:]...)
	}

	chunk := bytes.NewBuffer(nil)
	chunk.WriteString(cmapID)
	binary.Write(chunk, binary.LittleEndian, uint32(len(payload)))
	chunk.Write(payload)
	if len(payload)%2 == 1 {
		chunk.WriteByte(0)
	}

	output := make([]byte, 0, len(prefix)+chunk.Len()+len(suffix)+8)
	output = append(output, prefix...)
	output = append(output, chunk.Bytes()...)
	output = append(output, suffix...)
	binary.LittleEndian.PutUint32(output[4:8], uint32(len(output)-8))
	return output, nil
}

func declarationPayloadChanged(before, after *wavFile) bool {
	if (before.cmap == nil) != (after.cmap == nil) {
		return true
	}
	if before.cmap == nil {
		return false
	}
	return !equalStrings(before.cmap.names, after.cmap.names)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func firstSamples(w *wavFile) []int64 {
	channels := int(w.format.channels)
	bytesPerSample := int(w.format.bitsPerSample) / 8
	dataStart := w.data.offset + 8
	values := make([]int64, channels)
	for channel := 0; channel < channels; channel++ {
		start := dataStart + channel*bytesPerSample
		values[channel] = readPCM(w.raw[start : start+bytesPerSample])
	}
	return values
}
