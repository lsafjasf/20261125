package main

import (
	"testing"

	"ffv1mkaudit/audit"
)

func analyzeInjection(t *testing.T, path string) *audit.Report {
	t.Helper()
	report, err := audit.AnalyzePath(path, audit.Options{ExpectedSlices: 4})
	if err != nil {
		t.Fatalf("AnalyzePath(%s): %v", path, err)
	}
	return report
}

func TestCommittedInjectionArtifacts(t *testing.T) {
	unknown := analyzeInjection(t, "injections/01-unknown-matroska-element.mkv")
	if len(unknown.UnknownElements) != 1 || unknown.Summary.Errors != 0 {
		t.Fatalf("unknown injection wrong: %+v %+v", unknown.UnknownElements, unknown.Summary)
	}

	corrupt := analyzeInjection(t, "injections/02-ffv1-slice-crc-block.mkv")
	if len(corrupt.CorruptBlocks) == 0 || corrupt.CorruptBlocks[0].FrameIndex != 3 {
		t.Fatalf("CRC block not located: %+v", corrupt.CorruptBlocks)
	}
	if len(corrupt.RecoverableRanges) != 2 {
		t.Fatalf("recovery ranges=%+v", corrupt.RecoverableRanges)
	}

	truncated := analyzeInjection(t, "injections/03-truncated-block.mkv")
	if truncated.Summary.CorruptFrames != 1 || len(truncated.RecoverableRanges) != 1 {
		t.Fatalf("truncation summary=%+v ranges=%+v", truncated.Summary, truncated.RecoverableRanges)
	}
	if truncated.RecoverableRanges[0].EndFrameIndex != 3 {
		t.Fatalf("truncation recovery=%+v", truncated.RecoverableRanges[0])
	}
}
