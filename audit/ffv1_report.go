package audit

import (
	"fmt"

	"ffv1mkaudit/ebml"
	"ffv1mkaudit/ffv1"
)

func (p *parser) effectiveExpectedSlices() int {
	if p.opts.ExpectedSlices > 0 {
		return p.opts.ExpectedSlices
	}
	if p.opts.Expectations != nil && p.opts.Expectations.ExpectedSlices > 0 {
		return p.opts.Expectations.ExpectedSlices
	}
	return 0
}

func (p *parser) ffv1Mode() ffv1.Mode {
	switch p.opts.FFV1CRC {
	case "on", "crc", "required":
		return ffv1.ModeCRC
	case "off", "none":
		return ffv1.ModeNoCRC
	default:
		return ffv1.ModeAuto
	}
}

func (p *parser) applyFFV1(frame *FrameReport, sample []byte, mode ffv1.Mode) {
	expected := p.effectiveExpectedSlices()
	result := ffv1.Analyze(sample, mode, expected)
	frame.SliceCount = result.SliceCount
	frame.ExpectedSlices = result.ExpectedSlices
	frame.CRCEnabled = result.CRCEnabled
	if result.CRCAvailable {
		ok := result.CRCOK
		frame.CRCOK = &ok
	} else {
		frame.CRCOK = nil
	}
	if result.HasExpectation {
		ok := result.SliceCountOK
		frame.SliceCountOK = &ok
	}

	frame.Slices = frame.Slices[:0]
	for _, slice := range result.Slices {
		report := SliceReport{
			Index:         slice.Index,
			FrameOffset:   slice.Start,
			Offset:        frame.FrameOffset + int64(slice.Start),
			EndOffset:     frame.FrameOffset + int64(slice.End),
			Size:          slice.End - slice.Start,
			SizeField:     slice.SizeField,
			FooterOffset:  frame.FrameOffset + int64(slice.FooterStart),
			ErrorStatus:   slice.ErrorStatus,
			CRCEnabled:    slice.CRCEnabled,
			BoundaryOK:    true,
			CRCStored:     fmt.Sprintf("%08X", slice.StoredCRC),
			CRCCalculated: fmt.Sprintf("%08X", slice.CalculatedCRC),
		}
		if slice.CRCEnabled {
			ok := slice.CRCStatus == ffv1.CRCValid
			report.CRCOK = &ok
		}
		frame.Slices = append(frame.Slices, report)
	}

	filtered := frame.Issues[:0]
	for _, issue := range frame.Issues {
		if issue.Code != "ffv1_slice_chain" &&
			issue.Code != "ffv1_slice_crc_mismatch" &&
			issue.Code != "ffv1_slice_count_mismatch" {
			filtered = append(filtered, issue)
		}
	}
	frame.Issues = filtered

	if !result.ChainOK {
		frame.Issues = append(frame.Issues, Issue{
			Severity: SeverityError, Code: "ffv1_slice_chain", FrameIndex: frame.Index,
			Message: joinStrings(result.Issues), Offset: frame.FrameOffset,
		})
		if frame.Status == "ok" {
			frame.Status = "corrupt"
		}
	}
	crcBad := false
	for _, slice := range frame.Slices {
		if slice.CRCOK != nil && !*slice.CRCOK {
			crcBad = true
			frame.Issues = append(frame.Issues, Issue{
				Severity: SeverityError, Code: "ffv1_slice_crc_mismatch",
				FrameIndex: frame.Index, SliceIndex: slice.Index,
				Message: "FFV1 slice CRC mismatch", Offset: slice.Offset, EndOffset: slice.EndOffset,
			})
		}
	}
	if crcBad && frame.Status == "ok" {
		frame.Status = "corrupt"
	}
	if result.HasExpectation && !result.SliceCountOK {
		frame.Issues = append(frame.Issues, Issue{
			Severity: SeverityError, Code: "ffv1_slice_count_mismatch", FrameIndex: frame.Index,
			Message: fmt.Sprintf("slice count %d does not match expected %d", result.SliceCount, expected),
			Offset:  frame.FrameOffset,
		})
		if frame.Status == "ok" {
			frame.Status = "invalid"
		}
	}
}

func joinStrings(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += "; "
		}
		out += value
	}
	return out
}

func (p *parser) refineFFV1Modes() {
	if p.opts.FFV1CRC == "on" || p.opts.FFV1CRC == "off" {
		mode := p.ffv1Mode()
		for i := range p.report.Frames {
			frame := &p.report.Frames[i]
			if !p.trackIsFFV1(frame.TrackNumber) || frame.Size <= 0 {
				continue
			}
			sample, err := ebml.ReadBytes(p.r, frame.FrameOffset, int64(frame.Size))
			if err == nil {
				p.applyFFV1(frame, sample, mode)
			}
		}
		return
	}

	crcTracks := map[uint64]bool{}
	for _, frame := range p.report.Frames {
		if frame.CRCEnabled && frame.CRCOK != nil && *frame.CRCOK {
			crcTracks[frame.TrackNumber] = true
		}
	}
	for i := range p.report.Frames {
		frame := &p.report.Frames[i]
		if !p.trackIsFFV1(frame.TrackNumber) || frame.Size <= 0 || frame.CRCEnabled {
			continue
		}
		mode := ffv1.ModeAuto
		if crcTracks[frame.TrackNumber] {
			mode = ffv1.ModeCRC
		}
		sample, err := ebml.ReadBytes(p.r, frame.FrameOffset, int64(frame.Size))
		if err != nil {
			continue
		}
		p.applyFFV1(frame, sample, mode)
	}
}
