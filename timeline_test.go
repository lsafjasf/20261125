package main

import (
	"strings"
	"testing"
)

func testTrack(durations []uint32, offsets []int64) *track {
	present := make([]bool, len(durations))
	sync := make([]bool, len(durations))
	for i := range durations {
		present[i] = true
		sync[i] = i == 0
	}
	return &track{
		ID:            1,
		Codec:         "avc1",
		Timescale:     30000,
		SampleCount:   len(durations),
		Durations:     durations,
		Offsets:       offsets,
		OffsetPresent: present,
		Sync:          sync,
	}
}

func TestAnalyzeBFramePresentationOrder(t *testing.T) {
	// I P B B decode order. Negative ctts offsets are valid for B-frames.
	tr := testTrack(
		[]uint32{3000, 3000, 3000, 3000},
		[]int64{0, 6000, -3000, -3000},
	)
	got := analyzeTrack(tr, timelineOptions{LongFactor: 1.5})

	wantPTS := []int64{0, 9000, 3000, 6000}
	wantDTS := []int64{0, 3000, 6000, 9000}
	wantOrder := []int{1, 4, 2, 3}
	for i, fr := range got.Frames {
		if fr.PTS != wantPTS[i] || fr.DTS != wantDTS[i] {
			t.Fatalf("sample %d: got PTS/DTS %d/%d, want %d/%d",
				i+1, fr.PTS, fr.DTS, wantPTS[i], wantDTS[i])
		}
		if fr.DisplayOrder != wantOrder[i] {
			t.Fatalf("sample %d: got display order %d, want %d", i+1, fr.DisplayOrder, wantOrder[i])
		}
		if fr.DisplayEnd-fr.DisplayStart != 3000 {
			t.Fatalf("sample %d: display interval = %d, want 3000", i+1, fr.DisplayEnd-fr.DisplayStart)
		}
		if fr.Reason != reasonNormal {
			t.Fatalf("sample %d: reason = %q, want normal", i+1, fr.Reason)
		}
	}
}

func TestLongDisplayWithoutDecodeGap(t *testing.T) {
	// DTS is regular, but ctts leaves a hole in composition time.
	tr := testTrack(
		[]uint32{3000, 3000, 3000},
		[]int64{0, 0, 9000},
	)
	got := analyzeTrack(tr, timelineOptions{LongFactor: 1.5})
	fr := got.Frames[1]
	if fr.PTS != 3000 || fr.DisplayEnd != 15000 {
		t.Fatalf("sample 2: got PTS=%d end=%d", fr.PTS, fr.DisplayEnd)
	}
	if fr.Reason != reasonLongDisplay {
		t.Fatalf("sample 2: reason = %q, want %q", fr.Reason, reasonLongDisplay)
	}
	if strings.Contains(fr.Reason, reasonDecodeGap) {
		t.Fatalf("sample 2 must not have a decode gap: %q", fr.Reason)
	}
}

func TestDecodeGapAndLongDisplay(t *testing.T) {
	tr := testTrack(
		[]uint32{3000, 9000, 3000},
		[]int64{0, 0, 0},
	)
	got := analyzeTrack(tr, timelineOptions{LongFactor: 1.5})
	fr := got.Frames[1]
	if fr.Reason != reasonLongDisplay+"|"+reasonDecodeGap {
		t.Fatalf("sample 2: reason = %q, want long_display|decode_gap", fr.Reason)
	}
	if !strings.Contains(fr.Detail, "stts delta 9000") {
		t.Fatalf("sample 2 detail must name the stts cause: %q", fr.Detail)
	}
}

func TestDuplicatePTSHasSingleDisplayOwner(t *testing.T) {
	tr := testTrack(
		[]uint32{3000, 3000, 3000},
		[]int64{3000, 0, 3000},
	)
	got := analyzeTrack(tr, timelineOptions{LongFactor: 1.5})
	first, second := got.Frames[0], got.Frames[1]
	if first.PTS != second.PTS {
		t.Fatalf("fixture PTS mismatch: %d != %d", first.PTS, second.PTS)
	}
	if first.DisplayEnd != first.DisplayStart {
		t.Fatalf("superseded sample has interval %d..%d", first.DisplayStart, first.DisplayEnd)
	}
	if !strings.Contains(first.Reason, reasonRepeatPTS) {
		t.Fatalf("sample 1 reason = %q, want repeat_pts", first.Reason)
	}
	if second.DisplayEnd != got.Frames[2].PTS {
		t.Fatalf("winner interval ends at %d, want %d", second.DisplayEnd, got.Frames[2].PTS)
	}
	if !strings.Contains(second.Detail, "last decode-order sample owns") {
		t.Fatalf("winner detail = %q", second.Detail)
	}
}

func TestMissingCTTSFallsBackToDTS(t *testing.T) {
	tr := testTrack([]uint32{3000, 3000}, []int64{0, 0})
	tr.OffsetPresent[1] = false
	got := analyzeTrack(tr, timelineOptions{LongFactor: 1.5})
	if !strings.Contains(got.Frames[1].Reason, reasonMissingCTTS) {
		t.Fatalf("sample 2 reason = %q, want ctts_missing", got.Frames[1].Reason)
	}
	if got.Frames[1].PTS != got.Frames[1].DTS {
		t.Fatalf("PTS = %d, DTS = %d", got.Frames[1].PTS, got.Frames[1].DTS)
	}
}

func TestNominalDurationUsesMode(t *testing.T) {
	durations := []uint32{1001, 1001, 2002, 1001, 3003}
	if got := nominalDuration(durations); got != 1001 {
		t.Fatalf("nominal = %d, want 1001", got)
	}
}
