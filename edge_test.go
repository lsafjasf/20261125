package repack

import (
	"os"
	"path/filepath"
	"testing"
)

func writeBoxes(t *testing.T, path string, boxes []Box) {
	t.Helper()
	raw, err := EncodePack(boxes)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRejectsMissingTracks(t *testing.T) {
	dir := setupSources(t)
	base := buildSampleBoxes("en")
	removeType := func(tp BoxType) []Box {
		out := make([]Box, 0, len(base))
		for _, b := range base {
			if b.Type != tp {
				out = append(out, b)
			}
		}
		return out
	}
	for _, tp := range []BoxType{TypeVideo, TypeAudio, TypeSubtitle, TypeChapter} {
		path := filepath.Join(dir, "missing-"+tp.String()+".mcpack")
		writeBoxes(t, path, removeType(tp))
		err := Repack(Options{
			Inputs: []string{path},
			Output: filepath.Join(dir, "merged.mcpack"),
			Base:   dir,
		})
		if err == nil {
			t.Fatalf("missing %s: expected rejection", tp)
		}
	}
}

func TestRejectsOverlappingTimeline(t *testing.T) {
	dir := setupSources(t)
	bad := filepath.Join(dir, "overlap.mcpack")
	mutatePack(t, sourcePath(dir, "en"), bad, func(boxes []Box) []Box {
		for i := range boxes {
			if boxes[i].Type == TypeAudio && i > 0 {
				boxes[i].StartUs -= 1
				break
			}
		}
		return boxes
	})
	err := Repack(Options{
		Inputs: []string{bad},
		Output: filepath.Join(dir, "merged.mcpack"),
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected timeline overlap rejection")
	}
}

func TestRejectsAudioSpanDrift(t *testing.T) {
	dir := setupSources(t)
	good := sourcePath(dir, "en")
	bad := filepath.Join(dir, "drift-fr.mcpack")
	mutatePack(t, sourcePath(dir, "fr"), bad, func(boxes []Box) []Box {
		var lastAudio int
		for i := range boxes {
			if boxes[i].Type == TypeAudio {
				lastAudio = i
			}
		}
		boxes[lastAudio].EndUs += 500_000
		return boxes
	})
	err := Repack(Options{
		Inputs: []string{good, bad},
		Output: filepath.Join(dir, "merged.mcpack"),
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected audio span drift rejection")
	}
}

func TestRejectsOutputPathEscape(t *testing.T) {
	dir := setupSources(t)
	err := Repack(Options{
		Inputs: []string{sourcePath(dir, "en"), sourcePath(dir, "fr")},
		Output: "../outside.mcpack",
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected escaped output path rejection")
	}
	outside := filepath.Join(filepath.Dir(dir), "outside.mcpack")
	if _, statErr := os.Stat(outside); !os.IsNotExist(statErr) {
		t.Fatal("escaped output file was created")
	}
}

func TestRejectsMissingInput(t *testing.T) {
	dir := setupSources(t)
	err := Repack(Options{
		Inputs: []string{filepath.Join(dir, "nope.mcpack")},
		Output: filepath.Join(dir, "merged.mcpack"),
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected missing input rejection")
	}
}

func TestRejectsEmptyInputs(t *testing.T) {
	err := Repack(Options{
		Inputs: nil,
		Output: "merged.mcpack",
		Base:   t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected empty-input rejection")
	}
}

func TestRoundTripByteStable(t *testing.T) {
	dir := setupSources(t)
	out := filepath.Join(dir, "merged.mcpack")
	if err := Repack(Options{
		Inputs: []string{sourcePath(dir, "en"), sourcePath(dir, "fr")},
		Output: out, Base: dir,
	}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	boxes, err := ReadPack(out)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncodePack(boxes)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("merged pack is not byte-stable across round trips")
	}
}

func TestIdempotentRecommit(t *testing.T) {
	dir := setupSources(t)
	out := filepath.Join(dir, "merged.mcpack")
	opts := Options{
		Inputs: []string{sourcePath(dir, "en"), sourcePath(dir, "fr")},
		Output: out, Base: dir,
	}
	if err := Repack(opts); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(out)
	if err := Repack(opts); err != nil {
		t.Fatalf("second commit: %v", err)
	}
	second, _ := os.ReadFile(out)
	if string(first) != string(second) {
		t.Fatal("re-committing identical inputs changed output bytes")
	}
}
