package compare

import (
	"testing"

	"ffv1compare/internal/testutil"
)

func load(t *testing.T, name, hashes string) *Capture {
	t.Helper()
	c, err := LoadCapture(testutil.DataPath(name), testutil.DataPath(hashes))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestIdenticalCaptures(t *testing.T) {
	a := load(t, "machine-a.mkv", "hashes-a.framemd5")
	b := load(t, "machine-a-copy.mkv", "hashes-a.framemd5")
	res := Compare(a, b)
	if res.Verdict != "no_difference" {
		t.Fatalf("verdict=%s reasons=%v", res.Verdict, res.Reasons)
	}
	if res.Counts[VerdictIdentical] != len(res.Frames) {
		t.Fatalf("counts=%v", res.Counts)
	}
}

func TestDecodingDifferenceSameBitstream(t *testing.T) {
	a := load(t, "machine-a.mkv", "hashes-a.framemd5")
	b := load(t, "machine-a-copy.mkv", "hashes-a-rgb.framemd5")
	res := Compare(a, b)
	if res.Verdict != "decoding_difference" {
		t.Fatalf("verdict=%s reasons=%v", res.Verdict, res.Reasons)
	}
	if res.Counts[VerdictDecoding] != len(res.Frames) {
		t.Fatalf("counts=%v", res.Counts)
	}
}

func TestEncodingDifferenceResliced(t *testing.T) {
	a := load(t, "machine-a.mkv", "hashes-a.framemd5")
	b := load(t, "machine-b-slices.mkv", "hashes-b-slices.framemd5")
	res := Compare(a, b)
	if res.Verdict != "encoding_difference" {
		t.Fatalf("verdict=%s reasons=%v", res.Verdict, res.Reasons)
	}
	if a.Config.NumHSlices == b.Config.NumHSlices && a.Config.NumVSlices == b.Config.NumVSlices {
		t.Fatal("fixture should change the slice raster")
	}
	for _, f := range res.Frames {
		if f.A.DecodedHash != f.B.DecodedHash {
			t.Fatalf("frame %d decoded hash should match", f.Index)
		}
	}
}

func TestEncodingDifferenceSliceOrder(t *testing.T) {
	a := load(t, "machine-a.mkv", "hashes-a.framemd5")
	b := load(t, "machine-b-reorder.mkv", "hashes-b-reorder.framemd5")
	res := Compare(a, b)
	if res.Verdict != "encoding_difference" {
		t.Fatalf("verdict=%s reasons=%v", res.Verdict, res.Reasons)
	}
	if res.Counts[VerdictEncodingSliceOrder] != len(res.Frames) {
		t.Fatalf("counts=%v", res.Counts)
	}
	for _, f := range res.Frames {
		if f.A.SliceSetSHA256 != f.B.SliceSetSHA256 {
			t.Fatalf("frame %d slice multiset differs", f.Index)
		}
		if f.A.SliceSequenceSHA256 == f.B.SliceSequenceSHA256 {
			t.Fatalf("frame %d slice sequence should differ", f.Index)
		}
	}
}

func TestEncodingDifferencePixelFormat(t *testing.T) {
	a := load(t, "machine-a.mkv", "hashes-a.framemd5")
	b := load(t, "machine-b-444.mkv", "hashes-b-444.framemd5")
	res := Compare(a, b)
	if res.Verdict != "encoding_difference" {
		t.Fatalf("verdict=%s reasons=%v", res.Verdict, res.Reasons)
	}
	if a.Config.PixelFormat == b.Config.PixelFormat {
		t.Fatal("fixture should change pixel format")
	}
	if res.Counts[VerdictEncodingPixelFormat] != len(res.Frames) {
		t.Fatalf("counts=%v", res.Counts)
	}
}

func TestTimestampDifference(t *testing.T) {
	a := load(t, "machine-a.mkv", "hashes-a.framemd5")
	b := load(t, "machine-b-shift.mkv", "hashes-b-shift.framemd5")
	res := Compare(a, b)
	if res.Verdict != "container_timestamp_difference" {
		t.Fatalf("verdict=%s reasons=%v", res.Verdict, res.Reasons)
	}
	for _, f := range res.Frames {
		if f.PTSDeltaNS != 500_000_000 {
			t.Fatalf("frame %d PTS delta=%d", f.Index, f.PTSDeltaNS)
		}
	}
}
