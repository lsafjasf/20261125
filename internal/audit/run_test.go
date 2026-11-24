package audit

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestRunAndReports(t *testing.T) {
	dir := t.TempDir()
	img := image.NewGray(image.Rect(0, 0, 4, 4))
	for i := 0; i < 8; i++ {
		img.SetGray(i%4, i/4, color.Gray{Y: 16})
		img.SetGray(i%4, 2+i/4, color.Gray{Y: 235})
	}
	pngPath := filepath.Join(dir, "rail.png")
	f, err := os.Create(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	manifest := `{
  "defaults": {"declared": "full"},
  "clips": [{"id": "rail", "path": "rail.png"}]
}`
	manifestPath := filepath.Join(dir, "batch.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	specs, baseDir, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	report := Run(specs, baseDir, 1_000_000)
	if report.Batch.Analyzed != 1 || report.Batch.LabelMismatches != 1 {
		t.Fatalf("unexpected batch summary: %+v", report.Batch)
	}
	clip := report.Clips[0]
	if clip.Assessment.Decision != "limited" || clip.Assessment.LabelStatus != "incorrect" {
		t.Fatalf("unexpected assessment: %+v", clip.Assessment)
	}

	var jsonBuf bytes.Buffer
	if err := WriteJSON(&jsonBuf, report); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(jsonBuf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Clips[0].Assessment.CorrectedLabel != "limited (16-235)" {
		t.Fatalf("JSON lost corrected label")
	}

	var md bytes.Buffer
	if err := WriteMarkdown(&md, report); err != nil {
		t.Fatal(err)
	}
	text := md.String()
	for _, want := range []string{"limited (16-235)", "incorrect", "候选解释", "保留解释", "硬轨道"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Markdown missing %q\n%s", want, text)
		}
	}
}

func TestManifestDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	manifest := `{"clips": [{"id": "a", "path": "a.png"}, {"id": "a", "path": "b.png"}]}`
	path := filepath.Join(dir, "batch.json")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	specs, _, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if specs[1].ID != "a-2" {
		t.Fatalf("duplicate id = %q", specs[1].ID)
	}
}
