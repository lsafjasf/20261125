package repair_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"mp4index/fixture"
	"mp4index/repair"
)

// A garbage gap inside mdat must trigger resync, split playable intervals and
// be reported as unrecovered bytes.
func TestGarbageGapSplitsIntervals(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "healthy.mp4")
	g, err := fixture.Build(fixture.Config{
		Path: src, Frames: 24, GOP: 8, MoovFirst: true, WithAUD: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	const gapAtSample = 10 // insert before 0 based sample 10
	gapOff := g.Samples[gapAtSample].Offset
	gapBytes := make([]byte, 48) // all zero: impossible NAL length, then resync
	out := make([]byte, 0, len(raw)+len(gapBytes))
	out = append(out, raw[:gapOff]...)
	out = append(out, gapBytes...)
	out = append(out, raw[gapOff:]...)

	// Patch the mdat box size so top level parsing sees the inserted bytes.
	mdatBoxStart := g.MdatPayloadStart - 8
	binary.BigEndian.PutUint32(out[mdatBoxStart:mdatBoxStart+4],
		binary.BigEndian.Uint32(raw[mdatBoxStart:mdatBoxStart+4])+uint32(len(gapBytes)))

	gapPath := filepath.Join(dir, "gap.mp4")
	if err := os.WriteFile(gapPath, out, 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := repair.Analyze(gapPath, repair.Options{FPS: 15})
	if err != nil {
		t.Fatal(err)
	}
	rep := plan.Report
	if rep.ScannedSamples != 24 {
		t.Fatalf("resync should recover 24 samples, got %d", rep.ScannedSamples)
	}
	var sawGap bool
	for _, u := range rep.Unrecovered {
		if u.Kind == "scan_gap" && u.Length == int64(len(gapBytes)) {
			sawGap = true
		}
	}
	if !sawGap {
		t.Fatalf("expected 48 byte scan_gap, got %+v", rep.Unrecovered)
	}
	if len(rep.Intervals) != 2 {
		t.Fatalf("expected 2 intervals, got %+v", rep.Intervals)
	}
	if rep.Intervals[0].LastSample != gapAtSample {
		t.Fatalf("interval 1 last=%d want %d", rep.Intervals[0].LastSample, gapAtSample)
	}
	second := rep.Intervals[1]
	if second.FirstSample != gapAtSample+1 || second.FirstSyncSample != 17 {
		t.Fatalf("interval 2 bounds wrong: %+v", second)
	}

	_, dst := repairTo(t, gapPath, repair.Options{FPS: 15})
	assertOffsetsAddressNALs(t, dst)
	if got, want := mdatPayload(t, dst), mdatPayload(t, gapPath); string(got) != string(want) {
		t.Fatal("gap bytes must be copied verbatim with payload")
	}
}
