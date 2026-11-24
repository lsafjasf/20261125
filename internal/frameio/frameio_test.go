package frameio

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDrawGrayPreservesFullRange(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 1))
	src.Set(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	src.Set(1, 0, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	src.Set(2, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	gray := image.NewGray(image.Rect(0, 0, 3, 1))
	drawGray(gray, src)
	if gray.Pix[0] != 255 {
		t.Fatalf("white luma = %d, want 255", gray.Pix[0])
	}
	if gray.Pix[1] != 0 {
		t.Fatalf("black luma = %d, want 0", gray.Pix[1])
	}
	if gray.Pix[2] < 74 || gray.Pix[2] > 78 {
		t.Fatalf("red luma = %d, want about 76", gray.Pix[2])
	}
}

func TestManifestMustBeExplicitWhenRequestedMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadPlan(dir, "missing.json", 30, 1)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v, want explicit missing manifest error", err)
	}
}

func TestManifestRejectsUnsafeAndInvalidEntries(t *testing.T) {
	tests := []struct {
		name    string
		frames  []ManifestFrame
		wantErr string
	}{
		{
			name: "absolute path",
			frames: []ManifestFrame{{
				ID: "a", File: "/tmp/a.png", PTSUS: 0,
			}},
			wantErr: "must be relative",
		},
		{
			name: "parent escape",
			frames: []ManifestFrame{{
				ID: "a", File: "../a.png", PTSUS: 0,
			}},
			wantErr: "must be relative",
		},
		{
			name: "duplicate id",
			frames: []ManifestFrame{
				{ID: "a", File: "a.png", PTSUS: 0},
				{ID: "a", File: "b.png", PTSUS: 1},
			},
			wantErr: "duplicate frame id",
		},
		{
			name: "nonmonotonic pts",
			frames: []ManifestFrame{
				{ID: "a", File: "a.png", PTSUS: 2},
				{ID: "b", File: "b.png", PTSUS: 1},
			},
			wantErr: "earlier than previous",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestManifest(t, dir, test.frames)
			_, err := LoadPlan(dir, "", 30, 1)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestDirectoryFallbackTimestampsAreDeterministic(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.png", "a.png"} {
		img := image.NewGray(image.Rect(0, 0, 2, 2))
		file, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, img); err != nil {
			file.Close()
			t.Fatal(err)
		}
		file.Close()
	}
	plan, err := LoadPlan(dir, "", 25, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Frames) != 2 {
		t.Fatalf("frame count = %d", len(plan.Frames))
	}
	if plan.Frames[0].File != "a.png" || plan.Frames[1].File != "b.png" {
		t.Fatalf("sorted files = %q, %q", plan.Frames[0].File, plan.Frames[1].File)
	}
	if plan.Frames[1].PTSUS != 40_000 {
		t.Fatalf("second pts = %d, want 40000", plan.Frames[1].PTSUS)
	}
	if plan.TimestampSource != "directory_order_fps" {
		t.Fatalf("timestamp source = %q", plan.TimestampSource)
	}
}

func TestManifestCanDeriveTimestampsFromFPS(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{
  "schema": "tear-manifest/v1",
  "fps_num": 25,
  "fps_den": 1,
  "frames": [
    {"id": "a", "file": "a.png"},
    {"id": "b", "file": "b.png"}
  ]
}`)
	if err := os.WriteFile(filepath.Join(dir, "frames.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := LoadPlan(dir, "", 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Frames[1].PTSUS != 40_000 || plan.Frames[1].DurationUS != 40_000 {
		t.Fatalf("derived frame = %+v", plan.Frames[1])
	}
}

func TestManifestAcceptsMillisecondTimestamps(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{
  "schema": "tear-manifest/v1",
  "frames": [
    {"id": "a", "file": "a.png", "pts_ms": 0, "duration_ms": 40},
    {"id": "b", "file": "b.png", "pts_ms": 40, "duration_ms": 40}
  ]
}`)
	if err := os.WriteFile(filepath.Join(dir, "frames.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := LoadPlan(dir, "", 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Frames[1].PTSUS != 40_000 || plan.Frames[1].DurationUS != 40_000 {
		t.Fatalf("millisecond frame = %+v", plan.Frames[1])
	}
}

func writeTestManifest(t *testing.T, dir string, frames []ManifestFrame) {
	t.Helper()
	raw, err := json.Marshal(Manifest{Schema: ManifestSchema, Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "frames.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
