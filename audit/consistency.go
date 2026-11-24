package audit

func (p *parser) inferSliceConsistency() {
	if p.effectiveExpectedSlices() > 0 {
		return
	}
	counts := map[int]int{}
	for _, frame := range p.report.Frames {
		if frame.SliceCount > 0 {
			counts[frame.SliceCount]++
		}
	}
	baseline := 0
	bestCount := 0
	for count, occurrences := range counts {
		if occurrences > bestCount || (occurrences == bestCount && count < baseline) {
			baseline = count
			bestCount = occurrences
		}
	}
	if baseline == 0 {
		return
	}

	for i := range p.report.Frames {
		frame := &p.report.Frames[i]
		if frame.SliceCount == 0 {
			continue
		}
		frame.ExpectedSlices = baseline
		ok := frame.SliceCount == baseline
		frame.SliceCountOK = &ok
		if ok || frameHasIssueCode(frame, "ffv1_slice_count_mismatch") {
			continue
		}
		frame.Issues = append(frame.Issues, Issue{
			Severity:   SeverityError,
			Code:       "ffv1_slice_count_mismatch",
			FrameIndex: frame.Index,
			Message:    "slice count differs from dominant count in this FFV1 stream",
			Offset:     frame.FrameOffset,
		})
		if frame.Status == "ok" {
			frame.Status = "invalid"
		}
	}
}

func frameHasIssueCode(frame *FrameReport, code string) bool {
	for _, issue := range frame.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
