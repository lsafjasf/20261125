package audit

import "ffv1mkaudit/ebml"

func (p *parser) buildCorruptBlocks() {
	for _, frame := range p.report.Frames {
		if frame.Status == "ok" && !frameHasError(&frame) {
			continue
		}
		block := CorruptBlock{
			FrameIndex:    frame.Index,
			FrameNumber:   frame.Index + 1,
			TrackNumber:   frame.TrackNumber,
			ElementID:     frame.BlockElementID,
			ElementOffset: frame.ElementOffset,
			PayloadOffset: frame.FrameOffset,
			Size:          frame.Size,
			Kind:          firstErrorCode(&frame),
			Detail:        firstErrorMessage(&frame),
		}
		minOffset := int64(0)
		maxEnd := int64(0)
		for _, issue := range frame.Issues {
			if issue.Severity != SeverityError || issue.Offset == 0 {
				continue
			}
			if minOffset == 0 || issue.Offset < minOffset {
				minOffset = issue.Offset
			}
			end := issue.EndOffset
			if end == 0 {
				end = issue.Offset + 1
			}
			if end > maxEnd {
				maxEnd = end
			}
		}
		block.CorruptOffset = minOffset
		block.CorruptEndOffset = maxEnd
		for _, slice := range frame.Slices {
			if slice.CRCOK != nil && *slice.CRCOK {
				block.RecoverableSlices = append(block.RecoverableSlices, slice.Index)
			}
		}
		p.report.CorruptBlocks = append(p.report.CorruptBlocks, block)
	}

	clusterID := ebml.HexIDPadded(ebml.IDCluster, 4)
	for _, check := range p.report.ContainerCRCs {
		if check.OK || check.ParentElementID != clusterID || p.hasCorruptFrameInCluster(check.ParentOffset) {
			continue
		}
		p.report.CorruptBlocks = append(p.report.CorruptBlocks, CorruptBlock{
			ElementID:     check.ParentElementID,
			ElementOffset: check.ParentOffset,
			PayloadOffset: check.CRCElementOffset,
			Kind:          "matroska_crc_mismatch",
			Detail:        "Cluster CRC-32 mismatch without a more precise FFV1 slice failure",
			CorruptOffset: check.CRCElementOffset,
		})
	}
}

func (p *parser) hasCorruptFrameInCluster(clusterOffset int64) bool {
	for _, frame := range p.report.Frames {
		if frame.ClusterOffset == clusterOffset && (frame.Status != "ok" || frameHasError(&frame)) {
			return true
		}
	}
	return false
}

func firstErrorCode(frame *FrameReport) string {
	if len(frame.Issues) == 0 {
		if frame.Status != "" {
			return frame.Status
		}
		return "unknown"
	}
	return frame.Issues[0].Code
}

func firstErrorMessage(frame *FrameReport) string {
	if len(frame.Issues) == 0 {
		return frame.Status
	}
	return frame.Issues[0].Message
}

func (p *parser) buildRecoveryRanges() {
	frames := p.report.Frames
	for start := 0; start < len(frames); {
		if frames[start].Status != "ok" || frameHasError(&frames[start]) {
			start++
			continue
		}
		end := start
		for end+1 < len(frames) && frames[end+1].Status == "ok" && !frameHasError(&frames[end+1]) {
			end++
		}
		startFrame := frames[start]
		endFrame := frames[end]
		p.report.RecoverableRanges = append(p.report.RecoverableRanges, FrameRange{
			StartFrameIndex:  startFrame.Index,
			EndFrameIndex:    endFrame.Index,
			StartFrameNumber: startFrame.Index + 1,
			EndFrameNumber:   endFrame.Index + 1,
			FrameCount:       end - start + 1,
			StartOffset:      startFrame.FrameOffset,
			EndOffset:        endFrame.FrameOffset + int64(endFrame.Size),
		})
		start = end + 1
	}
}
