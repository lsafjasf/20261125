package audit

import (
	"bytes"
	"os"
	"testing"
)

const fixturePath = "../testdata/ffv1-4slices.mkv"

func analyzeFixtureBytes(t *testing.T, data []byte, expectedSlices int) *Report {
	t.Helper()
	report, err := AnalyzeReaderAt(bytes.NewReader(data), int64(len(data)), Options{
		ExpectedSlices: expectedSlices,
	})
	if err != nil {
		t.Fatalf("AnalyzeReaderAt: %v", err)
	}
	return report
}

func TestRealFFV1MatroskaFixtureClean(t *testing.T) {
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	report := analyzeFixtureBytes(t, data, 4)
	if len(report.Frames) != 10 {
		t.Fatalf("frames=%d, want 10", len(report.Frames))
	}
	wantPositions := []int64{627, 1808, 2876, 3909, 4951, 5984, 7131, 8182, 9231, 10259}
	for i, position := range wantPositions {
		if report.Frames[i].Position != position {
			t.Fatalf("frame %d position=%d, want %d", i, report.Frames[i].Position, position)
		}
		if report.Frames[i].SliceCount != 4 {
			t.Fatalf("frame %d slices=%d, want 4", i, report.Frames[i].SliceCount)
		}
		if report.Frames[i].CRCOK == nil || !*report.Frames[i].CRCOK {
			t.Fatalf("frame %d CRC not valid: %+v", i, report.Frames[i].CRCOK)
		}
	}
	if !report.Frames[0].Keyframe || !report.Frames[5].Keyframe {
		t.Fatalf("frames 0 and 5 must be keyframes")
	}
	if report.Frames[1].Keyframe {
		t.Fatalf("frame 1 must not be keyframe")
	}
	if len(report.CuePositions) != 2 || !report.CuePositions[0].PositionOK {
		t.Fatalf("cue positions not resolved: %+v", report.CuePositions)
	}
	if report.Summary.Errors != 0 || report.Summary.CorruptFrames != 0 {
		t.Fatalf("clean fixture has findings: errors=%d corrupt=%d issues=%+v",
			report.Summary.Errors, report.Summary.CorruptFrames, report.Issues)
	}
	if len(report.RecoverableRanges) != 1 || report.Summary.RecoverableFrames != 10 {
		t.Fatalf("unexpected recovery ranges: %+v", report.RecoverableRanges)
	}
}

func TestCRCInjectionLocatesSliceAndRecoveryRanges(t *testing.T) {
	clean, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	cleanReport := analyzeFixtureBytes(t, clean, 4)
	target := cleanReport.Frames[3]
	if len(target.Slices) != 4 {
		t.Fatalf("target slices=%d", len(target.Slices))
	}
	flipOffset := target.Slices[1].Offset + 12
	corrupt := append([]byte(nil), clean...)
	corrupt[flipOffset] ^= 0x01

	report := analyzeFixtureBytes(t, corrupt, 4)
	if report.Summary.CorruptFrames != 1 {
		t.Fatalf("corrupt frames=%d, want 1", report.Summary.CorruptFrames)
	}
	if len(report.CorruptBlocks) != 1 {
		t.Fatalf("corrupt blocks=%d, want 1", len(report.CorruptBlocks))
	}
	block := report.CorruptBlocks[0]
	if block.FrameIndex != 3 || block.Kind != "ffv1_slice_crc_mismatch" {
		t.Fatalf("unexpected block: %+v", block)
	}
	if block.CorruptOffset != target.Slices[1].Offset {
		t.Fatalf("corrupt offset=%d, want slice start %d", block.CorruptOffset, target.Slices[1].Offset)
	}
	wantSlices := []int{0, 2, 3}
	if len(block.RecoverableSlices) != len(wantSlices) {
		t.Fatalf("recoverable slices=%v, want %v", block.RecoverableSlices, wantSlices)
	}
	for i, want := range wantSlices {
		if block.RecoverableSlices[i] != want {
			t.Fatalf("recoverable slices=%v, want %v", block.RecoverableSlices, wantSlices)
		}
	}
	if len(report.RecoverableRanges) != 2 {
		t.Fatalf("ranges=%+v, want before/after", report.RecoverableRanges)
	}
	if report.RecoverableRanges[0].StartFrameIndex != 0 || report.RecoverableRanges[0].EndFrameIndex != 2 {
		t.Fatalf("bad first range: %+v", report.RecoverableRanges[0])
	}
	if report.RecoverableRanges[1].StartFrameIndex != 4 || report.RecoverableRanges[1].EndFrameIndex != 9 {
		t.Fatalf("bad second range: %+v", report.RecoverableRanges[1])
	}
	if report.Frames[3].SHA256 == cleanReport.Frames[3].SHA256 {
		t.Fatalf("corrupted frame hash did not change")
	}
	if report.Frames[4].SHA256 != cleanReport.Frames[4].SHA256 {
		t.Fatalf("later clean frame hash changed unexpectedly")
	}
}

func TestUnknownMatroskaElementIsRecordedNotFatal(t *testing.T) {
	clean, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	withUnknown := append(append([]byte(nil), clean...), 0xDE, 0x88)
	withUnknown = append(withUnknown, []byte("UNKNOWN!")...)
	report := analyzeFixtureBytes(t, withUnknown, 4)
	if len(report.UnknownElements) != 1 {
		t.Fatalf("unknown elements=%+v", report.UnknownElements)
	}
	if report.UnknownElements[0].ID != "0xDE" {
		t.Fatalf("unknown ID=%q, want 0xDE", report.UnknownElements[0].ID)
	}
	if len(report.Frames) != 10 || report.Summary.Errors != 0 {
		t.Fatalf("unknown element interrupted audit: frames=%d errors=%d issues=%+v",
			len(report.Frames), report.Summary.Errors, report.Issues)
	}
}

func TestMatroskaContainerCRCMismatch(t *testing.T) {
	clean, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	cleanReport := analyzeFixtureBytes(t, clean, 4)
	var crcOffset int64 = -1
	for _, check := range cleanReport.ContainerCRCs {
		if check.ParentElementID == "0x1549A966" && check.OK {
			crcOffset = check.CRCElementOffset
		}
	}
	if crcOffset < 0 {
		t.Fatalf("Info CRC check not found: %+v", cleanReport.ContainerCRCs)
	}
	corrupt := append([]byte(nil), clean...)
	corrupt[crcOffset+2] ^= 0xFF
	report := analyzeFixtureBytes(t, corrupt, 4)
	found := false
	for _, issue := range report.Issues {
		if issue.Code == "matroska_crc_mismatch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected matroska_crc_mismatch, issues=%+v", report.Issues)
	}
	if len(report.Frames) != 10 {
		t.Fatalf("container CRC failure should not stop frame parsing, frames=%d", len(report.Frames))
	}
}
