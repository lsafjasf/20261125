package audio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"testing"
)

func TestParsePreservesOriginalBytes(t *testing.T) {
	samples := []float64{0, 0.25, -0.25, 0.75, -0.75, 1, -1, 0.1, -0.1}
	raw, err := EncodePCM16(1, 8000, samples)
	if err != nil {
		t.Fatal(err)
	}
	wav, err := Parse(append([]byte(nil), raw...))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wav.Original, raw) {
		t.Fatal("parser changed the original byte slice")
	}
	sum := sha256.Sum256(raw)
	if wav.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected SHA-256: %s", wav.SHA256)
	}
	if wav.Channels != 1 || wav.SampleRate != 8000 || wav.BitsPerSample != 16 {
		t.Fatalf("unexpected format: %+v", wav)
	}
	if wav.FrameCount != len(samples) {
		t.Fatalf("got %d frames, want %d", wav.FrameCount, len(samples))
	}
	for i, want := range samples {
		if math.Abs(wav.Interleaved[i]-want) > 1.0/32768 {
			t.Fatalf("sample %d: got %.8f want %.8f", i, wav.Interleaved[i], want)
		}
	}
}

func TestParseRejectsNonWAV(t *testing.T) {
	if _, err := Parse([]byte("not a wave file")); err == nil {
		t.Fatal("expected an error")
	}
}
