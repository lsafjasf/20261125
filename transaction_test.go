package repack

import (
	"os"
	"path/filepath"
	"testing"
)

func mutatePack(t *testing.T, src, dst string, mutate func([]Box) []Box) {
	t.Helper()
	boxes, err := ReadPack(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	raw, err := EncodePack(mutate(boxes))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func hashFile(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func TestInputsRemainUnchangedAfterSuccessfulCommit(t *testing.T) {
	dir := setupSources(t)
	inputPaths := []string{
		sourcePath(dir, "en"),
		sourcePath(dir, "fr"),
		sourcePath(dir, "de"),
	}
	before := map[string]int64{}
	for _, p := range inputPaths {
		before[p] = hashFile(t, p)
	}
	out := filepath.Join(dir, "merged.mcpack")
	if err := Repack(Options{
		Inputs: inputPaths, Output: out, Base: dir,
	}); err != nil {
		t.Fatalf("Repack: %v", err)
	}
	for _, p := range inputPaths {
		if hashFile(t, p) != before[p] {
			t.Errorf("input %s changed during repack", p)
		}
	}
}

func TestReadOnlyInputsStillProcessed(t *testing.T) {
	dir := setupSources(t)
	ro := filepath.Join(dir, "readonly")
	if err := os.Mkdir(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	var roInputs []string
	for _, lang := range []string{"en", "fr"} {
		dst := filepath.Join(ro, "pack-"+lang+".mcpack")
		mutatePack(t, sourcePath(dir, lang), dst, func(b []Box) []Box { return b })
		if err := os.Chmod(dst, 0o444); err != nil {
			t.Fatal(err)
		}
		roInputs = append(roInputs, dst)
	}
	out := filepath.Join(dir, "merged.mcpack")
	if err := Repack(Options{
		Inputs: roInputs, Output: out, Base: dir,
	}); err != nil {
		t.Fatalf("Repack on read-only inputs: %v", err)
	}
	if _, err := ReadPack(out); err != nil {
		t.Fatalf("output unreadable: %v", err)
	}
}

func TestRollbackWhenNoPriorOutput(t *testing.T) {
	dir := setupSources(t)
	out := filepath.Join(dir, "merged.mcpack")
	for _, stage := range []FailStage{FailAfterWrite, FailBeforeRename, FailVerifyOutput, FailAfterRename} {
		err := Repack(Options{
			Inputs: []string{sourcePath(dir, "en"), sourcePath(dir, "fr")},
			Output: out,
			Base:   dir,
			FailAt: stage,
		})
		if err == nil {
			t.Fatalf("stage %s: expected injected error", stage)
		}
		if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
			t.Fatalf("stage %s: output left behind after rollback", stage)
		}
		if leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*")); len(leftovers) != 0 {
			t.Fatalf("stage %s: temp files left: %v", stage, leftovers)
		}
		if leftovers, _ := filepath.Glob(filepath.Join(dir, "*.backup-*")); len(leftovers) != 0 {
			t.Fatalf("stage %s: backup files left: %v", stage, leftovers)
		}
	}
}

func TestRollbackRestoresPriorOutput(t *testing.T) {
	dir := setupSources(t)
	out := filepath.Join(dir, "merged.mcpack")
	// Seed an existing, valid output.
	if err := Repack(Options{
		Inputs: []string{sourcePath(dir, "en"), sourcePath(dir, "fr"), sourcePath(dir, "de")},
		Output: out, Base: dir,
	}); err != nil {
		t.Fatalf("seed Repack: %v", err)
	}
	original, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []FailStage{FailAfterWrite, FailBeforeRename, FailVerifyOutput, FailAfterRename} {
		err := Repack(Options{
			Inputs: []string{sourcePath(dir, "en"), sourcePath(dir, "fr")},
			Output: out,
			Base:   dir,
			FailAt: stage,
		})
		if err == nil {
			t.Fatalf("stage %s: expected injected error", stage)
		}
		got, readErr := os.ReadFile(out)
		if readErr != nil {
			t.Fatalf("stage %s: prior output missing: %v", stage, readErr)
		}
		if string(got) != string(original) {
			t.Fatalf("stage %s: prior output not restored byte-exact", stage)
		}
	}
}

func TestCorruptTempDetectedAndRolledBack(t *testing.T) {
	dir := setupSources(t)
	out := filepath.Join(dir, "merged.mcpack")
	err := Repack(Options{
		Inputs:      []string{sourcePath(dir, "en"), sourcePath(dir, "fr")},
		Output:      out,
		Base:        dir,
		CorruptTemp: true,
	})
	if err == nil {
		t.Fatal("expected corruption detection")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("corrupt output was installed despite verification failure")
	}
}

// Regression: subtitles must be timed against the pack's own audio track.
// The old packager attached subtitle timing to another language's audio.
func TestRejectsSubtitleTimedAgainstForeignAudio(t *testing.T) {
	dir := setupSources(t)
	bad := filepath.Join(dir, "bad-es.mcpack")
	mutatePack(t, sourcePath(dir, "en"), bad, func(boxes []Box) []Box {
		for i := range boxes {
			if boxes[i].Type == TypeAudio {
				boxes[i].Language = "es"
			}
			if boxes[i].Type == TypeSubtitle {
				boxes[i].Language = "es"
				// Shift every cue by half a second so it no longer aligns
				// with any es audio sample.
				boxes[i].StartUs += 500_000
				boxes[i].EndUs += 500_000
			}
		}
		return boxes
	})
	err := Repack(Options{
		Inputs: []string{sourcePath(dir, "fr"), bad},
		Output: filepath.Join(dir, "merged.mcpack"),
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected misaligned subtitle rejection")
	}
}

func TestRejectsSubtitleLanguageMismatch(t *testing.T) {
	dir := setupSources(t)
	bad := filepath.Join(dir, "bad.mcpack")
	mutatePack(t, sourcePath(dir, "en"), bad, func(boxes []Box) []Box {
		for i := range boxes {
			if boxes[i].Type == TypeSubtitle {
				boxes[i].Language = "fr"
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
		t.Fatal("expected subtitle/audio language mismatch rejection")
	}
}

// Regression: switching languages must not restart the picture. All sources
// must carry identical video frames at identical timestamps.
func TestRejectsVideoRestartAcrossLanguages(t *testing.T) {
	dir := setupSources(t)
	bad := filepath.Join(dir, "bad-fr.mcpack")
	mutatePack(t, sourcePath(dir, "fr"), bad, func(boxes []Box) []Box {
		for i := range boxes {
			if boxes[i].Type == TypeVideo {
				boxes[i].StartUs += 250_000
				boxes[i].EndUs += 250_000
			}
		}
		return boxes
	})
	err := Repack(Options{
		Inputs: []string{sourcePath(dir, "en"), bad},
		Output: filepath.Join(dir, "merged.mcpack"),
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected video timing mismatch rejection")
	}
}

func TestRejectsVideoContentMismatch(t *testing.T) {
	dir := setupSources(t)
	bad := filepath.Join(dir, "bad-de.mcpack")
	mutatePack(t, sourcePath(dir, "de"), bad, func(boxes []Box) []Box {
		for i := range boxes {
			if boxes[i].Type == TypeVideo {
				boxes[i].Payload = []byte("different-frame")
			}
		}
		return boxes
	})
	err := Repack(Options{
		Inputs: []string{sourcePath(dir, "en"), bad},
		Output: filepath.Join(dir, "merged.mcpack"),
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected video content mismatch rejection")
	}
}

func TestRejectsChapterMismatch(t *testing.T) {
	dir := setupSources(t)
	bad := filepath.Join(dir, "bad-fr.mcpack")
	mutatePack(t, sourcePath(dir, "fr"), bad, func(boxes []Box) []Box {
		for i := range boxes {
			if boxes[i].Type == TypeChapter {
				boxes[i].StartUs += 1000
			}
		}
		return boxes
	})
	err := Repack(Options{
		Inputs: []string{sourcePath(dir, "en"), bad},
		Output: filepath.Join(dir, "merged.mcpack"),
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected chapter mismatch rejection")
	}
}

func TestRejectsCorruptInput(t *testing.T) {
	dir := setupSources(t)
	bad := filepath.Join(dir, "corrupt.mcpack")
	raw, err := os.ReadFile(sourcePath(dir, "en"))
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xFF
	if err := os.WriteFile(bad, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err = Repack(Options{
		Inputs: []string{bad},
		Output: filepath.Join(dir, "merged.mcpack"),
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected checksum failure on corrupt input")
	}
}
