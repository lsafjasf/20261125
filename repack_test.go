package repack

import (
	"os"
	"path/filepath"
	"testing"
)

func setupSources(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := GenerateSamplePacks(dir); err != nil {
		t.Fatalf("GenerateSamplePacks: %v", err)
	}
	return dir
}

func sourcePath(dir, lang string) string {
	return filepath.Join(dir, "pack-"+lang+".mcpack")
}

func TestRepackHappyPath(t *testing.T) {
	dir := setupSources(t)
	out := filepath.Join(dir, "merged.mcpack")
	opts := Options{
		Inputs: []string{
			sourcePath(dir, "en"),
			sourcePath(dir, "fr"),
			sourcePath(dir, "de"),
		},
		Output: out,
		Base:   dir,
	}
	if err := Repack(opts); err != nil {
		t.Fatalf("Repack: %v", err)
	}
	boxes, err := ReadPack(out)
	if err != nil {
		t.Fatalf("ReadPack output: %v", err)
	}
	want := 4 /* video shared */ + 2 /* chapters shared */ + 4*3 /* audio */ + 4*3 /* subs */
	if len(boxes) != want {
		t.Fatalf("merged box count = %d, want %d", len(boxes), want)
	}
	counts := map[BoxType]int{}
	langs := map[string]bool{}
	for _, b := range boxes {
		counts[b.Type]++
		if b.Type == TypeAudio {
			langs[b.Language] = true
		}
	}
	if counts[TypeVideo] != 4 || counts[TypeChapter] != 2 {
		t.Fatalf("shared tracks wrong: %#v", counts)
	}
	if counts[TypeAudio] != 12 || counts[TypeSubtitle] != 12 {
		t.Fatalf("per-language tracks wrong: %#v", counts)
	}
	if len(langs) != 3 || !langs["en"] || !langs["fr"] || !langs["de"] {
		t.Fatalf("audio languages = %#v", langs)
	}
}

func TestRepackRejectsDuplicateLanguageCode(t *testing.T) {
	dir := setupSources(t)
	out := filepath.Join(dir, "merged.mcpack")
	err := Repack(Options{
		Inputs: []string{sourcePath(dir, "en"), sourcePath(dir, "en")},
		Output: out,
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected duplicate language rejection")
	}
	assertNoOutput(t, out)
}

func TestRepackRejectsDuplicateInputPathSpelling(t *testing.T) {
	dir := setupSources(t)
	out := filepath.Join(dir, "merged.mcpack")
	rel := func(p string) string { return p }
	err := Repack(Options{
		Inputs: []string{
			rel(filepath.Join("pack-en.mcpack")),
			rel(filepath.Join(".", "pack-en.mcpack")),
		},
		Output: out,
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected duplicate path rejection")
	}
	assertNoOutput(t, out)
}

func TestResolveRejectsPathEscape(t *testing.T) {
	base := filepath.Clean(t.TempDir())
	cases := []string{"../escape.mcpack", "a/../../escape.mcpack", "../"}
	for _, p := range cases {
		if _, err := Resolve(base, p); err == nil {
			t.Errorf("Resolve(%q) = nil error, want escape rejection", p)
		}
	}
}

func TestResolveNormalizesRelativeVariants(t *testing.T) {
	base := t.TempDir()
	want := filepath.Join(base, "a", "b.mcpack")
	variants := []string{
		filepath.Join("a", "b.mcpack"),
		filepath.Join("a", ".", "b.mcpack"),
		filepath.Join("a", "sub", "..", "b.mcpack"),
		filepath.Join(".", "a", "b.mcpack"),
	}
	for _, v := range variants {
		got, err := Resolve(base, v)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", v, err)
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestRepackRelativePathConfinedToBase(t *testing.T) {
	dir := setupSources(t)
	out := "merged.mcpack"
	err := Repack(Options{
		Inputs: []string{
			filepath.Join(".", "pack-en.mcpack"),
			filepath.Join("sub", "..", "pack-fr.mcpack"),
			filepath.Join("pack-de.mcpack"),
		},
		Output: out,
		Base:   dir,
	})
	if err != nil {
		t.Fatalf("Repack with relative variants: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, out)); err != nil {
		t.Fatalf("output missing at base: %v", err)
	}
}

func TestRepackRejectsOutputAliasingInput(t *testing.T) {
	dir := setupSources(t)
	err := Repack(Options{
		Inputs: []string{filepath.Join(".", "pack-en.mcpack")},
		Output: filepath.Join("pack-en.mcpack"),
		Base:   dir,
	})
	if err == nil {
		t.Fatal("expected same-file output rejection")
	}
	// Input must remain untouched and valid.
	if _, err := ReadPack(sourcePath(dir, "en")); err != nil {
		t.Fatalf("input altered: %v", err)
	}
}

func assertNoOutput(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("output %s exists after failed run: %v", path, err)
	}
}
