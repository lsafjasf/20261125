package analyze

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeTempPNG16(t *testing.T, img image.Image) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadPNG16NormalizesToSignalDepth(t *testing.T) {
	img := image.NewRGBA64(image.Rect(0, 0, 2, 1))
	// Left-justified 10-bit code 769 in a 16-bit container.
	v10 := uint16(769 << 6)
	img.SetRGBA64(0, 0, color.RGBA64{R: v10, G: v10, B: v10, A: 0xffff})
	// Left-justified 12-bit code 3079 in a 16-bit container.
	v12 := uint16(3079 << 4)
	img.SetRGBA64(1, 0, color.RGBA64{R: v12, G: v12, B: v12, A: 0xffff})
	p := writeTempPNG16(t, img)

	f10, err := LoadFrame(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	if f10.R[0] != 769 || f10.R[1] != 769 {
		t.Fatalf("10-bit normalization = %d, %d; want 769, 769", f10.R[0], f10.R[1])
	}

	f12, err := LoadFrame(p, 12)
	if err != nil {
		t.Fatal(err)
	}
	if f12.R[0] != 3076 || f12.R[1] != 3079 {
		t.Fatalf("12-bit normalization = %d, %d; want 3076, 3079", f12.R[0], f12.R[1])
	}

	f16, err := LoadFrame(p, 16)
	if err != nil {
		t.Fatal(err)
	}
	if f16.R[0] != uint32(v10) {
		t.Fatalf("16-bit passthrough = %d, want %d", f16.R[0], v10)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := LoadFrame(filepath.Join(t.TempDir(), "nope.png"), 10); err == nil {
		t.Fatal("expected error for missing file")
	}
}
