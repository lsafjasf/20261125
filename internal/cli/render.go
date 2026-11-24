package cli

import (
	"fmt"
	"io"
	"strings"

	"ancscan/anc"
)

var classCN = map[anc.Classification]string{
	anc.ClassValidVANC: "合法VANC",
	anc.ClassDuplicate: "重复包",
	anc.ClassTruncated: "行间截断",
	anc.ClassInvalid:   "非法/校验错误",
}

func renderText(w io.Writer, rep *anc.Report) {
	fmt.Fprintf(w, "ANC scan: %d line(s), %d packet candidate(s) [%s]\n",
		rep.ScannedLines, rep.Summary.Packets, rep.Schema)
	if rep.Standard != "" {
		fmt.Fprintf(w, "standard: %s\n", rep.Standard)
	}
	s := rep.Summary
	fmt.Fprintf(w, "summary: valid=%d duplicate=%d truncated=%d invalid=%d unknown_did=%d parity_err=%d checksum_err=%d cross_line=%d reassembled_ok=%d\n\n",
		s.ValidVANC, s.Duplicate, s.Truncated, s.Invalid, s.UnknownDID,
		s.ParityErrors, s.ChecksumErrors, s.CrossLine, s.ReassembledOK)

	for _, p := range rep.Packets {
		fmt.Fprintf(w, "#%d [%s/%s] field=%d line=%d byte_offset=%d",
			p.Index, p.Classification, classCN[p.Classification],
			p.Field, p.Line, p.ByteOffset)
		if p.WordOffset != 0 {
			fmt.Fprintf(w, " word_offset=%d", p.WordOffset)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "    DID=%s SDID/DBN=%s DC=%d type=%s name=%s\n",
			p.DIDHex, p.SDIDHex, p.DC, p.IDType, p.DIDName)
		if p.SDIDName != "" {
			fmt.Fprintf(w, "    sub-id: %s\n", p.SDIDName)
		}
		fmt.Fprintf(w, "    payload(%d bytes)=%s\n", len(p.PayloadHex)/2, p.PayloadHex)
		if p.PayloadASCII != "" {
			fmt.Fprintf(w, "    ascii=%q\n", p.PayloadASCII)
		}
		fmt.Fprintf(w, "    raw(%d)=%s\n", p.RawLength, p.RawHex)
		if p.Parity != nil {
			if p.Parity.Present {
				fmt.Fprintf(w, "    parity: did=%s sdid/dbn=%s dc=%s\n", p.Parity.DID, p.Parity.SDID, p.Parity.DC)
			} else {
				fmt.Fprintln(w, "    parity: absent (8-bit encoding)")
			}
		}
		if p.Checksum != nil {
			fmt.Fprintf(w, "    checksum: mode=%s status=%s", p.Checksum.Mode, p.Checksum.Status)
			if p.Checksum.Expected != "" {
				fmt.Fprintf(w, " expected=%s actual=%s", p.Checksum.Expected, p.Checksum.Actual)
			}
			if p.Checksum.Framing != "" {
				fmt.Fprintf(w, " framing=%s", p.Checksum.Framing)
			}
			fmt.Fprintln(w)
		}
		if p.DuplicateOf != nil {
			fmt.Fprintf(w, "    duplicate_of: field=%d line=%d byte_offset=%d (%s match)\n",
				p.DuplicateOf.Field, p.DuplicateOf.Line, p.DuplicateOf.ByteOffset, p.DuplicateMatch)
		}
		if p.Continuation != nil {
			c := p.Continuation
			if c.UnitsTaken == 0 && c.NextStartsADF {
				fmt.Fprintf(w, "    continuation: missing=%d; next line starts a new ADF, nothing consumed\n", c.MissingUnits)
			} else {
				fmt.Fprintf(w, "    continuation: field=%d line=%d offset=%d taken=%d missing=%d reassembled=%v checksum_ok=%v next_starts_adf=%v\n",
					c.Field, c.Line, c.ByteOffset, c.UnitsTaken, c.MissingUnits,
					c.Reassembled, c.ChecksumOK, c.NextStartsADF)
				if c.RawHex != "" {
					fmt.Fprintf(w, "    continuation_raw=%s\n", c.RawHex)
				}
			}
		}
		if len(p.Labels) > 0 {
			fmt.Fprintf(w, "    labels: %s\n", strings.Join(p.Labels, ","))
		}
		for _, r := range p.Reasons {
			fmt.Fprintf(w, "    - %s\n", r)
		}
		fmt.Fprintln(w)
	}
}
