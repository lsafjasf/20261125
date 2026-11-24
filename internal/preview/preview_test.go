package preview

import (
	"math"
	"path/filepath"
	"testing"

	"surrounddiag/internal/wav"
)

func TestExportIsMonoAndSampleExact(t *testing.T) {
	dir := t.TempDir()
	const channels = 6
	frames := 4800
	data := make([]float64, frames*channels)
	for frame := 0; frame < frames; frame++ {
		for channel := 0; channel < channels; channel++ {
			data[frame*channels+channel] = float64(channel+1) / 20 * math.Sin(float64(frame)*0.02)
		}
	}
	source := filepath.Join(dir, "source.wav")
	if err := wav.WritePCM24(source, 48000, channels, data); err != nil {
		t.Fatal(err)
	}
	file, err := wav.Read(source)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Export(file, filepath.Join(dir, "previews"), 0.01, 0.09)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Items) != channels {
		t.Fatalf("got %d previews", len(manifest.Items))
	}
	for index, item := range manifest.Items {
		preview, err := wav.Read(item.File)
		if err != nil {
			t.Fatal(err)
		}
		if preview.Channels != 1 || preview.Frames != 3840 {
			t.Fatalf("preview %d format: channels=%d frames=%d", index, preview.Channels, preview.Frames)
		}
		expected := data[480*channels+index]
		if math.Abs(preview.Data[0]-expected) > 1.0/8388607 {
			t.Fatalf("preview %d first sample %.9f want %.9f", index, preview.Data[0], expected)
		}
	}
}
