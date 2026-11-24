package audit

import (
	"encoding/binary"
	"testing"

	"ffv1mkaudit/ffv1"
)

func ebmlElement(id []byte, payload ...[]byte) []byte {
	joined := []byte{}
	for _, part := range payload {
		joined = append(joined, part...)
	}
	size := uint64(0x0100000000000000) | uint64(len(joined))
	sizeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sizeBytes, size)
	out := append(append([]byte{}, id...), sizeBytes...)
	return append(out, joined...)
}

func ebmlUint(id []byte, value uint64) []byte {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	size := 8
	for size > 1 && raw[8-size] == 0 {
		size--
	}
	return ebmlElement(id, raw[8-size:])
}

func syntheticFFV1Frame(slices int) []byte {
	frame := []byte{}
	for i := 0; i < slices; i++ {
		body := []byte{byte('a' + i), byte(i + 1), 0x80}
		if i == 0 {
			body = append([]byte{0xFC, 0x00}, body...)
		}
		sizeField := len(body)
		footer := []byte{byte(sizeField >> 16), byte(sizeField >> 8), byte(sizeField), 0}
		prefix := append(append([]byte{}, body...), footer...)
		crc := ffv1.CRC32Raw(prefix)
		slice := append(prefix, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
		frame = append(frame, slice...)
	}
	return frame
}

func TestSyntheticMatroskaUnknownInsideSegmentAndBlockGroupKey(t *testing.T) {
	info := ebmlElement([]byte{0x15, 0x49, 0xA9, 0x66},
		ebmlUint([]byte{0x2A, 0xD7, 0xB1}, 1_000_000))
	codec := ebmlElement([]byte{0x86}, []byte("V_FFV1"))
	trackEntry := ebmlElement([]byte{0xAE},
		ebmlUint([]byte{0xD7}, 1),
		ebmlUint([]byte{0x83}, 1),
		codec)
	tracks := ebmlElement([]byte{0x16, 0x54, 0xAE, 0x6B}, trackEntry)

	frameOne := syntheticFFV1Frame(3)
	simplePayload := append([]byte{0x81, 0x00, 0x00, 0x80}, frameOne...)
	simple := ebmlElement([]byte{0xA3}, simplePayload)

	frameTwo := syntheticFFV1Frame(3)
	blockPayload := append([]byte{0x81, 0x00, 0x01, 0x00}, frameTwo...)
	block := ebmlElement([]byte{0xA1}, blockPayload)
	blockGroup := ebmlElement([]byte{0xA0}, block)

	cluster := ebmlElement([]byte{0x1F, 0x43, 0xB6, 0x75},
		ebmlUint([]byte{0xE7}, 0), simple, blockGroup)
	unknown := ebmlElement([]byte{0xDE}, []byte("hidden-extension"))
	segment := ebmlElement([]byte{0x18, 0x53, 0x80, 0x67},
		info, tracks, cluster, unknown)
	header := ebmlElement([]byte{0x1A, 0x45, 0xDF, 0xA3},
		ebmlElement([]byte{0x42, 0x82}, []byte("matroska")))
	doc := append(header, segment...)

	report := analyzeFixtureBytes(t, doc, 3)
	if len(report.Frames) != 2 {
		t.Fatalf("frames=%d issues=%+v", len(report.Frames), report.Issues)
	}
	if !report.Frames[0].Keyframe || report.Frames[0].KeyframeSource != "simpleblock_keyframe_flag" {
		t.Fatalf("simple block key flag wrong: %+v", report.Frames[0])
	}
	if !report.Frames[1].Keyframe || report.Frames[1].KeyframeSource != "blockgroup_without_referenceblock" {
		t.Fatalf("block group should be keyframe without ReferenceBlock: %+v", report.Frames[1])
	}
	for i, frame := range report.Frames {
		if frame.SliceCount != 3 || frame.CRCOK == nil || !*frame.CRCOK {
			t.Fatalf("frame %d slices=%d crc=%v issues=%+v", i, frame.SliceCount, frame.CRCOK, frame.Issues)
		}
	}
	if len(report.UnknownElements) != 1 || report.UnknownElements[0].ParentID != "0x18538067" {
		t.Fatalf("unknown element not recorded under Segment: %+v", report.UnknownElements)
	}
	if report.Summary.Errors != 0 {
		t.Fatalf("synthetic file has errors: %+v / frame issues %+v", report.Issues, report.Frames)
	}
}
