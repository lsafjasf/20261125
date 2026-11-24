package wav

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestPCM24RoundTripAndLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "six.wav")
	frames, channels := 100, 6
	data := make([]float64, frames*channels)
	for frame := 0; frame < frames; frame++ {
		for channel := 0; channel < channels; channel++ {
			data[frame*channels+channel] = float64(channel+1) / 16
		}
	}
	if err := WritePCM24(path, 48000, channels, data); err != nil {
		t.Fatal(err)
	}
	file, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Channels != channels || file.Frames != frames || file.SampleRate != 48000 {
		t.Fatalf("unexpected format: %+v", file)
	}
	want := []string{"FL", "FR", "FC", "LFE", "BL", "BR"}
	for i := range want {
		if file.Layout[i] != want[i] {
			t.Fatalf("layout[%d]=%q want %q", i, file.Layout[i], want[i])
		}
	}
	for frame := 0; frame < frames; frame++ {
		for channel := 0; channel < channels; channel++ {
			got := file.Data[frame*channels+channel]
			expected := data[frame*channels+channel]
			if math.Abs(got-expected) > 1.0/8388607 {
				t.Fatalf("sample %d/%d got %.9f want %.9f", frame, channel, got, expected)
			}
		}
	}
}

func TestParseLayoutAliases(t *testing.T) {
	layout, err := ParseLayout("L,R,C,LFE,Ls,Rs", 6)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"FL", "FR", "FC", "LFE", "BL", "BR"}
	for i := range want {
		if layout[i] != want[i] {
			t.Fatalf("layout[%d]=%q want %q", i, layout[i], want[i])
		}
	}
}

func TestExtensibleChannelMask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extensible.wav")
	const (
		channels = 6
		frames   = 8
	)
	pcm := make([]byte, frames*channels*3)
	fmtChunk := make([]byte, 40)
	binary.LittleEndian.PutUint16(fmtChunk[0:2], 0xfffe)
	binary.LittleEndian.PutUint16(fmtChunk[2:4], channels)
	binary.LittleEndian.PutUint32(fmtChunk[4:8], 48000)
	binary.LittleEndian.PutUint32(fmtChunk[8:12], 48000*channels*3)
	binary.LittleEndian.PutUint16(fmtChunk[12:14], channels*3)
	binary.LittleEndian.PutUint16(fmtChunk[14:16], 24)
	binary.LittleEndian.PutUint16(fmtChunk[16:18], 22)
	binary.LittleEndian.PutUint16(fmtChunk[18:20], 24)
	binary.LittleEndian.PutUint32(fmtChunk[20:24], 0x3f)
	binary.LittleEndian.PutUint16(fmtChunk[24:26], 1)
	copy(fmtChunk[26:40], guidSuffix)
	raw := make([]byte, 12)
	copy(raw[0:4], "RIFF")
	copy(raw[8:12], "WAVE")
	raw = appendChunk(raw, "fmt ", fmtChunk)
	raw = appendChunk(raw, "data", pcm)
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)-8))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"FL", "FR", "FC", "LFE", "BL", "BR"}
	for i := range want {
		if file.Layout[i] != want[i] {
			t.Fatalf("layout[%d]=%q want %q", i, file.Layout[i], want[i])
		}
	}
}

func appendChunk(dst []byte, id string, payload []byte) []byte {
	header := make([]byte, 8)
	copy(header[0:4], id)
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(payload)))
	dst = append(dst, header...)
	dst = append(dst, payload...)
	if len(payload)%2 != 0 {
		dst = append(dst, 0)
	}
	return dst
}
