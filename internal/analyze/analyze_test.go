package analyze

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDPX writes a minimal synthetic DPX file (generic header + padding).
func writeDPX(t *testing.T, path string, width, height, framePos uint32, fps float32, tc uint32) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	bo := binary.BigEndian
	copy(buf[0:], "SDPX")
	bo.PutUint32(buf[4:], 4096) // image data offset
	copy(buf[8:], "V2.0")
	copy(buf[36:], filepath.Base(path))
	bo.PutUint32(buf[772:], width)
	bo.PutUint32(buf[776:], height)
	bo.PutUint32(buf[1772:], framePos)
	bo.PutUint32(buf[1776:], 0xFFFFFFFF)
	bo.PutUint32(buf[1784:], math.Float32bits(fps))
	bo.PutUint32(buf[1980:], tc)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

// bcd builds a packed 0xHHMMSSFF timecode word.
func bcd(h, m, s, f int) uint32 {
	d := func(v int) uint32 { return uint32(v/10)<<4 | uint32(v%10) }
	return d(h)<<24 | d(m)<<16 | d(s)<<8 | d(f)
}

func kinds(rep *Report) map[string]int {
	m := map[string]int{}
	for _, is := range rep.Issues {
		m[is.Kind]++
	}
	return m
}

func TestScanReconstructsGlobalOrder(t *testing.T) {
	root := t.TempDir()
	// Reel A: TC 01:00:00:00 + 4 frames @24fps, framepos restarts at 1.
	for i := 0; i < 4; i++ {
		writeDPX(t, filepath.Join(root, "reelA", fmt.Sprintf("zz_%04d.dpx", 99-i)),
			2048, 1556, uint32(i+1), 24, bcd(1, 0, 0, i))
	}
	// Reel B: TC rolls back to 00:59:00:00 after the reel change,
	// framepos restarts at 1; one duplicate key and one gap.
	writeDPX(t, filepath.Join(root, "reelB", "a_0001.dpx"), 2048, 1556, 1, 24, bcd(0, 59, 0, 0))
	writeDPX(t, filepath.Join(root, "reelB", "a_0002.dpx"), 2048, 1556, 2, 24, bcd(0, 59, 0, 1))
	writeDPX(t, filepath.Join(root, "reelB", "a_0002_dup.dpx"), 2048, 1556, 2, 24, bcd(0, 59, 0, 1))
	// gap: one frame missing (keys jump 85561 -> 85563)
	writeDPX(t, filepath.Join(root, "reelB", "a_0004.dpx"), 2048, 1556, 4, 24, bcd(0, 59, 0, 3))
	// resolution change
	writeDPX(t, filepath.Join(root, "reelB", "a_0005.dpx"), 1920, 1080, 5, 24, bcd(0, 59, 0, 4))
	// Explicit reel order: reelB's rolled-back TC would otherwise sort first.
	rep, err := Scan(root, Options{ReelOrder: []string{"reelA", "reelB"}})
	if err != nil {
		t.Fatal(err)
	}
	k := kinds(rep)
	if k["duplicate"] != 1 {
		t.Errorf("want 1 duplicate, got %d (%v)", k["duplicate"], rep.Issues)
	}
	if k["gap"] != 1 {
		t.Errorf("want 1 gap, got %d (%v)", k["gap"], rep.Issues)
	}
	if k["resolution-change"] != 1 {
		t.Errorf("want 1 resolution-change, got %d", k["resolution-change"])
	}
	// TC keys: reelA = 86400..86403, reelB = 85560,85561,(dup),85563,85564.
	if len(rep.ReelOrder) != 2 {
		t.Fatalf("want 2 reels, got %v", rep.ReelOrder)
	}
	// Playable ranges: reelA run, reelB run up to the gap, post-gap frame,
	// then the 1920x1080 frame = 4 ranges.
	if len(rep.Ranges) != 4 {
		t.Fatalf("want 4 ranges, got %d: %+v", len(rep.Ranges), rep.Ranges)
	}
	if got := len(rep.Ranges[0].Frames); got != 4 {
		t.Errorf("range 1: want 4 frames, got %d", got)
	}
	// Global order must follow header keys, not file names: reelA files are
	// named zz_0099..zz_0096 descending but must come out in TC order.
	first := rep.Ranges[0].Frames[0]
	if first.Header.TimeCode.Frame != 0 {
		t.Errorf("first frame should have TC frame 0, got %d", first.Header.TimeCode.Frame)
	}
	joined := ""
	for _, s := range rep.Suggestions {
		joined += s + "\n"
	}
	if !strings.Contains(joined, "rolls back") {
		t.Errorf("expected a timecode rollback suggestion, got:\n%s", joined)
	}
	if !strings.Contains(joined, "missing") {
		t.Errorf("expected a gap suggestion, got:\n%s", joined)
	}
}

func TestReelChainingByTimecode(t *testing.T) {
	root := t.TempDir()
	// Two reels with continuous TC across the boundary: R2 ends at
	// 01:00:00:02, R1 starts at 01:00:00:03.
	for i := 0; i < 3; i++ {
		writeDPX(t, filepath.Join(root, "R2", fmt.Sprintf("f%03d.dpx", i)),
			2048, 1556, uint32(i+1), 24, bcd(1, 0, 0, i))
		writeDPX(t, filepath.Join(root, "R1", fmt.Sprintf("f%03d.dpx", i)),
			2048, 1556, uint32(i+1), 24, bcd(1, 0, 0, i+3))
	}
	rep, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Chaining must put R2 (TC 01:00:00:00) before R1 despite the name.
	if len(rep.ReelOrder) != 2 || rep.ReelOrder[0] != "R2" {
		t.Fatalf("want reel order [R2 R1], got %v", rep.ReelOrder)
	}
	if len(rep.Ranges) != 1 || len(rep.Ranges[0].Frames) != 6 {
		t.Fatalf("want 1 continuous range of 6 frames, got %+v", rep.Ranges)
	}
}

func TestManifestMissingAndChecksum(t *testing.T) {
	root := t.TempDir()
	writeDPX(t, filepath.Join(root, "reel", "f001.dpx"), 2048, 1556, 1, 24, bcd(1, 0, 0, 0))
	man := filepath.Join(root, "delivery.md5")
	content := "d41d8cd98f00b204e9800998ecf8427e  reel/f001.dpx\n" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  reel/f002.dpx\n"
	if err := os.WriteFile(man, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Scan(root, Options{Manifests: []string{man}})
	if err != nil {
		t.Fatal(err)
	}
	k := kinds(rep)
	if k["manifest-missing"] != 1 {
		t.Errorf("want 1 manifest-missing, got %d (%v)", k["manifest-missing"], rep.Issues)
	}
	// With verification on, the wrong checksum must be flagged.
	rep, err = Scan(root, Options{Manifests: []string{man}, VerifyMD5: true})
	if err != nil {
		t.Fatal(err)
	}
	if kinds(rep)["checksum-mismatch"] != 1 {
		t.Errorf("want 1 checksum-mismatch, got %v", rep.Issues)
	}
}
