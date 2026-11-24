package mixloud

import (
	"bytes"
	"fmt"
	"math"
	"testing"
)

func TestWAVRoundTripPCM16(t *testing.T) {
	a, err := NewAudio(48000, 2, 4800)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a.Channels[0] {
		x := 0.5 * math.Sin(2*math.Pi*1000*float64(i)/48000)
		a.Channels[0][i] = x
		a.Channels[1][i] = -x
	}
	var buf bytes.Buffer
	if err := WriteWAV(&buf, a, false); err != nil {
		t.Fatal(err)
	}
	got, err := ReadWAV(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.SampleRate != 48000 || got.NumChannels() != 2 || got.Frames() != a.Frames() {
		t.Fatalf("bad WAV shape: sr=%d ch=%d frames=%d", got.SampleRate, got.NumChannels(), got.Frames())
	}
	for i := 100; i < a.Frames(); i += 997 {
		if math.Abs(got.Channels[0][i]-a.Channels[0][i]) > 2e-4 {
			t.Fatalf("sample %d mismatch: got %f want %f", i, got.Channels[0][i], a.Channels[0][i])
		}
	}
}

func TestIntegratedLoudnessFullScaleStereoTone(t *testing.T) {
	for _, rate := range []int{48000, 44100} {
		t.Run(fmt.Sprintf("%d", rate), func(t *testing.T) {
			a, _ := NewAudio(rate, 2, rate)
			for i := 0; i < a.Frames(); i++ {
				x := math.Sin(2 * math.Pi * 1000 * float64(i) / float64(rate))
				a.Channels[0][i] = x
				a.Channels[1][i] = x
			}
			sr, err := AnalyzeScheme(a, "test", "test", DefaultOptions(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if sr.Metrics.IntegratedLUFS == nil {
				t.Fatal("integrated loudness is nil")
			}
			if *sr.Metrics.IntegratedLUFS < -2 || *sr.Metrics.IntegratedLUFS > 1 {
				t.Fatalf("rate %d: I=%v LUFS", rate, *sr.Metrics.IntegratedLUFS)
			}
		})
	}
}

func TestPeakLimitCeiling(t *testing.T) {
	a, _ := NewAudio(48000, 2, 4800)
	for i := range a.Channels[0] {
		x := 1.8 * math.Sin(2*math.Pi*1000*float64(i)/48000)
		a.Channels[0][i] = x
		a.Channels[1][i] = -0.9 * x
	}
	limited := PeakLimit(a, -1, 5)
	ceiling := db2gain(-1) + 1e-9
	var peak float64
	for ch := range limited.Channels {
		for _, x := range limited.Channels[ch] {
			if math.Abs(x) > peak {
				peak = math.Abs(x)
			}
		}
	}
	if peak > ceiling {
		t.Fatalf("limited peak %f exceeds ceiling %f", peak, ceiling)
	}
}

func TestExactPhaseInversion(t *testing.T) {
	a, _ := NewAudio(48000, 2, 24000)
	for i := range a.Channels[0] {
		x := 0.4 * math.Sin(2*math.Pi*440*float64(i)/48000)
		a.Channels[0][i] = x
		a.Channels[1][i] = -x
	}
	sr, err := AnalyzeScheme(a, "phase", "phase", DefaultOptions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sr.PhaseInversions) == 0 {
		t.Fatal("expected phase inversion event")
	}
	if sr.PhaseInversions[0].Correlation == nil || *sr.PhaseInversions[0].Correlation > -0.99 {
		t.Fatalf("correlation not strongly negative: %+v", sr.PhaseInversions[0])
	}
}

func TestSlidingFutureMin(t *testing.T) {
	got := slidingFutureMin([]float64{3, 1, 4, 1, 5, 9, 2, 6}, 1)
	want := []float64{1, 1, 1, 1, 5, 2, 2, 6}
	if len(got) != len(want) {
		t.Fatalf("len=%d", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("at %d got %v want %v (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestDemoComparisonReportsAllSpecialCases(t *testing.T) {
	mix, stems := DemoAudio()
	cmp, err := Compare(mix, stems, "demo", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]SchemeReport{}
	for _, sr := range cmp.Report.Schemes {
		byID[sr.Metrics.Scheme] = sr
	}
	bypass := byID["bypass"]
	if len(bypass.SilenceSegments) != 1 {
		t.Fatalf("silence events=%d, want 1", len(bypass.SilenceSegments))
	}
	if d := bypass.SilenceSegments[0].DurationSec; d < 0.9 || d > 1.1 {
		t.Fatalf("silence duration %.3f", d)
	}
	if len(bypass.AdIntrusions) == 0 {
		t.Fatal("expected ad intrusion")
	}
	if len(bypass.PhaseInversions) == 0 {
		t.Fatal("expected phase inversion")
	}
	if bypass.Metrics.IntegratedLUFS == nil || bypass.Metrics.MaxWindowLUFS == nil {
		t.Fatal("missing loudness metrics")
	}
	if *bypass.Metrics.MaxWindowLUFS-*bypass.Metrics.IntegratedLUFS < 5 {
		t.Fatalf("local max %.2f not sufficiently above integrated %.2f", *bypass.Metrics.MaxWindowLUFS, *bypass.Metrics.IntegratedLUFS)
	}
	if bypass.Metrics.DialogueRatioTotal < 0.65 || bypass.Metrics.DialogueRatioTotal > 0.82 {
		t.Fatalf("dialogue ratio %.3f outside expected demo range", bypass.Metrics.DialogueRatioTotal)
	}

	limited := byID["peak_limit"]
	if limited.Metrics.MaxPeakDBFS == nil || *limited.Metrics.MaxPeakDBFS > -0.98 {
		t.Fatalf("limiter max peak %v not at/below -1 dBFS", limited.Metrics.MaxPeakDBFS)
	}
	if limited.Metrics.ClippedSamples != 0 {
		t.Fatalf("limited signal has %d full-scale samples", limited.Metrics.ClippedSamples)
	}
	if len(cmp.Report.Stems) != 3 {
		t.Fatalf("stem reports=%d, want 3", len(cmp.Report.Stems))
	}
}
