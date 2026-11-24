package repair_test

import (
	"path/filepath"
	"testing"

	"mp4index/fixture"
	"mp4index/movie"
	"mp4index/repair"
)

// Group 1: truncated file / truncated final sample.
func TestTruncatedGroup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "fast.mp4")
	g, err := fixture.Build(fixture.Config{Path: src, Frames: 24, GOP: 8, MoovFirst: true, WithAUD: true})
	if err != nil {
		t.Fatal(err)
	}

	const keepFull = 14
	cut := g.Samples[keepFull].Offset + int64(g.Samples[keepFull].Size/2)
	damaged := filepath.Join(dir, "truncated.mp4")
	if err := fixture.TruncateCopy(src, damaged, cut); err != nil {
		t.Fatal(err)
	}

	plan, err := repair.Analyze(damaged, repair.Options{FPS: 15})
	if err != nil {
		t.Fatal(err)
	}
	rep := plan.Report
	if rep.ScannedSamples != keepFull {
		t.Fatalf("truncated: scanned %d complete samples, want %d", rep.ScannedSamples, keepFull)
	}
	if !rep.Damage.Truncated {
		t.Fatalf("expected truncation flag, damage=%+v", rep.Damage)
	}

	var sawTail, sawMissing bool
	for _, u := range rep.Unrecovered {
		if u.Kind == "truncated_tail" && u.Length > 0 {
			sawTail = true
		}
		if u.Kind == "missing_index_samples" && u.MissingSamples == 24-keepFull {
			sawMissing = true
		}
	}
	if !sawTail {
		t.Fatalf("missing truncated_tail report: %+v", rep.Unrecovered)
	}
	if !sawMissing {
		t.Fatalf("missing missing_index_samples report: %+v", rep.Unrecovered)
	}
	if len(rep.Intervals) != 1 || rep.Intervals[0].LastSample != keepFull {
		t.Fatalf("interval should cover samples 1..%d, got %+v", keepFull, rep.Intervals)
	}

	srcHash := hashFile(t, damaged)
	rep2, dst := repairTo(t, damaged, repair.Options{FPS: 15})
	if rep2.IndexedSamples != keepFull {
		t.Fatalf("repaired indexed = %d, want %d", rep2.IndexedSamples, keepFull)
	}
	if hashFile(t, damaged) != srcHash {
		t.Fatal("truncated source was modified")
	}
	if got, want := mdatPayload(t, dst), mdatPayload(t, damaged); string(got) != string(want) {
		t.Fatal("truncated repair changed/copied a different payload prefix")
	}
	assertOffsetsAddressNALs(t, dst)
}

// Group 2a: duplicated records baked into the old index.
func TestDuplicateIndexGroupBuiltin(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dup-index.mp4")
	_, err := fixture.Build(fixture.Config{
		Path: src, Frames: 24, GOP: 8, MoovFirst: true,
		WithAUD: true, DupAfter: []int{3, 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	plan, err := repair.Analyze(src, repair.Options{FPS: 15})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Report.ScannedSamples != 24 {
		t.Fatalf("scanned %d, want truth 24", plan.Report.ScannedSamples)
	}
	if plan.Report.Damage.IndexDuplicateRecords < 2 {
		t.Fatalf("expected >=2 duplicate records, damage=%+v", plan.Report.Damage)
	}

	srcHash := hashFile(t, src)
	r, dst := repairTo(t, src, repair.Options{FPS: 15})
	if r.IndexedSamples != 24 {
		t.Fatalf("rebuilt count %d, want 24 (duplicates removed by rescan truth)", r.IndexedSamples)
	}
	if hashFile(t, src) != srcHash {
		t.Fatal("source modified")
	}
	if got, want := mdatPayload(t, dst), mdatPayload(t, src); string(got) != string(want) {
		t.Fatal("payload changed")
	}
	assertOffsetsAddressNALs(t, dst)
}

// Group 2b: generic duplicate-record surgery on a moov-last file.
func TestDuplicateIndexGroupSurgery(t *testing.T) {
	dir := t.TempDir()
	healthy := filepath.Join(dir, "late.mp4")
	if _, err := fixture.Build(fixture.Config{Path: healthy, Frames: 18, GOP: 6, MoovFirst: false}); err != nil {
		t.Fatal(err)
	}
	dup := filepath.Join(dir, "dup-surgery.mp4")
	if err := fixture.DuplicateIndexRecords(healthy, dup, []int{4, 9, 13}); err != nil {
		t.Fatal(err)
	}
	rep, err := repair.Analyze(dup, repair.Options{FPS: 15})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Report.ScannedSamples != 18 {
		t.Fatalf("scanned %d want 18", rep.Report.ScannedSamples)
	}
	if rep.Report.Damage.IndexDuplicateOffsets < 3 {
		t.Fatalf("expected duplicate offsets, %+v", rep.Report.Damage)
	}
	_, dst := repairTo(t, dup, repair.Options{FPS: 15})
	assertOffsetsAddressNALs(t, dst)
	if got, want := mdatPayload(t, dst), mdatPayload(t, dup); string(got) != string(want) {
		t.Fatal("payload changed")
	}
}

// Group 2c: payload level consecutive duplicate frames, detection + dedupe.
func TestDuplicateStreamFrames(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dup-frames.mp4")
	if _, err := fixture.Build(fixture.Config{
		Path: src, Frames: 20, GOP: 10, RepeatFrames: []int{5, 6},
	}); err != nil {
		t.Fatal(err)
	}
	rep, err := repair.Analyze(src, repair.Options{FPS: 15})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Report.Damage.StreamDuplicateSamples < 2 {
		t.Fatalf("expected >=2 stream duplicates, got %+v", rep.Report.Damage)
	}

	r1, dst1 := repairTo(t, src, repair.Options{FPS: 15})
	if r1.IndexedSamples != 20 {
		t.Fatalf("default must keep all payload samples indexed, got %d", r1.IndexedSamples)
	}
	if got, want := mdatPayload(t, dst1), mdatPayload(t, src); string(got) != string(want) {
		t.Fatal("default repair payload changed")
	}

	dst2 := filepath.Join(t.TempDir(), "dedup.mp4")
	r2, err := repair.RepairFile(src, dst2, repair.Options{FPS: 15, DedupeFrames: true})
	if err != nil {
		t.Fatal(err)
	}
	if r2.IndexedSamples != 18 {
		t.Fatalf("dedupe indexed = %d, want 18", r2.IndexedSamples)
	}
	if got, want := mdatPayload(t, dst2), mdatPayload(t, src); string(got) != string(want) {
		t.Fatal("dedupe must not remove or alter payload bytes")
	}
}

// Group 3: wrong chunk offsets in stco.
func TestBadOffsetsGroup(t *testing.T) {
	dir := t.TempDir()
	healthy := filepath.Join(dir, "healthy.mp4")
	if _, err := fixture.Build(fixture.Config{Path: healthy, Frames: 24, GOP: 8, MoovFirst: true}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "badoffsets.mp4")
	n, err := fixture.OffsetAllSTCO(healthy, bad, 97)
	if err != nil {
		t.Fatal(err)
	}
	if n != 24 {
		t.Fatalf("patched %d offsets, want 24", n)
	}

	rep, err := repair.Analyze(bad, repair.Options{FPS: 15})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Report.Damage.BadOffsets != 24 {
		t.Fatalf("bad offsets = %d, want 24; damage=%+v", rep.Report.Damage.BadOffsets, rep.Report.Damage)
	}

	srcHash := hashFile(t, bad)
	r, dst := repairTo(t, bad, repair.Options{FPS: 15})
	if r.IndexedSamples != 24 {
		t.Fatalf("indexed = %d", r.IndexedSamples)
	}
	if hashFile(t, bad) != srcHash {
		t.Fatal("bad-offset source modified")
	}
	if got, want := mdatPayload(t, dst), mdatPayload(t, bad); string(got) != string(want) {
		t.Fatal("payload changed while fixing offsets")
	}
	assertOffsetsAddressNALs(t, dst)
}

// assertOffsetsAddressNALs verifies every rebuilt stco offset points at a
// length prefix whose NAL size matches the rebuilt stsz entry.
func assertOffsetsAddressNALs(t *testing.T, path string) {
	t.Helper()
	f, info, err := movie.OpenAndParse(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	idx := info.Video.Index
	ls := info.Video.LengthSize
	if ls == 0 {
		ls = 4
	}
	for i, off := range idx.Offsets {
		remaining := int64(idx.Sizes[i])
		pos := int64(off)
		nals := 0
		lbuf := make([]byte, ls)
		for remaining > 0 {
			if remaining < int64(ls)+1 {
				t.Fatalf("sample %d: %d trailing bytes too small for NAL", i, remaining)
			}
			if _, err := f.ReadAt(lbuf, pos); err != nil {
				t.Fatalf("sample %d: read length prefix: %v", i, err)
			}
			var nalLen int64
			for _, b := range lbuf {
				nalLen = nalLen<<8 | int64(b)
			}
			wire := int64(ls) + nalLen
			if nalLen < 1 || wire > remaining {
				t.Fatalf("sample %d at %d: invalid NAL length %d (remaining %d)", i, pos, nalLen, remaining)
			}
			var hdr [1]byte
			if _, err := f.ReadAt(hdr[:1], pos+int64(ls)); err != nil {
				t.Fatal(err)
			}
			if hdr[0]&0x80 != 0 {
				t.Fatalf("sample %d NAL header has forbidden bit set", i)
			}
			pos += wire
			remaining -= wire
			nals++
		}
		if nals == 0 {
			t.Fatalf("sample %d has no NAL units", i)
		}
	}
}
