package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func be32(v uint32) []byte {
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], v)
	return out[:]
}

func be64(v uint64) []byte {
	var out [8]byte
	binary.BigEndian.PutUint64(out[:], v)
	return out[:]
}

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func makeBox(typ string, payload []byte) []byte {
	return cat(be32(uint32(8+len(payload))), []byte(typ), payload)
}

func makeFullBox(typ string, version uint8, flags uint32, body []byte) []byte {
	head := []byte{
		version,
		byte(flags >> 16),
		byte(flags >> 8),
		byte(flags),
	}
	return makeBox(typ, cat(head, body))
}

func syntheticMP4(sampleCount uint32, offsets []int64, stss []uint32) []byte {
	tkhd := makeFullBox("tkhd", 0, 0, cat(
		be32(0), be32(0), be32(1),
	))
	mdhd := makeFullBox("mdhd", 0, 0, cat(
		be32(0), be32(0), be32(30000), be32(9000),
	))
	hdlr := makeFullBox("hdlr", 0, 0, cat(
		be32(0), []byte("vide"), make([]byte, 12),
	))
	avc1 := makeBox("avc1", nil)
	stsd := makeFullBox("stsd", 0, 0, cat(be32(1), avc1))
	stts := makeFullBox("stts", 0, 0, cat(
		be32(1), be32(sampleCount), be32(3000),
	))
	var cttsBody []byte
	if offsets != nil {
		cttsBody = append(cttsBody, be32(uint32(len(offsets)))...)
		for _, offset := range offsets {
			cttsBody = append(cttsBody, be32(1)...)
			cttsBody = append(cttsBody, be32(uint32(int32(offset)))...)
		}
	}
	ctts := makeFullBox("ctts", 1, 0, cttsBody)
	var stssBody []byte
	if stss != nil {
		stssBody = append(stssBody, be32(uint32(len(stss)))...)
		for _, sample := range stss {
			stssBody = append(stssBody, be32(sample)...)
		}
	}
	stssBox := makeFullBox("stss", 0, 0, stssBody)
	sizes := make([]byte, 0, int(sampleCount)*4)
	for i := uint32(0); i < sampleCount; i++ {
		sizes = append(sizes, be32(100+i)...)
	}
	stsz := makeFullBox("stsz", 0, 0, cat(
		be32(0), be32(sampleCount), sizes,
	))

	stbl := makeBox("stbl", cat(stsd, stts, ctts, stssBox, stsz))
	minf := makeBox("minf", stbl)
	mdia := makeBox("mdia", cat(mdhd, hdlr, minf))
	trak := makeBox("trak", cat(tkhd, mdia))
	moov := makeBox("moov", trak)
	ftyp := makeBox("ftyp", []byte("isom\x00\x00\x02\x00isom"))
	mdat := makeBox("mdat", []byte("not-decoded"))
	return cat(ftyp, moov, mdat)
}

func TestParseMovieTables(t *testing.T) {
	data := syntheticMP4(3, []int64{0, 6000, -3000}, []uint32{1})
	m, err := parseMovie(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(m.Tracks))
	}
	tr := m.Tracks[0]
	if tr.ID != 1 || tr.Timescale != 30000 || tr.Codec != "avc1" {
		t.Fatalf("unexpected track metadata: id=%d timescale=%d codec=%q",
			tr.ID, tr.Timescale, tr.Codec)
	}
	if tr.SampleCount != 3 {
		t.Fatalf("sample count = %d, want 3", tr.SampleCount)
	}
	if got := tr.Offsets; len(got) != 3 || got[1] != 6000 || got[2] != -3000 {
		t.Fatalf("offsets = %v", got)
	}
	if !tr.Sync[0] || tr.Sync[1] || tr.Sync[2] {
		t.Fatalf("sync = %v, want [true false false]", tr.Sync)
	}
	if len(tr.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", tr.Issues)
	}
}

func TestSTSSValidation(t *testing.T) {
	data := syntheticMP4(3, []int64{0, 0, 0}, []uint32{2, 2, 9})
	m, err := parseMovie(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, is := range m.Tracks[0].Issues {
		codes[is.Code] = true
	}
	if !codes["stss_sample_out_of_range"] || !codes["stss_not_strictly_increasing"] {
		t.Fatalf("issues = %+v", m.Tracks[0].Issues)
	}
}

func TestBuildTrackCountMismatch(t *testing.T) {
	tr := buildTrack(&track{Timescale: 30000}, &sampleTables{
		stts:          []sttsEntry{{count: 2, delta: 3000}},
		cttsPresent:   true,
		ctts:          []cttsEntry{{count: 1, offset: 0}},
		stssPresent:   true,
		stss:          []uint32{1, 3},
		sampleCount:   3,
		sampleCountOK: true,
	})
	codes := map[string]bool{}
	for _, is := range tr.Issues {
		codes[is.Code] = true
	}
	for _, want := range []string{
		"sample_count_mismatch",
		"ctts_count_mismatch",
		"stss_sample_out_of_range",
	} {
		if !codes[want] {
			t.Fatalf("missing issue %q in %+v", want, tr.Issues)
		}
	}
	if !tr.Sync[0] {
		t.Fatal("sample 1 should be a sync sample")
	}
}

func TestRunTSVOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timeline.mp4")
	data := syntheticMP4(3, []int64{0, 0, 0}, []uint32{1})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-format", "tsv", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines:\n%s", len(lines), stdout.String())
	}
	if !strings.HasPrefix(lines[0], "track_id\tsample\tpts\tdts\t") {
		t.Fatalf("header = %q", lines[0])
	}
	if !strings.Contains(lines[1], "\t1\t0\t0\t") {
		t.Fatalf("sample 1 row = %q", lines[1])
	}
	if !strings.HasSuffix(lines[1], "\tnormal\t") {
		t.Fatalf("sample 1 row = %q", lines[1])
	}
}
