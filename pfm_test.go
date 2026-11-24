package sdrquant

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestPFMRoundTripPreservesOrientationAndValues(t *testing.T) {
	src, _ := NewFloatImage(3, 2, 3)
	values := []float32{
		0.01, 0.2, 0.3,
		0.02, 0.4, 0.5,
		0.03, 0.6, 0.7,
		0.04, 0.8, 0.9,
		0.05, 0.10, 0.11,
		0.06, 0.12, 0.13,
	}
	copy(src.Pix, values)
	path := filepath.Join(t.TempDir(), "test.pfm")
	if err := SavePFM(path, src); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFloatImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != src.Width || got.Height != src.Height || got.Channels != 3 {
		t.Fatalf("bad dimensions: %dx%dx%d", got.Width, got.Height, got.Channels)
	}
	for i := range src.Pix {
		if math.Abs(float64(got.Pix[i]-src.Pix[i])) > 1e-6 {
			t.Fatalf("pixel %d = %g, want %g", i, got.Pix[i], src.Pix[i])
		}
	}
}

func TestLoad16BitPNGPreservesHighBitDepth(t *testing.T) {
	img := image.NewGray16(image.Rect(0, 0, 2, 1))
	img.SetGray16(0, 0, color.Gray16{Y: 257})
	img.SetGray16(1, 0, color.Gray16{Y: 32768})
	path := filepath.Join(t.TempDir(), "gray16.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFloatImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Channels != 1 {
		t.Fatalf("channels = %d, want 1", got.Channels)
	}
	if math.Abs(float64(got.Pix[0])-1.0/255) > 1e-6 {
		t.Fatalf("dark value = %g", got.Pix[0])
	}
	if math.Abs(float64(got.Pix[1])-float64(32768)/65535.0) > 1e-6 {
		t.Fatalf("mid value = %g", got.Pix[1])
	}
}

func TestReadPFMBigEndianAndCRLF(t *testing.T) {
	var data bytes.Buffer
	data.WriteString("Pf\r\n2 1\r\n1\r\n")
	for _, value := range []float32{0.25, 0.75} {
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], math.Float32bits(value))
		data.Write(buf[:])
	}
	path := filepath.Join(t.TempDir(), "be.pfm")
	if err := os.WriteFile(path, data.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFloatImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 2 || got.Height != 1 {
		t.Fatalf("bad dimensions: %dx%d", got.Width, got.Height)
	}
	for i, want := range []float32{0.25, 0.75} {
		if math.Abs(float64(got.Pix[i]-want)) > 1e-6 {
			t.Fatalf("pixel %d = %g, want %g", i, got.Pix[i], want)
		}
	}
}

func TestSavePNGAndLoadRoundTrip(t *testing.T) {
	src, _ := NewFloatImage(2, 1, 3)
	copy(src.Pix, []float32{0.1, 0.2, 0.3, 0.9, 0.8, 0.7})
	path := filepath.Join(t.TempDir(), "rgb.png")
	if err := SavePNG(path, src); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFloatImage(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []float32{26, 51, 77, 229, 204, 179} {
		if math.Abs(float64(got.Pix[i]*255)-float64(want)) > 1 {
			t.Fatalf("pixel %d = %g, want %g", i, got.Pix[i]*255, want)
		}
	}
}
