package ffv1

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
)

type Slice struct {
	Index         int
	Start         int
	FooterStart   int
	End           int
	SizeField     uint32
	Payload       []byte
	StoredCRC     uint32
	CalculatedCRC uint32
	CRCOK         bool
	CRCEnabled    bool
	ErrorStatus   byte
	PayloadSHA256 string
}

type SliceReport struct {
	Count      int
	Expected   int
	CRCEnabled bool
	CRCOK      bool
	Slices     []Slice
	// SetSHA256 hashes the sorted multiset of payload hashes, so it is
	// invariant to slice reordering and CRC footers.
	SetSHA256 string
	// SequenceSHA256 hashes payload hashes in stored order.
	SequenceSHA256 string
	Error          string
}

// AnalyzeSlices splits an FFV1 frame into slices using the self-describing
// footer chain, verifies per-slice CRCs, and produces a content hash that is
// invariant to slice ordering and CRC footers.
func AnalyzeSlices(sample []byte, expected int) SliceReport {
	if len(sample) == 0 {
		return SliceReport{Expected: expected, Error: "empty FFV1 sample"}
	}
	crcReport, crcOK := parseChain(sample, expected, true)
	plainReport, plainOK := parseChain(sample, expected, false)

	// Prefer the CRC chain when it is structurally complete and either the
	// count matches or at least one stored CRC is present and valid.
	useCRC := false
	if crcOK && (expected <= 0 || crcReport.Count == expected) {
		useCRC = true
		if expected <= 0 && plainOK && plainReport.Count == crcReport.Count && crcReport.Count > 1 {
			// Both parses are possible; CRC footers are only accepted if
			// they look like CRC footers rather than arbitrary sizes.
			valid := 0
			for _, s := range crcReport.Slices {
				if s.CRCOK {
					valid++
				}
			}
			if valid == 0 {
				useCRC = false
			}
		}
	}
	if useCRC {
		finalizeSlices(&crcReport)
		return crcReport
	}
	if plainOK {
		finalizeSlices(&plainReport)
		return plainReport
	}
	if crcOK {
		finalizeSlices(&crcReport)
		return crcReport
	}
	return SliceReport{Expected: expected, Error: "slice footer chain is invalid"}
}

func parseChain(sample []byte, expected int, withCRC bool) (SliceReport, bool) {
	footerLen := 3
	if withCRC {
		footerLen = 8
	}
	report := SliceReport{
		Expected:   expected,
		CRCEnabled: withCRC,
		CRCOK:      true,
	}
	end := len(sample)
	for end > 0 {
		if end < footerLen {
			return report, false
		}
		footerStart := end - footerLen
		payloadLen := int(uint32(sample[footerStart])<<16 |
			uint32(sample[footerStart+1])<<8 |
			uint32(sample[footerStart+2]))
		start := footerStart - payloadLen
		if payloadLen == 0 || start < 0 || start >= footerStart {
			return report, false
		}
		s := Slice{
			Index:       len(report.Slices),
			Start:       start,
			FooterStart: footerStart,
			End:         end,
			SizeField:   uint32(payloadLen),
			Payload:     sample[start:footerStart],
			CRCEnabled:  withCRC,
		}
		if withCRC {
			s.ErrorStatus = sample[footerStart+3]
			s.StoredCRC = binary.BigEndian.Uint32(sample[footerStart+4 : end])
			s.CalculatedCRC = CRC32Raw(sample[start : end-4])
			s.CRCOK = CRC32Raw(sample[start:end]) == 0
			if !s.CRCOK {
				report.CRCOK = false
			}
		}
		report.Slices = append(report.Slices, s)
		end = start
	}
	// Footer chain was read from the end; restore storage order.
	for i, j := 0, len(report.Slices)-1; i < j; i, j = i+1, j-1 {
		report.Slices[i], report.Slices[j] = report.Slices[j], report.Slices[i]
	}
	for i := range report.Slices {
		report.Slices[i].Index = i
	}
	report.Count = len(report.Slices)
	if expected > 0 && report.Count != expected {
		// A structurally valid but unexpected count is still useful
		// evidence, but only when the chain itself is unambiguous.
		if withCRC && report.Count == 0 {
			return report, false
		}
	}
	return report, report.Count > 0
}

func finalizeSlices(report *SliceReport) {
	hashes := make([]string, len(report.Slices))
	for i := range report.Slices {
		s := &report.Slices[i]
		sum := sha256.Sum256(s.Payload)
		s.PayloadSHA256 = hex.EncodeToString(sum[:])
		hashes[i] = s.PayloadSHA256
	}
	sorted := append([]string(nil), hashes...)
	sort.Strings(sorted)

	h := sha256.New()
	fmt.Fprintf(h, "slice_count=%d\n", report.Count)
	for _, s := range sorted {
		fmt.Fprintln(h, s)
	}
	report.SetSHA256 = hex.EncodeToString(h.Sum(nil))

	seq := sha256.New()
	fmt.Fprintf(seq, "slice_count=%d\n", report.Count)
	for _, s := range hashes {
		fmt.Fprintln(seq, s)
	}
	report.SequenceSHA256 = hex.EncodeToString(seq.Sum(nil))
}
