package audit

import (
	"strings"
)

func (p *parser) applyExpectations() {
	if p.opts.Expectations == nil {
		return
	}
	for i, expectation := range p.opts.Expectations.Frames {
		index := i
		if expectation.Index != nil {
			index = *expectation.Index
		}
		if index < 0 || index >= len(p.report.Frames) {
			p.addIssue(Issue{
				Severity: SeverityError, Code: "expectation_frame_missing",
				Message: "expected frame index is not present in Matroska blocks", FrameIndex: index,
			})
			continue
		}
		frame := &p.report.Frames[index]
		if expectation.SHA256 != "" && !strings.EqualFold(frame.SHA256, expectation.SHA256) {
			frame.Issues = append(frame.Issues, Issue{
				Severity: SeverityError, Code: "frame_hash_mismatch", FrameIndex: index,
				Message: "frame SHA-256 does not match expectation", Offset: frame.FrameOffset,
			})
		}
		if expectation.Size > 0 && frame.Size != expectation.Size {
			frame.Issues = append(frame.Issues, Issue{
				Severity: SeverityError, Code: "frame_size_mismatch", FrameIndex: index,
				Message: "frame size does not match expectation", Offset: frame.FrameOffset,
			})
		}
		if expectation.Position != 0 && frame.Position != expectation.Position {
			frame.Issues = append(frame.Issues, Issue{
				Severity: SeverityError, Code: "sample_position_mismatch", FrameIndex: index,
				Message: "Matroska block payload position does not match expectation",
				Offset:  frame.Position,
			})
		}
		if expectation.Keyframe != nil && frame.Keyframe != *expectation.Keyframe {
			frame.Issues = append(frame.Issues, Issue{
				Severity: SeverityError, Code: "keyframe_flag_mismatch", FrameIndex: index,
				Message: "keyframe flag does not match expectation", Offset: frame.Position,
			})
		}
		if expectation.Slices > 0 && frame.SliceCount != expectation.Slices {
			frame.Issues = append(frame.Issues, Issue{
				Severity: SeverityError, Code: "ffv1_slice_count_mismatch", FrameIndex: index,
				Message: "per-frame expected slice count does not match", Offset: frame.FrameOffset,
			})
		}
		if frame.Status == "ok" && frameHasError(frame) {
			frame.Status = "corrupt"
		}
	}
}

func frameHasError(frame *FrameReport) bool {
	for _, issue := range frame.Issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (p *parser) finalize() {
	p.deduplicateIssues()
	p.buildCorruptBlocks()
	p.buildRecoveryRanges()

	summary := Summary{
		Frames:            len(p.report.Frames),
		UnknownElements:   len(p.report.UnknownElements),
		RecoverableRanges: len(p.report.RecoverableRanges),
	}
	for _, frame := range p.report.Frames {
		if frame.Keyframe {
			summary.Keyframes++
		}
		if frame.Status == "ok" && !frameHasError(&frame) {
			summary.OKFrames++
		} else {
			summary.CorruptFrames++
		}
		for _, issue := range frame.Issues {
			if issue.Severity == SeverityError {
				summary.Errors++
			} else if issue.Severity == SeverityWarning {
				summary.Warnings++
			}
		}
	}
	for _, issue := range p.report.Issues {
		if issue.Severity == SeverityError {
			summary.Errors++
		} else if issue.Severity == SeverityWarning {
			summary.Warnings++
		}
	}
	for _, r := range p.report.RecoverableRanges {
		summary.RecoverableFrames += r.FrameCount
	}
	p.report.Summary = summary
}

func (p *parser) deduplicateIssues() {
	seen := map[string]bool{}
	out := p.report.Issues[:0]
	for _, issue := range p.report.Issues {
		key := issue.Code + "|" + issue.Message + "|" +
			itoa(issue.Offset) + "|" + itoa(issue.EndOffset) + "|" + itoa(int64(issue.FrameIndex))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, issue)
	}
	p.report.Issues = out
}

func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
