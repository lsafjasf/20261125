package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeTestFrame(t *testing.T, dir, name string) string {
	t.Helper()
	f := NewFrame(name, 4, 4, "sRGB")
	for i := range f.Pix {
		v := float64(i) / float64(len(f.Pix))
		f.Pix[i] = RGB{v, v * 0.8, 1 - v}
	}
	path := filepath.Join(dir, name)
	if err := WritePNG(path, f); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunDiamondRoundTripNearIdentity(t *testing.T) {
	dir := t.TempDir()
	framePath := writeTestFrame(t, dir, "f1.png")

	r := BuildReport(cfgFromJSON(t, diamondJSON))
	if !r.Valid {
		t.Fatalf("diamond invalid: %+v", r.Diagnostics)
	}
	outDir := filepath.Join(dir, "out")
	execReport, err := RunFrames(r, []string{framePath}, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(execReport.Frames[0].Outputs) != 1 {
		t.Fatalf("expected one target output: %+v", execReport.Frames[0].Outputs)
	}
	outPath := execReport.Frames[0].Outputs[0].Path
	got, err := ReadPNG(outPath, "sRGB")
	if err != nil {
		t.Fatal(err)
	}
	in, err := ReadPNG(framePath, "sRGB")
	if err != nil {
		t.Fatal(err)
	}
	// +10%/-10% branches averaged in linear light reconstruct the original,
	// within 8-bit quantization.
	for i := range in.Pix {
		for c := 0; c < 3; c++ {
			if d := abs(got.Pix[i][c] - in.Pix[i][c]); d > 0.03 {
				t.Fatalf("pixel %d channel %d drifted: in %.3f out %.3f", i, c, in.Pix[i][c], got.Pix[i][c])
			}
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestProvidedConfigAndFramesExecute(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("configs", "color_graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	r := BuildReport(cfg)
	if !r.Valid {
		t.Fatalf("shipped config must be valid: %+v", r.Diagnostics)
	}
	frames, err := ListFrames(filepath.Join("frames", "input"))
	if err != nil || len(frames) != 3 {
		t.Fatalf("expected 3 shipped frames, got %v err=%v", frames, err)
	}
	outDir := t.TempDir()
	execReport, err := RunFrames(r, frames[:1], outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(execReport.SkippedUnused) != 3 {
		t.Fatalf("expected 3 skipped unused nodes, got %v", execReport.SkippedUnused)
	}
	if len(execReport.Frames[0].Outputs) != 1 {
		t.Fatalf("expected deliver output")
	}
	if _, err := os.Stat(execReport.Frames[0].Outputs[0].Path); err != nil {
		t.Fatalf("output PNG missing: %v", err)
	}
}

func TestRunRefusesInvalidGraph(t *testing.T) {
	r := BuildReport(cfgFromJSON(t, `{"nodes":[{"id":"a","type":"gain","inputs":["a"]}]}`))
	if _, err := RunFrames(r, nil, t.TempDir()); err == nil {
		t.Fatal("cyclic graph execution must be refused")
	}
}

func TestGainOperator(t *testing.T) {
	f := NewFrame("x", 1, 1, "Linear sRGB")
	f.Set(0, 0, RGB{0.25, 0.5, 0.75})
	n := &Node{ID: "g", Type: "gain", Params: map[string]any{"gain": 2.0}}
	out, err := applyOp(n, []*Frame{f})
	if err != nil {
		t.Fatal(err)
	}
	px := out.At(0, 0)
	if px[0] != 0.5 || px[1] != 1.0 || px[2] != 1.5 {
		t.Fatalf("gain math wrong: %v", px)
	}
}

func TestNaturalSort(t *testing.T) {
	names := []string{"frame_10.png", "frame_2.png", "frame_1.png", "frame_20.png"}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if naturalLess(names[j], names[i]) {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	want := []string{"frame_1.png", "frame_2.png", "frame_10.png", "frame_20.png"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("natural sort got %v want %v", names, want)
		}
	}
}

func TestCLIAnalyzeCycleExit2(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runCLI([]string{"analyze", "-config", filepath.Join("configs", "cycle.json")}, &out, &errOut)
	if code != 2 {
		t.Fatalf("cycle exit code = %d, output=%s err=%s", code, out.String(), errOut.String())
	}
	if !bytes.Contains([]byte(out.String()), []byte("cycle")) {
		t.Fatalf("analyze output should mention cycle:\n%s", out.String())
	}
}

func TestCLIRunWritesReports(t *testing.T) {
	outDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{
		"run",
		"-config", filepath.Join("configs", "diamond.json"),
		"-frames", filepath.Join("frames", "input"),
		"-out", outDir,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run code=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	for _, name := range []string{"preflight.txt", "preflight.json", "run-report.json"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if !bytes.Contains(stdout.Bytes(), []byte("adopted")) {
		t.Fatalf("stdout must contain preflight color-space explanation")
	}
}
