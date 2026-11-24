package main

import (
	"fmt"
	"strings"

	"ffv1mkaudit/audit"
)

func printHuman(r *audit.Report) {
	fmt.Printf("文件: %s (%d bytes)\n", r.Path, r.FileSize)
	if r.DocType != "" {
		fmt.Printf("容器: %s v%d, timecode scale=%dns\n", r.DocType, r.DocTypeVersion, r.TimecodeScaleNS)
	}
	fmt.Printf("帧: %d，关键帧: %d，正常: %d，异常: %d，未知元素: %d\n",
		r.Summary.Frames, r.Summary.Keyframes, r.Summary.OKFrames,
		r.Summary.CorruptFrames, r.Summary.UnknownElements)

	if len(r.UnknownElements) > 0 {
		fmt.Println("\n未知 Matroska 元素（已记录，未中断）:")
		for _, unknown := range r.UnknownElements {
			fmt.Printf("  %s offset=%d size=%d parent=%s\n",
				unknown.ID, unknown.Offset, unknown.DeclaredSize, unknown.ParentID)
		}
	}

	if len(r.Issues) > 0 {
		fmt.Println("\n容器/结构问题:")
		for _, issue := range r.Issues {
			fmt.Printf("  [%s] %s offset=%d\n", issue.Code, issue.Message, issue.Offset)
		}
	}

	fmt.Println("\n逐帧核对:")
	for _, frame := range r.Frames {
		flags := "P"
		if frame.Keyframe {
			flags = "K"
		}
		crc := "-"
		if frame.CRCOK != nil {
			if *frame.CRCOK {
				crc = "OK"
			} else {
				crc = "BAD"
			}
		}
		sliceCount := fmt.Sprintf("%d", frame.SliceCount)
		if frame.ExpectedSlices > 0 {
			sliceCount += "/" + fmt.Sprintf("%d", frame.ExpectedSlices)
		}
		fmt.Printf("  #%-3d %s pos=%d frame_off=%d size=%-5d slices=%-5s crc=%-3s %-9s sha256=%s\n",
			frame.FrameNumber, flags, frame.Position, frame.FrameOffset, frame.Size,
			sliceCount, crc, frame.Status, shortHash(frame.SHA256))
		for _, issue := range frame.Issues {
			fmt.Printf("      [%s] %s\n", issue.Code, issue.Message)
		}
	}

	if len(r.CorruptBlocks) > 0 {
		fmt.Println("\n损坏 block:")
		for _, block := range r.CorruptBlocks {
			frameLabel := "-"
			if block.FrameNumber > 0 {
				frameLabel = fmt.Sprintf("%d", block.FrameNumber)
			}
			fmt.Printf("  frame=%s element=%s offset=%d kind=%s range=%d-%d\n",
				frameLabel, block.ElementID, block.ElementOffset,
				block.Kind, block.CorruptOffset, block.CorruptEndOffset)
			if len(block.RecoverableSlices) > 0 {
				fmt.Printf("      同 block 内 CRC 正常 slice: %s\n", joinInts(block.RecoverableSlices))
			}
		}
	}

	if len(r.RecoverableRanges) > 0 {
		fmt.Println("\n可恢复帧范围:")
		for _, span := range r.RecoverableRanges {
			fmt.Printf("  frames %d-%d (%d 帧), byte range %d-%d\n",
				span.StartFrameNumber, span.EndFrameNumber, span.FrameCount,
				span.StartOffset, span.EndOffset)
		}
	}
}

func shortHash(hash string) string {
	if len(hash) <= 16 {
		return hash
	}
	return hash[:16]
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprintf("%d", value)
	}
	return strings.Join(parts, ",")
}
