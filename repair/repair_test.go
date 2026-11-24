package repair_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"mp4index/fixture"
	"mp4index/movie"
	"mp4index/repair"
)

func hashFile(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func mdatPayload(t *testing.T, path string) []byte {
	t.Helper()
	f, info, err := movie.OpenAndParse(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	defer f.Close()
	if len(info.Mdats) != 1 {
		t.Fatalf("%s: expected 1 mdat, got %d", path, len(info.Mdats))
	}
	sec := info.Mdats[0]
	buf := make([]byte, sec.PayloadLength)
	if _, err := f.ReadAt(buf, sec.PayloadOffset); err != nil {
		t.Fatalf("read mdat payload: %v", err)
	}
	return buf
}

func repairTo(t *testing.T, src string, opts repair.Options) (*repair.Report, string) {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "repaired.mp4")
	if opts.Output != "" {
		dst = opts.Output
	}
	opts.Output = dst
	rep, err := repair.RepairFile(src, dst, opts)
	if err != nil {
		t.Fatalf("RepairFile(%s): %v", src, err)
	}
	return rep, dst
}

// Healthy synthetic files must scan back to exactly the built ground truth in
// every supported layout / framing combination.
func TestHealthySynthetic(t *testing.T) {
	cases := []struct {
		name      string
		aud       bool
		moovFirst bool
	}{
		{"aud_moovfirst", true, true},
		{"noaud_moovfirst", false, true},
		{"aud_moovlast", true, false},
		{"noaud_moovlast", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "healthy.mp4")
			g, err := fixture.Build(fixture.Config{
				Path: path, Frames: 24, GOP: 8,
				WithAUD: tc.aud, MoovFirst: tc.moovFirst,
			})
			if err != nil {
				t.Fatal(err)
			}

			plan, err := repair.Analyze(path, repair.Options{FPS: 15})
			if err != nil {
				t.Fatal(err)
			}
			rep := plan.Report
			if rep.ScannedSamples != len(g.Samples) {
				t.Fatalf("scanned %d samples, want %d", rep.ScannedSamples, len(g.Samples))
			}
			for i, s := range plan.SamplesInfo() {
				if s.Offset != g.Samples[i].Offset || s.Size != int64(g.Samples[i].Size) {
					t.Fatalf("sample %d = off %d size %d, want off %d size %d",
						i, s.Offset, s.Size, g.Samples[i].Offset, g.Samples[i].Size)
				}
				if s.Keyframe != g.Samples[i].Key {
					t.Fatalf("sample %d key=%v want %v", i, s.Keyframe, g.Samples[i].Key)
				}
			}
			if len(rep.Intervals) != 1 {
				t.Fatalf("intervals = %d, want 1: %+v", len(rep.Intervals), rep.Intervals)
			}
			if len(rep.Unrecovered) != 0 {
				t.Fatalf("unexpected unrecovered: %+v", rep.Unrecovered)
			}
			if rep.Damage.BadOffsets != 0 || rep.Damage.IndexDuplicateRecords != 0 {
				t.Fatalf("healthy file flagged damage: %+v", rep.Damage)
			}

			srcHash := hashFile(t, path)
			rep2, dst := repairTo(t, path, repair.Options{FPS: 15})
			if hashFile(t, path) != srcHash {
				t.Fatal("source file changed after repair")
			}
			if rep2.IndexedSamples != 24 {
				t.Fatalf("indexed = %d", rep2.IndexedSamples)
			}
			if got := mdatPayload(t, dst); string(got) != string(mdatPayload(t, path)) {
				t.Fatal("repaired mdat payload differs from source payload")
			}

			outF, outInfo, err := movie.OpenAndParse(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer outF.Close()
			idx := outInfo.Video.Index
			if int(idx.SampleCount) != 24 || len(idx.Offsets) != 24 {
				t.Fatalf("rebuilt tables count=%d offsets=%d", idx.SampleCount, len(idx.Offsets))
			}
			for i, o := range idx.Offsets {
				end := int64(o) + int64(idx.Sizes[i])
				if end > outInfo.FileSize {
					t.Fatalf("rebuilt sample %d out of bounds: %d > %d", i, end, outInfo.FileSize)
				}
			}
		})
	}
}
