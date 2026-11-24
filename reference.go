package mp4diag

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func compareReference(candidate io.ReaderAt, candidateSize int64, candidateReport *Report, refReport *Report) ReferenceReport {
	comparison := ReferenceReport{Path: refReport.Path, PixelDamage: PixelRuledOut}
	refFile, err := os.Open(refReport.Path)
	if err != nil {
		comparison.PixelDamage = PixelNotAssessed
		return comparison
	}
	defer refFile.Close()

	for _, refSample := range refReport.Samples {
		end := refSample.Offset + int64(refSample.Size)
		item := SampleComparison{
			Sample: refSample.Number, RefOffset: refSample.Offset, RefSize: refSample.Size, CandidateEnd: end,
		}
		item.RefSHA256 = hashRange(refFile, refSample.Offset, int64(refSample.Size))
		if refSample.Offset < 0 || end > candidateSize {
			item.Status = "missing"
			comparison.MissingSamples = append(comparison.MissingSamples, refSample.Number)
		} else {
			item.ActualSHA256 = hashRange(candidate, refSample.Offset, int64(refSample.Size))
			if item.ActualSHA256 == item.RefSHA256 {
				item.Status = "identical"
				comparison.IntactSamples = append(comparison.IntactSamples, refSample.Number)
			} else {
				item.Status = "modified"
				comparison.ChangedSamples = append(comparison.ChangedSamples, refSample.Number)
			}
		}
		comparison.SampleComparisons = append(comparison.SampleComparisons, item)
	}

	switch {
	case len(comparison.ChangedSamples) > 0:
		comparison.PixelDamage = PixelSuspected
	case len(comparison.MissingSamples) > 0:
		comparison.PixelDamage = PixelNotAssessed
	default:
		comparison.PixelDamage = PixelRuledOut
	}
	return comparison
}

func hashRange(r io.ReaderAt, offset, size int64) string {
	if size <= 0 {
		sum := sha256.Sum256(nil)
		return hex.EncodeToString(sum[:])
	}
	hasher := sha256.New()
	if _, err := io.CopyN(hasher, io.NewSectionReader(r, offset, size), size); err != nil {
		return ""
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func appendReferenceFindings(report *Report, refReport *Report) {
	if report.Reference == nil {
		return
	}
	reference := report.Reference
	for _, number := range reference.ChangedSamples {
		var byteRanges []ByteSpan
		for _, cmp := range reference.SampleComparisons {
			if cmp.Sample == number {
				byteRanges = []ByteSpan{{Offset: cmp.RefOffset, Length: int64(cmp.RefSize)}}
			}
		}
		report.Findings = append(report.Findings, Finding{
			Layer: LayerCompressed, Severity: SeverityError,
			Code:    "sample_bytes_differ_from_reference",
			Message: fmt.Sprintf("sample %d 的长度前缀/NAL 区间与已知良好参考不一致；压缩数据层存在注入", number),
			Samples: []int{number}, Bytes: byteRanges, PixelDamage: PixelSuspected,
		})
	}

	appendSTSZReferenceFinding(report, refReport)

	if reference.PixelDamage == PixelRuledOut && hasMetadataFault(report) {
		for i := range report.Findings {
			if report.Findings[i].Layer == LayerSampleTable && report.Findings[i].PixelDamage == "" {
				report.Findings[i].PixelDamage = PixelRuledOut
			}
		}
	}
}

func hasMetadataFault(report *Report) bool {
	for _, finding := range report.Findings {
		if finding.Layer == LayerSampleTable {
			return true
		}
	}
	return false
}

func appendSTSZReferenceFinding(report *Report, refReport *Report) {
	candBox := findBoxOfType(report.Boxes, "stsz")
	refBox := findBoxOfType(refReport.Boxes, "stsz")
	if candBox == nil || refBox == nil {
		return
	}
	candFile, err := os.Open(report.Path)
	if err != nil {
		return
	}
	defer candFile.Close()
	refFile, err := os.Open(refReport.Path)
	if err != nil {
		return
	}
	defer refFile.Close()

	candSizes, candCount := readSTSZSizesForDiff(candFile, *candBox)
	refSizes, refCount := readSTSZSizesForDiff(refFile, *refBox)
	if candCount != refCount {
		report.Findings = append(report.Findings, metadataReferenceFinding(candBox, "sample_count", 0,
			fmt.Sprintf("stsz sample_count: reference=%d candidate=%d", refCount, candCount)))
		return
	}
	for i := 0; i < len(candSizes) && i < len(refSizes); i++ {
		if candSizes[i] == refSizes[i] {
			continue
		}
		finding := metadataReferenceFinding(candBox, "sample_size", i+1, fmt.Sprintf(
			"stsz entry %d: reference size=%d candidate size=%d；同绝对区间的 sample 负载 SHA-256 未变，因此不是像素损坏",
			i+1, refSizes[i], candSizes[i]))
		finding.Samples = []int{i + 1}
		finding.PixelDamage = PixelRuledOut
		report.Findings = append(report.Findings, finding)
		return
	}
}

func metadataReferenceFinding(box *BoxInfo, field string, entry int, message string) Finding {
	return Finding{
		Layer: LayerSampleTable, Severity: SeverityError,
		Code: "metadata_entry_differs_from_reference", Message: message,
		Box:         &BoxLocator{Path: box.Path, Type: box.Type, Offset: box.Offset, Field: field, Entry: entry},
		PixelDamage: PixelRuledOut,
	}
}

func readSTSZSizesForDiff(r io.ReaderAt, box BoxInfo) ([]uint32, uint32) {
	payload, err := readPayload(r, box)
	if err != nil || len(payload) < 12 {
		return nil, 0
	}
	uniform := beUint32(payload[4:8])
	count := beUint32(payload[8:12])
	if uniform != 0 {
		sizes := make([]uint32, minUint32(count, maxAnalyzedSamples))
		for i := range sizes {
			sizes[i] = uniform
		}
		return sizes, count
	}
	n := int(minUint32(count, maxAnalyzedSamples))
	sizes := make([]uint32, 0, n)
	for i := 0; i < n && 12+i*4+4 <= len(payload); i++ {
		sizes = append(sizes, beUint32(payload[12+i*4:16+i*4]))
	}
	return sizes, count
}

func beUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func findBoxOfType(boxes []BoxInfo, typ string) *BoxInfo {
	for i := range boxes {
		if boxes[i].Type == typ {
			return &boxes[i]
		}
	}
	return nil
}
