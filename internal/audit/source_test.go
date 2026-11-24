package audit

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gray.png")
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	values := []uint8{16, 32, 64, 128, 160, 192, 220, 235}
	for i, v := range values {
		img.SetNRGBA(i%4, i/4, color.NRGBA{R: v, G: v, B: v, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	info, acc, err := LoadClip(ClipSpec{Path: path}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 4 || info.Height != 2 || info.Frames != 1 {
		t.Fatalf("unexpected info: %+v", info)
	}
	stats := acc.Stats(info.Width, info.Height, info.Frames)
	if stats.Samples != 8 || stats.Min != 16 || stats.Max != 235 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestLoadRawYUV420(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frame.yuv")
	data := []byte{16, 32, 64, 128, 128, 128}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	spec := ClipSpec{Path: path, Width: 2, Height: 2, PixelFormat: "yuv420p"}
	info, acc, err := LoadClip(spec, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	stats := acc.Stats(info.Width, info.Height, info.Frames)
	if stats.Samples != 4 || stats.Min != 16 || stats.Max != 128 {
		t.Fatalf("unexpected raw stats: %+v", stats)
	}
	if info.Frames != 1 || info.Width != 2 {
		t.Fatalf("unexpected raw info: %+v", info)
	}
}

func TestLoadRawRequiresDimensions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frame.yuv")
	if err := os.WriteFile(path, make([]byte, 6), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadClip(ClipSpec{Path: path, PixelFormat: "yuv420p"}, "", 100)
	if err == nil {
		t.Fatal("expected an error without width and height")
	}
}

func TestLoadY4M(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frame.y4m")
	var buf bytes.Buffer
	buf.WriteString("YUV4MPEG2 W2 H2 F25:1 Ip C420jpeg\n")
	buf.WriteString("FRAME\n")
	buf.Write([]byte{16, 32, 64, 128, 128, 128})
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	info, acc, err := LoadClip(ClipSpec{Path: path}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	stats := acc.Stats(info.Width, info.Height, info.Frames)
	if stats.Samples != 4 || stats.Min != 16 || stats.Max != 128 {
		t.Fatalf("unexpected Y4M stats: %+v", stats)
	}
	if info.Format != "y4m/420jpeg" {
		t.Fatalf("format = %s", info.Format)
	}
}
