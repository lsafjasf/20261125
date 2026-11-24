package wavio

import (
	"math"
	"path/filepath"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	n := 8000
	x := make([]float64, n)
	for i := range x {
		x[i] = 0.5 * math.Sin(2*math.Pi*440*float64(i)/16000)
	}
	path := filepath.Join(t.TempDir(), "rt.wav")
	if err := WriteMono16(path, x, 16000); err != nil {
		t.Fatal(err)
	}
	w, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if w.SampleRate != 16000 || w.Channels != 1 || w.BitsPerSample != 16 {
		t.Fatalf("header mismatch: %+v", w)
	}
	if len(w.Mono) != n {
		t.Fatalf("sample count %d want %d", len(w.Mono), n)
	}
	maxErr := 0.0
	for i := range x {
		if e := math.Abs(x[i] - w.Mono[i]); e > maxErr {
			maxErr = e
		}
	}
	if maxErr > 2.0/32768 {
		t.Fatalf("16-bit roundtrip error %.5f exceeds quantisation bound", maxErr)
	}
}

func TestRejectsNonWAV(t *testing.T) {
	if _, err := Parse([]byte("RIFFxxxxJUNK")); err == nil {
		t.Fatal("expected parse error for truncated/non-WAVE input")
	}
}
