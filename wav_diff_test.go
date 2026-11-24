package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func buildWAV(t *testing.T, pcm []byte, extraBeforeData []byte, mutateDataAt int) []byte {
	t.Helper()
	fmtPayload := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtPayload[0:2], 1) // PCM
	binary.LittleEndian.PutUint16(fmtPayload[2:4], 1) // mono
	binary.LittleEndian.PutUint32(fmtPayload[4:8], 8000)
	binary.LittleEndian.PutUint32(fmtPayload[8:12], 16000)
	binary.LittleEndian.PutUint16(fmtPayload[12:14], 2)
	binary.LittleEndian.PutUint16(fmtPayload[14:16], 16)

	if mutateDataAt >= 0 && mutateDataAt < len(pcm) {
		pcm[mutateDataAt] ^= 0x01
	}

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	riffSizePos := buf.Len()
	buf.Write([]byte{0, 0, 0, 0})
	buf.WriteString("WAVE")
	writeRIFFChunk(&buf, "fmt ", fmtPayload)
	if extraBeforeData != nil {
		writeRIFFChunk(&buf, "JUNK", extraBeforeData)
	}
	writeRIFFChunk(&buf, "data", pcm)
	out := buf.Bytes()
	binary.LittleEndian.PutUint32(out[riffSizePos:riffSizePos+4], uint32(len(out)-8))
	return out
}

func writeRIFFChunk(buf *bytes.Buffer, id string, payload []byte) {
	buf.WriteString(id)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	buf.Write(size[:])
	buf.Write(payload)
	if len(payload)%2 == 1 {
		buf.WriteByte(0)
	}
}

func testArchive(t *testing.T, data []byte, opts Options) *Archive {
	t.Helper()
	manifest, err := BuildManifest(bytes.NewReader(data), int64(len(data)), "test", opts)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	return &Archive{Reader: bytes.NewReader(data), Size: int64(len(data)), Manifest: manifest}
}

func TestWAVMetadataShiftAndPCMLocalization(t *testing.T) {
	pcm := bytes.Repeat([]byte{0x10, 0x00}, 8)
	original := buildWAV(t, append([]byte(nil), pcm...), nil, -1)
	remuxed := buildWAV(t, append([]byte(nil), pcm...), make([]byte, 12), -1)

	opts := Options{BlockSize: 64, AudioIntervalSamples: 4}
	report, err := CompareArchives(testArchive(t, original, opts), testArchive(t, remuxed, opts))
	if err != nil {
		t.Fatal(err)
	}
	if !report.MediaIdentical {
		t.Fatalf("metadata/JUNK shift caused media false positive: %+v", report.Changes)
	}
	if report.WholeFileIdentical {
		t.Fatal("expected container files to differ")
	}
	if len(report.IgnoredChanges) == 0 {
		t.Fatal("expected JUNK/ancillary chunk difference to be recorded")
	}

	mutated := buildWAV(t, append([]byte(nil), pcm...), make([]byte, 20), 10)
	report, err = CompareArchives(testArchive(t, original, opts), testArchive(t, mutated, opts))
	if err != nil {
		t.Fatal(err)
	}
	if report.MediaIdentical {
		t.Fatal("expected PCM mutation to be detected")
	}
	if report.EarliestMediaChange == nil {
		t.Fatal("missing earliest media change")
	}
	change := report.EarliestMediaChange
	if change.Layer != LayerAudioInterval && change.Layer != LayerSample {
		t.Fatalf("unexpected earliest layer %q", change.Layer)
	}
	if change.RelativeOffset != 2 {
		t.Fatalf("relative interval byte = %d, want 2", change.RelativeOffset)
	}
	if change.Media == nil || change.Media.IntervalIndex != 2 {
		t.Fatalf("expected audio interval 2, got %+v", change.Media)
	}
	if change.Media.EarliestSample != 5 {
		t.Fatalf("earliest PCM frame = %d, want 5", change.Media.EarliestSample)
	}
}
