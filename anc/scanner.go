package anc

import (
	"fmt"
	"sort"
	"strings"
)

const reportSchema = "ancscan/report-v1"

type candidate struct {
	pkt          *Packet
	lineIndex    int
	adfUnit      int
	headerWords  []Word // DID, SDID/DBN, DC
	udwWords     []Word // UDW present in the own line
	missingUnits int    // >0 when truncated
	wordSig      string
	coreKey      string
}

// Scan inspects every supplied video line for ANC packets and returns a
// location-rich report. Lines are processed in slice order, which is also the
// assumed capture order for cross-line continuation detection.
func Scan(lines []Line, opts *Options) (*Report, error) {
	cfg := opts.withDefaults()
	nls := make([]*normLine, 0, len(lines))
	for _, l := range lines {
		nl, err := normalize(l, cfg.Encoding)
		if err != nil {
			return nil, err
		}
		nls = append(nls, nl)
	}

	var cands []*candidate
	for li, nl := range nls {
		cands = append(cands, scanNormLine(nl, li, cfg)...)
	}

	resolveContinuations(cands, nls, cfg)
	resolveDuplicates(cands)

	// Input slice order is treated as capture/scan order; within a line the
	// ADF unit index gives the byte order. Field/line numbers stay factual
	// metadata and are not assumed monotonic for interlaced captures.
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].lineIndex != cands[j].lineIndex {
			return cands[i].lineIndex < cands[j].lineIndex
		}
		return cands[i].adfUnit < cands[j].adfUnit
	})

	rep := &Report{
		Schema:       reportSchema,
		Encoding:     string(cfg.Encoding),
		ScannedLines: len(lines),
		Packets:      make([]*Packet, 0, len(cands)),
	}
	for idx, c := range cands {
		c.pkt.Index = idx + 1
		rep.Packets = append(rep.Packets, c.pkt)
		accumulateSummary(&rep.Summary, c.pkt)
	}
	return rep, nil
}

func scanNormLine(nl *normLine, li int, cfg Options) []*candidate {
	var out []*candidate
	u := nl.units
	for i := 0; i+2 < len(u); i++ {
		if !nl.adfAt(i) {
			continue
		}
		c := parseCandidate(nl, li, i, cfg)
		if c != nil {
			out = append(out, c)
		}
	}
	return out
}

// adfAt reports whether the three units starting at i form the Ancillary Data
// Flag. Ten-bit encodings use the exact 000/3FF/3FF pattern; the 8-bit byte
// representation uses 00/FF/FF.
func (nl *normLine) adfAt(i int) bool {
	u := nl.units
	if nl.enc == EncANC8 {
		return u[i].w == 0x00 && u[i+1].w == 0xFF && u[i+2].w == 0xFF
	}
	return u[i].w == ADFZero && u[i+1].w == ADFOne && u[i+2].w == ADFOne
}

func parseCandidate(nl *normLine, li, adf int, cfg Options) *candidate {
	units := nl.units
	line := nl.line
	byteOff := units[adf].start
	pkt := &Packet{
		Field:        line.Field,
		Line:         line.Number,
		ByteOffset:   byteOff,
		StreamOffset: line.StreamOffset + byteOff,
	}
	if nl.enc != EncANC8 {
		pkt.WordOffset = adf
	}
	pkt.Encoding = string(nl.enc)

	tenBit := nl.enc != EncANC8
	parity := &ParityStatus{Present: tenBit}
	if !tenBit {
		parity.DID, parity.SDID, parity.DC = "absent", "absent", "absent"
	}
	pkt.Parity = parity

	// Header short: not enough units for DID/SDID/DC.
	if adf+6 > len(units) {
		rawEnd := units[len(units)-1].end
		pkt.RawHex = hexEncode(nl.raw[units[adf].start:minInt(rawEnd, len(nl.raw))])
		pkt.RawLength = len(pkt.RawHex) / 2
		pkt.Classification = ClassTruncated
		pkt.Labels = []string{LabelTruncated, LabelHeaderShort}
		pkt.Reasons = []string{"header_incomplete: missing DID/SDID/DC words at line end"}
		return &candidate{pkt: pkt, lineIndex: li, adfUnit: adf, missingUnits: -1}
	}

	didW, sdidW, dcW := units[adf+3].w, units[adf+4].w, units[adf+5].w
	header := []Word{didW, sdidW, dcW}
	payloadByteOff := units[adf+5].end
	if adf+6 < len(units) {
		payloadByteOff = units[adf+6].start
	}
	pkt.Offsets = &FieldOffsets{
		ADF:       units[adf].start,
		DID:       units[adf+3].start,
		SDIDOrDBN: units[adf+4].start,
		DC:        units[adf+5].start,
		Payload:   payloadByteOff,
	}
	pkt.DID = didW.Data()
	pkt.DIDHex = fmt.Sprintf("0x%02X", pkt.DID)
	pkt.SDIDOrDBN = sdidW.Data()
	pkt.SDIDHex = fmt.Sprintf("0x%02X", pkt.SDIDOrDBN)
	pkt.DC = int(dcW.Data())
	pkt.IDType = "type2_sdid"
	if pkt.DID&0x80 != 0 {
		pkt.IDType = "type1_dbn"
	}
	pkt.DIDName = DIDName(pkt.DID)
	if name, ok := LookupDID(pkt.DID); !ok {
		pkt.UnknownDID = true
		pkt.Labels = append(pkt.Labels, LabelUnknownDID)
	} else if sub := SDIDName(pkt.DID, pkt.SDIDOrDBN); sub != "" {
		pkt.SDIDName = sub
	} else {
		_ = name
	}

	if tenBit {
		markParity := func(w Word) string {
			if ParityOK(w) {
				return "ok"
			}
			pkt.Labels = appendUnique(pkt.Labels, LabelParityError)
			return "error"
		}
		parity.DID = markParity(didW)
		parity.SDID = markParity(sdidW)
		parity.DC = markParity(dcW)
		if strings.Contains(strings.Join(pkt.Labels, ","), LabelParityError) {
			pkt.Reasons = append(pkt.Reasons, "parity_error: DID/SDID/DC parity bit mismatch")
		}
	}

	payloadStart := adf + 6
	available := len(units) - payloadStart // UDW + CS units left in this line
	udwCount := minInt(pkt.DC, maxInt(0, available))
	udwWords := make([]Word, 0, udwCount)
	payload := make([]byte, 0, udwCount)
	udwInts := make([]int, 0, udwCount)
	for j := 0; j < udwCount; j++ {
		w := units[payloadStart+j].w
		udwWords = append(udwWords, w)
		payload = append(payload, w.Data())
		udwInts = append(udwInts, int(w))
	}
	pkt.PayloadHex = hexEncode(payload)
	pkt.PayloadASCII = asciiPreview(payload)
	pkt.UDW = udwInts

	csIndex := payloadStart + pkt.DC
	complete := csIndex < len(units)

	var rawEnd int
	if complete {
		rawEnd = units[csIndex].end
	} else {
		rawEnd = units[len(units)-1].end
	}
	rawEnd = minInt(rawEnd, len(nl.raw))
	pkt.RawHex = hexEncode(nl.raw[units[adf].start:rawEnd])
	pkt.RawLength = len(pkt.RawHex) / 2

	cand := &candidate{
		pkt:         pkt,
		lineIndex:   li,
		adfUnit:     adf,
		headerWords: header,
		udwWords:    udwWords,
	}

	if cfg.ExpectedLines != nil {
		if expected, ok := cfg.ExpectedLines[pkt.DID]; ok && len(expected) > 0 && !containsInt(expected, line.Number) {
			pkt.Labels = appendUnique(pkt.Labels, LabelDIDLineMismatch)
			pkt.Reasons = append(pkt.Reasons, fmt.Sprintf("did_line_mismatch: DID %02X not expected on line %d", pkt.DID, line.Number))
		}
	}

	if complete {
		csW := units[csIndex].w
		pkt.Offsets.Checksum = units[csIndex].start
		verifyChecksum(pkt, nl.enc, header, udwWords, csW)
		cand.wordSig = wordSignature(append(append([]Word{ADFZero, ADFOne, ADFOne}, header...), append(udwWords, csW)...))
		cand.coreKey = contentKey(pkt.DID, pkt.SDIDOrDBN, pkt.DC, udwWords)
		if checksumHealthy(pkt) && parityHealthy(pkt) {
			pkt.Classification = ClassValidVANC
			pkt.Labels = appendUnique(pkt.Labels, LabelValidVANC)
		} else {
			pkt.Classification = ClassInvalid
		}
	} else {
		cand.missingUnits = pkt.DC + 1 - available
		if cand.missingUnits < 1 {
			cand.missingUnits = 1
		}
		pkt.Classification = ClassTruncated
		pkt.Labels = appendUnique(pkt.Labels, LabelTruncated)
		pkt.Reasons = append(pkt.Reasons, fmt.Sprintf("dc_exceeds_line: DC=%d but %d word(s) missing before line end", pkt.DC, cand.missingUnits))
		pkt.Checksum = &ChecksumInfo{Mode: checksumModeName(nl.enc), Status: "missing"}
		cand.coreKey = contentKey(pkt.DID, pkt.SDIDOrDBN, pkt.DC, udwWords)
	}

	return cand
}

func verifyChecksum(pkt *Packet, enc Encoding, header []Word, udw []Word, csW Word) {
	info := &ChecksumInfo{Mode: checksumModeName(enc)}
	if enc == EncANC8 {
		payload := make([]byte, len(udw))
		for i, w := range udw {
			payload[i] = w.Data()
		}
		want := Checksum8(header[0].Data(), header[1].Data(), header[2].Data(), payload)
		got := csW.Data()
		info.Expected = fmt.Sprintf("0x%02X", want)
		info.Actual = fmt.Sprintf("0x%02X", got)
		if want == got {
			info.Status = "ok"
		} else {
			info.Status = "error"
			pkt.Labels = appendUnique(pkt.Labels, LabelChecksumError)
			pkt.Reasons = append(pkt.Reasons, "checksum_mismatch: 8-bit sum does not match CS byte")
		}
		pkt.Checksum = info
		return
	}

	words := append(append([]Word{}, header...), udw...)
	want := Checksum9(words...)
	got := uint16(csW) & 0x1FF
	info.Expected = fmt.Sprintf("0x%03X", want)
	info.Actual = fmt.Sprintf("0x%03X", got)
	if ChecksumFramingOK(csW) {
		info.Framing = "ok"
	} else {
		info.Framing = "error"
		pkt.Labels = appendUnique(pkt.Labels, LabelChecksumFraming)
		pkt.Reasons = append(pkt.Reasons, "checksum_framing_error: CS bit b9 must be NOT b8")
	}
	if want == got {
		if info.Framing == "ok" {
			info.Status = "ok"
		} else {
			info.Status = "error"
			pkt.Labels = appendUnique(pkt.Labels, LabelChecksumError)
			pkt.Reasons = append(pkt.Reasons, "checksum_value_ok_but_framing_bad")
		}
	} else {
		info.Status = "error"
		pkt.Labels = appendUnique(pkt.Labels, LabelChecksumError)
		pkt.Reasons = append(pkt.Reasons, fmt.Sprintf("checksum_mismatch: expected 0x%03X got 0x%03X", want, got))
	}
	pkt.Checksum = info
}

func checksumModeName(enc Encoding) string {
	if enc == EncANC8 {
		return "sum8"
	}
	return "st291_9bit"
}

func parityHealthy(p *Packet) bool {
	if p.Parity == nil || !p.Parity.Present {
		return true
	}
	return p.Parity.DID == "ok" && p.Parity.SDID == "ok" && p.Parity.DC == "ok"
}

func checksumHealthy(p *Packet) bool {
	return p.Checksum != nil && p.Checksum.Status == "ok"
}

func resolveDuplicates(cands []*candidate) {
	first := map[string]*candidate{}
	for _, c := range cands {
		if c.pkt.Classification == ClassTruncated || c.coreKey == "" {
			continue
		}
		if prev, ok := first[c.coreKey]; ok {
			loc := Location{
				Field:        prev.pkt.Field,
				Line:         prev.pkt.Line,
				ByteOffset:   prev.pkt.ByteOffset,
				WordOffset:   prev.pkt.WordOffset,
				StreamOffset: prev.pkt.StreamOffset,
			}
			c.pkt.DuplicateOf = &loc
			if c.wordSig != "" && c.wordSig == prev.wordSig {
				c.pkt.DuplicateMatch = "raw"
				c.pkt.Labels = appendUnique(c.pkt.Labels, LabelExactDuplicate)
			} else {
				c.pkt.DuplicateMatch = "content"
			}
			c.pkt.Classification = ClassDuplicate
			c.pkt.Labels = appendUnique(c.pkt.Labels, LabelDuplicate)
			c.pkt.Reasons = append(c.pkt.Reasons, fmt.Sprintf("duplicate_of field=%d line=%d byte_offset=%d (%s match)",
				loc.Field, loc.Line, loc.ByteOffset, c.pkt.DuplicateMatch))
			continue
		}
		first[c.coreKey] = c
	}
}

func accumulateSummary(s *Summary, p *Packet) {
	s.Packets++
	switch p.Classification {
	case ClassValidVANC:
		s.ValidVANC++
	case ClassDuplicate:
		s.Duplicate++
	case ClassTruncated:
		s.Truncated++
	case ClassInvalid:
		s.Invalid++
	}
	if p.UnknownDID {
		s.UnknownDID++
	}
	if hasLabel(p.Labels, LabelParityError) {
		s.ParityErrors++
	}
	if hasLabel(p.Labels, LabelChecksumError) {
		s.ChecksumErrors++
	}
	if hasLabel(p.Labels, LabelCrossLine) {
		s.CrossLine++
	}
	if p.Continuation != nil && p.Continuation.ChecksumOK {
		s.ReassembledOK++
	}
}

func wordSignature(ws []Word) string {
	var b strings.Builder
	for _, w := range ws {
		fmt.Fprintf(&b, "%03X", uint16(w)&0x3FF)
	}
	return b.String()
}

// contentKey identifies packet content independent of ADF/CS and of parity-bit
// corruption: DID/SDID/DC data bytes plus the full (possibly >8-bit) UDW words.
func contentKey(did, sdid byte, dc int, udw []Word) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%02X/%02X/%02X", did, sdid, dc&0xFF)
	for _, w := range udw {
		fmt.Fprintf(&b, "/%03X", uint16(w)&0x3FF)
	}
	return b.String()
}

func appendUnique(labels []string, label string) []string {
	if !hasLabel(labels, label) {
		return append(labels, label)
	}
	return labels
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func asciiPreview(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	printable := 0
	out := make([]byte, len(b))
	for i, v := range b {
		if v >= 0x20 && v < 0x7F {
			out[i] = v
			printable++
		} else {
			out[i] = '.'
		}
	}
	if printable*100/len(b) < 75 {
		return ""
	}
	return string(out)
}
