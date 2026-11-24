package aes3

import (
	"fmt"
	"strings"
)

// FormatText renders a compact human-readable diagnosis.
func FormatText(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "AES3/BMC decode: %d edges, half-bit=%.6g (%s)\n", r.Edges, r.HalfUnit, r.HalfUnitSource)
	fmt.Fprintf(&b, "subframes=%d left=%d right=%d parity_errors=%d untrusted=%d\n",
		r.Summary.Subframes, r.Summary.Left, r.Summary.Right, r.Summary.ParityErrors, r.Summary.Untrusted)

	if len(r.LockIntervals) == 0 {
		b.WriteString("lock intervals: none\n")
	} else {
		b.WriteString("lock intervals:\n")
		for _, in := range r.LockIntervals {
			relock := in.RelockCode
			if relock == "" {
				relock = "no-relock"
			}
			fmt.Fprintf(&b, "  #%d t=%.6g..%.6g duration=%.6g edges=[%d,%d) missing_subframes=%d relock=%s\n",
				in.Index, in.StartTime, in.EndTime, in.Duration, in.StartEdge, in.EndEdge, in.MissingSlots, relock)
			fmt.Fprintf(&b, "    reason: %s\n", in.Reason)
		}
	}

	b.WriteString("subframes:\n")
	for _, s := range r.Subframes {
		flags := make([]string, 0, 6)
		if s.BlockStart {
			flags = append(flags, "block-start")
		}
		if !s.AudioValid {
			flags = append(flags, "invalid-audio")
		}
		if s.ChannelStatusBit {
			flags = append(flags, "C=1")
		} else {
			flags = append(flags, "C=0")
		}
		if s.UserBit {
			flags = append(flags, "U=1")
		}
		if !s.ParityOK {
			flags = append(flags, "PARITY-ERR")
		}
		if !s.Trusted {
			flags = append(flags, "QUARANTINED")
		}
		fmt.Fprintf(&b, "  %03d frame=%d %-5s preamble=%s t=%-12.6g sample24=%-9d V=%d C=%d [%s]\n",
			s.Index, s.Frame, s.Channel, s.Code, s.StartTime, s.Audio.Sample24,
			boolToInt(s.ValidityBit), boolToInt(s.ChannelStatusBit), strings.Join(flags, ","))
	}

	writeSegments := func(title string, segments []StatusSegment) {
		if len(segments) == 0 {
			return
		}
		fmt.Fprintf(&b, "%s channel-status segments:\n", title)
		for _, seg := range segments {
			align := ""
			if seg.BlockAligned {
				align = " block-aligned"
			}
			fmt.Fprintf(&b, "  sf=%d..%d bits=%d hex=%s%s\n",
				seg.StartSubframe, seg.EndSubframe, len(seg.Bits), seg.Hex, align)
		}
	}
	writeSegments("left", r.ChannelStatus.Left)
	writeSegments("right", r.ChannelStatus.Right)
	return b.String()
}
