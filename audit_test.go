package main

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"
)

type synthDirs struct {
	ref, a, b string
}

func setupSynth(t *testing.T, frames int) synthDirs {
	t.Helper()
	root := t.TempDir()
	d := synthDirs{
		ref: filepath.Join(root, "ref"),
		a:   filepath.Join(root, "a"),
		b:   filepath.Join(root, "b"),
	}
	for _, p := range []string{d.ref, d.a, d.b} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < frames; i++ {
		name := frameName(i)
		base := baseFrame()
		writePNG(t, d.ref, name, base)
		writePNG(t, d.a, name, withMildNoise(base))
		writePNG(t, d.b, name, withBanding(base))
	}
	return d
}

func frameName(i int) string {
	return "frame_" + twoDigits(i) + ".png"
}

func twoDigits(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

func TestNaturalSort(t *testing.T) {
	names := []string{"frame_2.png", "frame_10.png", "frame_1.png", "frame_20.png"}
	got := make([]string, len(names))
	copy(got, names)
	// sort via naturalLess
	for i := 0; i < len(got); i++ {
		for j := i + 1; j < len(got); j++ {
			if naturalLess(got[j], got[i]) {
				got[i], got[j] = got[j], got[i]
			}
		}
	}
	want := []string{"frame_1.png", "frame_2.png", "frame_10.png", "frame_20.png"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("natural sort = %v, want %v", got, want)
		}
	}
}

func TestClassifiesConversionBanding(t *testing.T) {
	base := baseFrame()
	ref := mustLoadImage(t, base)
	coded := mustLoadImage(t, withBanding(base))
	fs := analyzeFrame(ref, coded, "B")

	if fs.BandSteps == 0 {
		t.Fatalf("expected band steps in dark smooth region, got 0")
	}
	if fs.DarkSmoothPx == 0 {
		t.Fatalf("expected dark smooth pixels, got 0")
	}
	conversion := fs.TileCounts[classConversion] + fs.TileCounts[classMixed]
	if conversion == 0 {
		t.Fatalf("expected at least one conversion/mixed tile, counts=%v", fs.TileCounts)
	}
	for _, tile := range fs.Tiles {
		if tile.Class == classCoding {
			t.Fatalf("banding-only frame must not be labeled coding: %+v", tile)
		}
	}
}

func TestClassifiesCodingBlocks(t *testing.T) {
	base := baseFrame()
	ref := mustLoadImage(t, base)
	coded := mustLoadImage(t, withBlockArtifacts(base))
	fs := analyzeFrame(ref, coded, "A")

	if fs.BlockScore < blockAlignMin {
		t.Fatalf("block score %.3f below threshold %.2f", fs.BlockScore, blockAlignMin)
	}
	coding := fs.TileCounts[classCoding] + fs.TileCounts[classMixed]
	if coding == 0 {
		t.Fatalf("expected at least one coding/mixed tile, counts=%v", fs.TileCounts)
	}
}

func TestMildNoiseHasNoStructuredArtifact(t *testing.T) {
	base := baseFrame()
	ref := mustLoadImage(t, base)
	coded := mustLoadImage(t, withMildNoise(base))
	fs := analyzeFrame(ref, coded, "A")

	structured := fs.TileCounts[classCoding] + fs.TileCounts[classConversion] + fs.TileCounts[classMixed]
	if structured != 0 {
		t.Fatalf("mild noise produced structured labels: %v", fs.TileCounts)
	}
	if fs.PSNRY < 35 {
		t.Fatalf("mild noise PSNR unexpectedly low: %.2f", fs.PSNRY)
	}
}

func TestEndToEndDeterministic(t *testing.T) {
	d := setupSynth(t, 11) // double digits exercise natural ordering
	out1 := filepath.Join(t.TempDir(), "run1")
	args := []string{
		"-ref", d.ref, "-a", d.a, "-b", d.b,
		"-label-a", "paramA", "-label-b", "paramB",
		"-out", out1, "-top", "5",
		"-now", "2026-11-25T08:00:00Z",
	}
	if err := run(args); err != nil {
		t.Fatal(err)
	}

	rep1 := readReport(t, filepath.Join(out1, "report.json"))
	if rep1.RunID != "run_20261125T080000Z" {
		t.Fatalf("unexpected run id %s", rep1.RunID)
	}
	if len(rep1.Frames) != 11 {
		t.Fatalf("frames=%d", len(rep1.Frames))
	}
	if rep1.Frames[0].Frame != "frame_00.png" || rep1.Frames[10].Frame != "frame_10.png" {
		t.Fatalf("frame order wrong: %s ... %s", rep1.Frames[0].Frame, rep1.Frames[10].Frame)
	}
	if len(rep1.Inspection) == 0 {
		t.Fatal("expected inspection items for banding set B")
	}
	if rep1.Inspection[0].Set != "paramB" {
		t.Fatalf("top inspection item should come from banding set, got %s", rep1.Inspection[0].Set)
	}
	if rep1.Inspection[0].Class != classConversion {
		t.Fatalf("top item class = %s, want conversion", rep1.Inspection[0].Class)
	}
	if rep1.SummaryB.TilesConversion == 0 {
		t.Fatal("set B summary missing conversion tiles")
	}

	// Rerun into a second directory with the same fixed time and assert the
	// reports and inspection CSVs are byte-identical (reproducibility).
	out2 := filepath.Join(t.TempDir(), "run2")
	args2 := make([]string, len(args))
	copy(args2, args)
	for i, v := range args2 {
		if v == out1 {
			args2[i] = out2
		}
	}
	if err := run(args2); err != nil {
		t.Fatal(err)
	}
	assertSameBytes(t, filepath.Join(out1, "report.json"), filepath.Join(out2, "report.json"))
	assertSameBytes(t, filepath.Join(out1, "inspection.csv"), filepath.Join(out2, "inspection.csv"))

	man := readManifest(t, filepath.Join(out1, "manifest.json"))
	if len(man.Inputs) != 33 {
		t.Fatalf("manifest inputs=%d, want 33", len(man.Inputs))
	}
	if len(man.FrameOrder) != 11 || man.FrameOrder[10] != "frame_10.png" {
		t.Fatalf("manifest frame order wrong: %v", man.FrameOrder)
	}
	for _, fp := range man.Inputs {
		if len(fp.SHA256) != 64 {
			t.Fatalf("bad fingerprint: %+v", fp)
		}
	}
	if rep1.NextCheck == "" || rep1.NextCheck == buildNextCheck(nil) {
		t.Fatal("next-check guidance missing")
	}
}

func TestFrameCountMismatch(t *testing.T) {
	d := setupSynth(t, 2)
	if err := os.Remove(filepath.Join(d.b, "frame_01.png")); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"-ref", d.ref, "-a", d.a, "-b", d.b,
		"-out", filepath.Join(t.TempDir(), "x"),
		"-now", "2026-11-25T08:00:00Z"})
	if err == nil {
		t.Fatal("expected frame count mismatch error")
	}
}

func mustLoadImage(t *testing.T, img *image.RGBA) *Frame {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.png")
	writePNG(t, dir, "f.png", img)
	f, err := loadFrame(path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func readReport(t *testing.T, path string) *report {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var r report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

func readManifest(t *testing.T, path string) *manifest {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

func assertSameBytes(t *testing.T, a, b string) {
	da, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	db, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(da) != string(db) {
		t.Fatalf("files differ: %s vs %s", a, b)
	}
}
