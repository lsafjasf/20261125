package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"cutmux/internal/fixture"
)

func TestIdenticalPayloadsAcrossContainersIsLegal(t *testing.T) {
	video := makeVideoSamples(10)
	audio := makeAudioSamples(10)
	mp4Path, mkvPath := writePair(t, video, video, audio, audio)
	rep, err := Run(mp4Path, mkvPath, Options{LabelA: "mp4", LabelB: "mkv", Now: fixedNow()})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Video.Classification != "identical-payload-and-timeline" {
		t.Fatalf("video class = %s: %s", rep.Video.Classification, rep.Video.Explanation)
	}
	if rep.Audio.Classification != "identical-payload-and-timeline" {
		t.Fatalf("audio class = %s: %s", rep.Audio.Classification, rep.Audio.Explanation)
	}
	if !rep.Verdict.LegalMux || rep.Verdict.RealLoss {
		t.Fatalf("verdict = %+v", rep.Verdict)
	}
}

func TestInternalMissingVideoFrameIsRealLoss(t *testing.T) {
	videoA := makeVideoSamples(12)
	videoB := append(append([]fixture.SampleSpec{}, videoA[:5]...), videoA[6:]...)
	audio := makeAudioSamples(10)
	mp4Path, mkvPath := writePair(t, videoA, videoB, audio, audio)
	rep, err := Run(mp4Path, mkvPath, Options{LabelA: "long", LabelB: "short", Now: fixedNow()})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Video.Classification != "real-frame-loss-or-insert" {
		t.Fatalf("video class = %s: %s", rep.Video.Classification, rep.Video.Explanation)
	}
	if !rep.Verdict.RealLoss {
		t.Fatalf("expected real loss verdict, got %+v", rep.Verdict)
	}
	if len(rep.Video.OnlyA) != 1 || rep.Video.OnlyA[0].Frames != 1 {
		t.Fatalf("onlyA = %+v", rep.Video.OnlyA)
	}
}

func TestTwoTrailingBlackFramesAreHandleNotDroppedFrames(t *testing.T) {
	videoA := makeVideoSamples(12)
	videoB := makeVideoSamples(10)
	audio := makeAudioSamples(10)
	mp4Path, mkvPath := writePair(t, videoA, videoB, audio, audio)
	sidecar := filepath.Join(t.TempDir(), "frames-a.csv")
	var b strings.Builder
	b.WriteString("pts_seconds,black,luma\n")
	for i := 0; i < 12; i++ {
		black := 0
		if i >= 10 {
			black = 1
		}
		b.WriteString(fmt.Sprintf("%.3f", float64(i)*0.04))
		if _, err := b.WriteString("," + strconv.Itoa(black) + ",16\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(sidecar, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Run(mp4Path, mkvPath, Options{
		LabelA: "mp4", LabelB: "mkv", FramesA: sidecar, Now: fixedNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Video.Classification != "black-handle-only" {
		t.Fatalf("video class = %s: %s", rep.Video.Classification, rep.Video.Explanation)
	}
	if !rep.BlackFrames.A.Definitive || rep.BlackFrames.A.Matched != 2 {
		t.Fatalf("black evidence = %+v", rep.BlackFrames.A)
	}
	if rep.Verdict.RealLoss {
		t.Fatalf("black handle must not be reported as real loss: %+v", rep.Verdict)
	}
}

func TestExtraAudioPacketsArePaddingBoundary(t *testing.T) {
	video := makeVideoSamples(10)
	audioA := makeAudioSamples(10)
	audioB := makeAudioSamples(12)
	mp4Path, mkvPath := writePair(t, video, video, audioA, audioB)
	rep, err := Run(mp4Path, mkvPath, Options{LabelA: "mp4", LabelB: "mkv", Now: fixedNow()})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Audio.Classification != "padding-boundary" {
		t.Fatalf("audio class = %s: %s", rep.Audio.Classification, rep.Audio.Explanation)
	}
	if rep.Audio.DurationDeltaMs < 49.9 || rep.Audio.DurationDeltaMs > 50.1 {
		t.Fatalf("audio delta = %.3f", rep.Audio.DurationDeltaMs)
	}
	if len(rep.Audio.OnlyB) != 1 || rep.Audio.OnlyB[0].Frames != 2 {
		t.Fatalf("onlyB = %+v", rep.Audio.OnlyB)
	}
	if rep.Verdict.RealLoss {
		t.Fatalf("padding must not be reported as real loss: %+v", rep.Verdict)
	}
}

func TestTwoFrameAndFiftyMillisecondCase(t *testing.T) {
	videoA := makeVideoSamples(12)
	videoB := makeVideoSamples(10)
	audioA := makeAudioSamples(10)
	audioB := makeAudioSamples(12)
	mp4Path, mkvPath := writePair(t, videoA, videoB, audioA, audioB)
	sidecar := filepath.Join(t.TempDir(), "black.csv")
	content := "pts_seconds,black\n0.000,0\n0.040,0\n0.080,0\n0.120,0\n" +
		"0.160,0\n0.200,0\n0.240,0\n0.280,0\n0.320,0\n0.360,0\n0.400,1\n0.440,1\n"
	if err := os.WriteFile(sidecar, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Run(mp4Path, mkvPath, Options{
		LabelA: "mp4", LabelB: "mkv", FramesA: sidecar, Now: fixedNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Video.Classification != "black-handle-only" {
		t.Fatalf("video = %s: %s", rep.Video.Classification, rep.Video.Explanation)
	}
	if rep.Audio.Classification != "padding-boundary" {
		t.Fatalf("audio = %s: %s", rep.Audio.Classification, rep.Audio.Explanation)
	}
	if rep.Verdict.RealLoss {
		t.Fatalf("unexpected real loss: %+v", rep.Verdict)
	}
	if !rep.Verdict.NeedsReview {
		t.Fatalf("duration difference should remain visible for review: %+v", rep.Verdict)
	}
	if !strings.Contains(rep.NextCheck, "400.000") || !strings.Contains(rep.NextCheck, "音频") {
		t.Fatalf("next check is not actionable: %s", rep.NextCheck)
	}
}

func TestSubtitleInPointShift(t *testing.T) {
	video := makeVideoSamples(10)
	audio := makeAudioSamples(10)
	mp4 := fixture.BuildMP4(fixture.MP4Spec{
		MovieTimescale: 1000, DurationMS: 400, Width: 320, Height: 180,
		VideoTimescale: 1000, Video: video,
		AudioTimescale: 1000, AudioRate: 48000, AudioChannels: 2, Audio: audio,
		SubtitleTimescale: 1000,
		Subtitle:          []fixture.SampleSpec{{Data: fixture.TextSample(1), Duration: 1000, Key: true}},
		SubtitleEdits: []fixture.EditSpec{
			{SegmentDuration: 100, MediaTime: -1},
			{SegmentDuration: 300, MediaTime: 0},
		},
	})
	mkv := fixture.BuildMKV(fixture.MKVSpec{
		TimestampScale: 1_000_000, DurationMS: 400,
		Tracks: []fixture.MKVTrackSpec{
			mkvVideo(video), mkvAudio(audio),
			{Number: 3, Type: 17, CodecID: "S_TEXT/UTF8", Samples: []fixture.MKVSampleSpec{
				{TimeMS: 150, Key: true, Data: []byte("subtitle")},
			}},
		},
	})
	mp4Path, mkvPath := writeBytes(t, mp4, mkv)
	rep, err := Run(mp4Path, mkvPath, Options{LabelA: "mp4", LabelB: "mkv", Now: fixedNow()})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Subtitle.Classification != "in-point-shift" {
		t.Fatalf("subtitle class = %s: %s", rep.Subtitle.Classification, rep.Subtitle.Explanation)
	}
}

func TestReportIsDeterministicWithFixedNow(t *testing.T) {
	video := makeVideoSamples(10)
	audio := makeAudioSamples(10)
	mp4Path, mkvPath := writePair(t, video, video, audio, audio)
	opts := Options{LabelA: "mp4", LabelB: "mkv", Now: fixedNow()}
	first, err := Run(mp4Path, mkvPath, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Run(mp4Path, mkvPath, opts)
	if err != nil {
		t.Fatal(err)
	}
	a, err := json.MarshalIndent(first, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(second, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("report is not deterministic")
	}
}

func writePair(t *testing.T, videoA, videoB, audioA, audioB []fixture.SampleSpec) (string, string) {
	t.Helper()
	duration := uint32(len(videoA) * 40)
	mp4 := fixture.BuildMP4(fixture.MP4Spec{
		MovieTimescale: 1000, DurationMS: duration, Width: 320, Height: 180,
		VideoTimescale: 1000, Video: videoA,
		AudioTimescale: 1000, AudioRate: 48000, AudioChannels: 2, Audio: audioA,
	})
	mkv := fixture.BuildMKV(fixture.MKVSpec{
		TimestampScale: 1_000_000, DurationMS: float64(duration),
		Tracks: []fixture.MKVTrackSpec{mkvVideo(videoB), mkvAudio(audioB)},
	})
	return writeBytes(t, mp4, mkv)
}

func writeBytes(t *testing.T, mp4, mkv []byte) (string, string) {
	t.Helper()
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.mp4"), filepath.Join(dir, "b.mkv")
	if err := os.WriteFile(a, mp4, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, mkv, 0o644); err != nil {
		t.Fatal(err)
	}
	return a, b
}

func makeVideoSamples(n int) []fixture.SampleSpec {
	out := make([]fixture.SampleSpec, n)
	for i := range out {
		out[i] = fixture.SampleSpec{Data: []byte{0x56, 0x49, 0x44, byte(i), byte(i >> 8)}, Duration: 40, Key: true}
	}
	return out
}

func makeAudioSamples(n int) []fixture.SampleSpec {
	out := make([]fixture.SampleSpec, n)
	for i := range out {
		out[i] = fixture.SampleSpec{Data: []byte{0x41, 0x55, byte(i), byte(i >> 8)}, Duration: 25, Key: true}
	}
	return out
}

func mkvVideo(samples []fixture.SampleSpec) fixture.MKVTrackSpec {
	out := fixture.MKVTrackSpec{
		Number: 1, Type: 1, CodecID: "V_MPEG4/ISO/AVC",
		DefaultDurationNS: 40_000_000, Width: 320, Height: 180,
	}
	for i, s := range samples {
		out.Samples = append(out.Samples, fixture.MKVSampleSpec{
			TimeMS: int16(i * 40), Key: true, Data: s.Data,
		})
	}
	return out
}

func mkvAudio(samples []fixture.SampleSpec) fixture.MKVTrackSpec {
	out := fixture.MKVTrackSpec{
		Number: 2, Type: 2, CodecID: "A_AAC",
		DefaultDurationNS: 25_000_000, SampleRate: 48000, Channels: 2,
	}
	for i, s := range samples {
		out.Samples = append(out.Samples, fixture.MKVSampleSpec{
			TimeMS: int16(i * 25), Key: true, Data: s.Data,
		})
	}
	return out
}

func fixedNow() time.Time {
	return time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
}
