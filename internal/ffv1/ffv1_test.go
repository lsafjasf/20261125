package ffv1

import (
	"bytes"
	"encoding/binary"
	"testing"

	"ffv1compare/internal/mkv"
	"ffv1compare/internal/testutil"
)

func TestCRC32RawSelfCheck(t *testing.T) {
	data := []byte("FFV1 archive fixity")
	crc := CRC32Raw(data)
	var tail [4]byte
	binary.BigEndian.PutUint32(tail[:], crc)
	whole := append(append([]byte(nil), data...), tail[:]...)
	if CRC32Raw(whole) != 0 {
		t.Fatalf("CRC over data+stored CRC = %08x, want 0", CRC32Raw(whole))
	}
}

func loadFrameSamples(t *testing.T, name string) (*Config, []FrameSample) {
	t.Helper()
	doc, err := mkv.ParseFile(testutil.DataPath(name))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Tracks) == 0 {
		t.Fatal("no tracks")
	}
	cfg := ParseConfig(doc.Tracks[0].CodecID, doc.Tracks[0].CodecPrivate)
	var out []FrameSample
	for _, f := range doc.Frames {
		out = append(out, FrameSample{Sample: f.Sample, Track: f.TrackNumber})
	}
	return &cfg, out
}

type FrameSample struct {
	Sample []byte
	Track  uint64
}

func TestParseConfigMachineA(t *testing.T) {
	cfg, _ := loadFrameSamples(t, "machine-a.mkv")
	if !cfg.Parsed {
		t.Fatalf("config not parsed: %s", cfg.Error)
	}
	if !cfg.CRCOK {
		t.Fatalf("config CRC mismatch: stored=%08x calculated=%08x", cfg.CRCStored, cfg.CRCCalculated)
	}
	if cfg.Version != 3 || cfg.MicroVersion != 4 || cfg.CoderType != 0 {
		t.Fatalf("header=%+v", cfg)
	}
	if cfg.PixelFormat != "yuv420p" || cfg.BitsPerRawSample != 8 {
		t.Fatalf("pixfmt=%s bits=%d", cfg.PixelFormat, cfg.BitsPerRawSample)
	}
	if cfg.NumHSlices != 2 || cfg.NumVSlices != 2 {
		t.Fatalf("slices=%dx%d", cfg.NumHSlices, cfg.NumVSlices)
	}
}

func TestParseConfigRangeCoder10Bit(t *testing.T) {
	doc, err := mkv.ParseFile(testutil.DataPath("ffv1-10bit.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := ParseConfig(doc.Tracks[0].CodecID, doc.Tracks[0].CodecPrivate)
	if !cfg.Parsed || !cfg.CRCOK {
		t.Fatalf("config=%+v error=%s", cfg, cfg.Error)
	}
	if cfg.CoderType != 2 || cfg.BitsPerRawSample != 10 || cfg.PixelFormat != "yuv420p10" {
		t.Fatalf("config=%+v", cfg)
	}
	if cfg.NumHSlices != 1 || cfg.NumVSlices != 1 {
		t.Fatalf("slices=%dx%d", cfg.NumHSlices, cfg.NumVSlices)
	}
}

func TestSliceChainAndOrder(t *testing.T) {
	cfgA, framesA := loadFrameSamples(t, "machine-a.mkv")
	_, framesB := loadFrameSamples(t, "machine-b-reorder.mkv")
	if !cfgA.Parsed || len(framesA) == 0 || len(framesB) == 0 {
		t.Fatal("fixtures not parsed")
	}
	expected := cfgA.NumHSlices * cfgA.NumVSlices
	a := AnalyzeSlices(framesA[0].Sample, expected)
	b := AnalyzeSlices(framesB[0].Sample, expected)
	if a.Count != expected || b.Count != expected {
		t.Fatalf("slice counts A=%d B=%d expected=%d", a.Count, b.Count, expected)
	}
	if !a.CRCEnabled || !a.CRCOK || !b.CRCEnabled || !b.CRCOK {
		t.Fatalf("CRC status A=%+v B=%+v", a, b)
	}
	if a.SetSHA256 != b.SetSHA256 {
		t.Fatal("reordered frame should have the same slice multiset")
	}
	if a.SequenceSHA256 == b.SequenceSHA256 {
		t.Fatal("reordered frame should have a different slice sequence")
	}
}

func TestSliceChainWithoutCRC(t *testing.T) {
	cfg, frames := loadFrameSamples(t, "machine-b-slices.mkv")
	if !cfg.Parsed || len(frames) == 0 {
		t.Fatal("fixture not parsed")
	}
	r := AnalyzeSlices(frames[0].Sample, cfg.NumHSlices*cfg.NumVSlices)
	if r.Count != 1 || r.CRCEnabled {
		t.Fatalf("slice report=%+v", r)
	}
	if r.SetSHA256 == "" || r.SequenceSHA256 == "" {
		t.Fatal("missing slice hashes")
	}
}

func TestReorderedSlicesAreSamePayloads(t *testing.T) {
	_, framesA := loadFrameSamples(t, "machine-a.mkv")
	_, framesB := loadFrameSamples(t, "machine-b-reorder.mkv")
	if len(framesA[0].Sample) != len(framesB[0].Sample) {
		t.Fatal("reorder fixture changed sample size")
	}
	if bytes.Equal(framesA[0].Sample, framesB[0].Sample) {
		t.Fatal("reorder fixture is byte-identical to A")
	}
}

func TestSliceCRCFailureIsReported(t *testing.T) {
	cfg, frames := loadFrameSamples(t, "machine-a.mkv")
	if !cfg.Parsed {
		t.Fatal(cfg.Error)
	}
	sample := append([]byte(nil), frames[0].Sample...)
	// Byte 10 is inside the first slice payload, after the frame/slice header
	// and well before the footer chain.
	sample[10] ^= 0x01
	r := AnalyzeSlices(sample, cfg.NumHSlices*cfg.NumVSlices)
	if !r.CRCEnabled {
		t.Fatalf("CRC should still be detected: %+v", r)
	}
	if r.CRCOK {
		t.Fatal("corrupted slice CRC was reported as valid")
	}
	if r.Count != cfg.NumHSlices*cfg.NumVSlices {
		t.Fatalf("slice count=%d", r.Count)
	}
}
