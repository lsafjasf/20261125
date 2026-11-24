package main

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"ffv1mkaudit/audit"
)

func expectationsFrom(report *audit.Report) audit.Expectations {
	expected := audit.Expectations{ExpectedSlices: 4}
	for _, frame := range report.Frames {
		index := frame.Index
		keyframe := frame.Keyframe
		expected.Frames = append(expected.Frames, audit.FrameExpectation{
			Index:    &index,
			SHA256:   frame.SHA256,
			Size:     frame.Size,
			Position: frame.Position,
			Keyframe: &keyframe,
			Slices:   frame.SliceCount,
		})
	}
	return expected
}

func summarize(data []byte, injection *Injection) error {
	report, err := audit.AnalyzeReaderAt(bytes.NewReader(data), int64(len(data)), audit.Options{ExpectedSlices: 4})
	if err != nil {
		return err
	}
	injection.Frames = report.Summary.Frames
	injection.Errors = report.Summary.Errors
	if injection.UnknownElements == 0 {
		injection.UnknownElements = report.Summary.UnknownElements
	}
	return nil
}

func makeUnknownInjection(clean []byte, report *audit.Report) ([]byte, Injection) {
	data := append([]byte(nil), clean...)
	segmentID := []byte{0x18, 0x53, 0x80, 0x67}
	segmentOffset := bytes.Index(data, segmentID)
	custom := append([]byte{0x1F, 0xC0, 0xFF, 0xEE, 0x88}, []byte("UNKNOWN!")...)
	if segmentOffset >= 0 && data[segmentOffset+4] == 0x01 {
		sizeOffset := segmentOffset + 4
		sizeVINT := binary.BigEndian.Uint64(data[sizeOffset : sizeOffset+8])
		binary.BigEndian.PutUint64(data[sizeOffset:sizeOffset+8], sizeVINT+uint64(len(custom)))
	}
	data = append(data, custom...)
	injection := Injection{
		Kind:            "unknown_matroska_element",
		Description:     "Segment 末尾加入合法但未注册的 EBML ID 0x1FC0FFEE；必须记录且继续审计",
		UnknownElements: 1,
	}
	_ = report
	return data, injection
}

func makeCRCInjection(clean []byte, report *audit.Report) ([]byte, Injection, error) {
	frameIndex := 3
	if len(report.Frames) <= frameIndex || len(report.Frames[frameIndex].Slices) < 2 {
		frameIndex = 0
		for i, frame := range report.Frames {
			if len(frame.Slices) >= 2 {
				frameIndex = i
				break
			}
		}
	}
	if len(report.Frames) == 0 || len(report.Frames[frameIndex].Slices) < 2 {
		return nil, Injection{}, fmt.Errorf("fixture has no frame with at least two slices")
	}
	frame := report.Frames[frameIndex]
	slice := frame.Slices[1]
	flipOffset := slice.Offset + 12
	if flipOffset >= slice.EndOffset {
		flipOffset = slice.Offset + 1
	}

	data := append([]byte(nil), clean...)
	data[flipOffset] ^= 0x01

	recoverable := []int{}
	for _, candidate := range frame.Slices {
		if candidate.Index == slice.Index {
			continue
		}
		recoverable = append(recoverable, candidate.Index)
	}
	injection := Injection{
		Kind:              "ffv1_slice_crc_mismatch",
		Description:       "在第 4 帧第二个 slice 的压缩负载中翻转一个比特；FFV1 CRC 应定位该 slice",
		CorruptFrame:      frame.Index,
		ElementOffset:     frame.ElementOffset,
		CorruptOffset:     slice.Offset,
		RecoverableSlices: recoverable,
		RecoverableRanges: [][2]int{{0, frame.Index - 1}, {frame.Index + 1, len(report.Frames) - 1}},
	}
	return data, injection, nil
}

func makeTruncatedInjection(clean []byte, report *audit.Report) ([]byte, Injection, error) {
	frameIndex := 4
	if len(report.Frames) <= frameIndex {
		frameIndex = len(report.Frames) - 1
	}
	if frameIndex < 0 {
		return nil, Injection{}, fmt.Errorf("fixture has no frames")
	}
	frame := report.Frames[frameIndex]
	cut := frame.ElementOffset + 120
	if cut >= int64(len(clean)) || cut <= frame.ElementOffset {
		cut = int64(len(clean)) - 1
	}
	injection := Injection{
		Kind:          "truncated_block",
		Description:   "截断第 5 个 block 的负载；之前的帧仍应可恢复",
		CorruptFrame:  frame.Index,
		ElementOffset: frame.ElementOffset,
		CorruptOffset: cut,
		RecoverableRanges: [][2]int{
			{0, frame.Index - 1},
		},
	}
	return append([]byte(nil), clean[:cut]...), injection, nil
}
