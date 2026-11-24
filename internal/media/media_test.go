package media_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"cutmux/internal/fixture"
	"cutmux/internal/media"
)

func TestMP4EditListAndRationalFrameRate(t *testing.T) {
	samples := make([]fixture.SampleSpec, 10)
	for i := range samples {
		samples[i] = fixture.SampleSpec{Duration: 1001, Key: true, Data: []byte{byte(i), 1, 2, 3}}
	}
	data := fixture.BuildMP4(fixture.MP4Spec{
		MovieTimescale: 1000, DurationMS: 400, Width: 320, Height: 180,
		VideoTimescale: 30000, Video: samples,
		VideoEdits: []fixture.EditSpec{
			{SegmentDuration: 100, MediaTime: -1},
			{SegmentDuration: 300, MediaTime: 0},
		},
	})
	path := writeTemp(t, "edit.mp4", data)
	f, err := media.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v := f.Primary(media.KindVideo)
	if v == nil {
		t.Fatal("missing video")
	}
	if got := v.FrameRate(); got.Num != 30000 || got.Den != 1001 {
		t.Fatalf("fps = %s", got)
	}
	span := v.PresentedSpan()
	if math.Abs(float64(span.Start)/1e6-100) > 0.01 {
		t.Fatalf("presented start = %.3f ms", float64(span.Start)/1e6)
	}
	if math.Abs(float64(span.End)/1e6-400) > 0.01 {
		t.Fatalf("presented end = %.3f ms", float64(span.End)/1e6)
	}
}

func TestMatroskaCodecDelayAndDiscardPadding(t *testing.T) {
	samples := make([]fixture.MKVSampleSpec, 6)
	for i := range samples {
		samples[i] = fixture.MKVSampleSpec{
			TimeMS: int16(10 + i*25), Key: true, Data: []byte{byte(i), 9, 8, 7},
		}
	}
	samples[len(samples)-1].DiscardPaddingNS = 5_000_000
	data := fixture.BuildMKV(fixture.MKVSpec{
		TimestampScale: 1_000_000, DurationMS: 150,
		Tracks: []fixture.MKVTrackSpec{{
			Number: 1, Type: 2, CodecID: "A_OPUS",
			DefaultDurationNS: 25_000_000, CodecDelayNS: 10_000_000,
			SampleRate: 48000, Channels: 2, Samples: samples,
		}},
	})
	path := writeTemp(t, "delay.mkv", data)
	f, err := media.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a := f.Primary(media.KindAudio)
	if a == nil {
		t.Fatal("missing audio")
	}
	if a.CodecDelay != 10_000_000 {
		t.Fatalf("codec delay = %d", a.CodecDelay)
	}
	span := a.PresentedSpan()
	if math.Abs(float64(span.Start)/1e6) > 0.01 {
		t.Fatalf("presented start = %.3f ms", float64(span.Start)/1e6)
	}
	// All block timestamps include 10 ms codec delay; the last block also discards 5 ms.
	if math.Abs(float64(span.End)/1e6-145) > 0.01 {
		t.Fatalf("presented end = %.3f ms", float64(span.End)/1e6)
	}
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
