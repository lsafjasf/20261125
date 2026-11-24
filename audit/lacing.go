package audit

import "fmt"

type sampleRange struct {
	Data          []byte
	PayloadOffset int
}

func splitLacedSamples(payload []byte, start int, lacing byte) ([]sampleRange, string) {
	if int(start) > len(payload) {
		return nil, "block data starts outside payload"
	}
	switch lacing {
	case 0:
		return []sampleRange{{Data: payload[start:], PayloadOffset: start}}, ""
	case 3:
		return splitFixedLacing(payload, int(start))
	case 1:
		return splitXiphLacing(payload, int(start))
	default:
		return nil, fmt.Sprintf("EBML lacing type %d is not decoded by this audit build", lacing)
	}
}

func splitFixedLacing(payload []byte, start int) ([]sampleRange, string) {
	if start >= len(payload) {
		return nil, "fixed lacing header is truncated"
	}
	count := int(payload[start]) + 1
	dataStart := start + 1
	remaining := len(payload) - dataStart
	if count <= 0 || remaining < 0 || remaining%count != 0 {
		return nil, "fixed lacing payload is not divisible by frame count"
	}
	size := remaining / count
	out := make([]sampleRange, 0, count)
	for i := 0; i < count; i++ {
		offset := dataStart + i*size
		out = append(out, sampleRange{Data: payload[offset : offset+size], PayloadOffset: offset})
	}
	return out, ""
}

func splitXiphLacing(payload []byte, start int) ([]sampleRange, string) {
	if start >= len(payload) {
		return nil, "Xiph lacing header is truncated"
	}
	frames := int(payload[start]) + 1
	offset := start + 1
	sizes := make([]int, 0, frames)
	for i := 0; i < frames-1; i++ {
		size := 0
		for offset < len(payload) && payload[offset] == 255 {
			size += 255
			offset++
		}
		if offset >= len(payload) {
			return nil, "Xiph lacing size table is truncated"
		}
		size += int(payload[offset])
		offset++
		sizes = append(sizes, size)
	}
	out := make([]sampleRange, 0, frames)
	for _, size := range sizes {
		if offset+size > len(payload) {
			return nil, "Xiph lacing frame size exceeds block payload"
		}
		out = append(out, sampleRange{Data: payload[offset : offset+size], PayloadOffset: offset})
		offset += size
	}
	out = append(out, sampleRange{Data: payload[offset:], PayloadOffset: offset})
	return out, ""
}
