package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"math/rand"
	"testing"

	"forensicaudio/internal/audio"
	"forensicaudio/internal/demo"
)

func analyzeDemo(t *testing.T, sensitivity float64) (*audio.WAV, *Analysis) {
	t.Helper()
	raw, err := demo.Generate()
	if err != nil {
		t.Fatal(err)
	}
	wav, err := audio.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Analyze(wav, "demo.wav", sensitivity)
	if err != nil {
		t.Fatal(err)
	}
	return wav, result
}

func TestNoiseFloorDriftAloneNeverFlags(t *testing.T) {
	const (
		sampleRate = 48000
		seconds    = 9
		channels   = 2
	)
	rng := rand.New(rand.NewSource(42))
	n := sampleRate * seconds
	samples := make([]float64, n*channels)
	for i := 0; i < n; i++ {
		t := float64(i) / sampleRate
		amp := 0.008 + 0.004*math.Sin(2*math.Pi*t/3.0)
		if t >= 3.0 && t < 5.2 {
			amp = 0.035
		}
		if t >= 6.8 {
			amp = 0.012 + 0.006*(t-6.8)
		}
		samples[i*channels] = amp * rng.NormFloat64()
		samples[i*channels+1] = amp * 0.8 * rng.NormFloat64()
	}
	raw, err := audio.EncodePCM16(channels, sampleRate, samples)
	if err != nil {
		t.Fatal(err)
	}
	wav, err := audio.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Analyze(wav, "drift.wav", 1.0)
	if err != nil {
		t.Fatal(err)
	}
	contextCount := 0
	for _, interval := range result.Intervals {
		if interval.Context.Score >= 0.35 {
			contextCount++
		}
	}
	if contextCount == 0 {
		t.Fatal("test signal did not exercise context evidence")
	}
	if result.Stats.Flagged != 0 {
		t.Fatalf("noise-floor drift alone flagged %d intervals", result.Stats.Flagged)
	}
	if result.Stats.Evidence["repeat"] != 0 {
		t.Fatalf("ambient noise produced repeat evidence: %d", result.Stats.Evidence["repeat"])
	}
}

func TestDemoFindsSpliceAndRepeatedSamples(t *testing.T) {
	_, result := analyzeDemo(t, 0.55)
	if len(result.Repeats) == 0 {
		t.Fatal("expected at least one repeated region")
	}
	foundCopy := false
	for _, region := range result.Repeats {
		if math.Abs(region.Start-2.0) < 0.1 && math.Abs(region.MatchStart-15.0) < 0.1 {
			foundCopy = true
		}
	}
	if !foundCopy {
		t.Fatalf("copied region not found: %+v", result.Repeats)
	}
	phaseHit := false
	contextOnly := false
	for _, interval := range result.Intervals {
		if interval.Phase.Score >= 0.8 && interval.Flagged && math.Abs(interval.Start-8.55) < 0.08 {
			phaseHit = true
		}
		if interval.Context.Score >= 0.5 && !interval.Flagged {
			contextOnly = true
		}
	}
	if !phaseHit {
		t.Fatal("phase splice was not flagged")
	}
	if !contextOnly {
		t.Fatal("expected context anomalies that remain below the red threshold")
	}
	if result.Stats.Evidence["repeat"] < 2 || result.Stats.Evidence["phase"] < 1 {
		t.Fatalf("missing evidence counts: %+v", result.Stats.Evidence)
	}
}

func TestSensitivityOnlyRecomputesCrossingIntervals(t *testing.T) {
	_, result := analyzeDemo(t, 0.55)
	beforeFlags := make([]bool, len(result.Intervals))
	for i, interval := range result.Intervals {
		beforeFlags[i] = interval.Flagged
	}
	beforeStats := cloneStats(result.Stats)
	oldCut := result.cutoff

	revision := result.SetSensitivity(0.10)
	newCut := result.cutoff
	low := math.Min(oldCut, newCut)
	high := math.Max(oldCut, newCut)
	expected := 0
	expectedIdx := map[int]bool{}
	for i, interval := range result.Intervals {
		if interval.Confidence >= low && interval.Confidence < high {
			expected++
			expectedIdx[i] = true
		}
	}
	if revision.RecomputedIntervals != expected {
		t.Fatalf("recomputed %d intervals, expected %d", revision.RecomputedIntervals, expected)
	}
	if expected == 0 || expected >= len(result.Intervals) {
		t.Fatalf("test did not produce a partial recomputation: %d/%d", expected, len(result.Intervals))
	}
	if revision.Before.Intervals != beforeStats.Intervals ||
		revision.After.Intervals != beforeStats.Intervals ||
		revision.Before.GridStart != revision.After.GridStart ||
		revision.Before.GridEnd != revision.After.GridEnd {
		t.Fatal("before/after statistics are not on the same grid")
	}
	if revision.Before.Flagged != beforeStats.Flagged {
		t.Fatalf("before snapshot changed: %d != %d", revision.Before.Flagged, beforeStats.Flagged)
	}
	changed := map[int]bool{}
	for _, change := range revision.Changes {
		changed[change.Index] = true
		if change.Score < low || change.Score >= high {
			t.Fatalf("change outside threshold band: %+v", change)
		}
		if result.Intervals[change.Index].Flagged == beforeFlags[change.Index] {
			t.Fatalf("interval %d was recorded as changed without toggling", change.Index)
		}
	}
	for i, interval := range result.Intervals {
		if !changed[i] && interval.Flagged != beforeFlags[i] {
			t.Fatalf("unaffected interval %d changed", i)
		}
		want := interval.Confidence >= newCut
		if interval.Flagged != want {
			t.Fatalf("interval %d classification is stale", i)
		}
	}
	if revision.After.Flagged != result.Stats.Flagged {
		t.Fatal("after statistics do not reflect the updated intervals")
	}
	if revision.Changed != len(revision.Changes) {
		t.Fatal("changed count mismatch")
	}
	covered := 0
	for i, interval := range result.Intervals {
		if !expectedIdx[i] {
			continue
		}
		for _, rng := range revision.RecomputedRanges {
			if interval.Start >= rng.Start-1e-9 && interval.End <= rng.End+1e-9 {
				covered++
				break
			}
		}
	}
	if covered != expected {
		t.Fatalf("recomputed ranges cover %d intervals, expected %d", covered, expected)
	}

	restore := result.SetSensitivity(0.55)
	for i, interval := range result.Intervals {
		if interval.Flagged != beforeFlags[i] {
			t.Fatalf("interval %d was not restored at the original sensitivity", i)
		}
	}
	if restore.After.Flagged != beforeStats.Flagged {
		t.Fatalf("restored count %d, want %d", restore.After.Flagged, beforeStats.Flagged)
	}
}

func TestStatisticsUseSameIntervalGrid(t *testing.T) {
	_, result := analyzeDemo(t, 0.55)
	if result.Stats.Intervals != result.Legacy.Intervals {
		t.Fatalf("interval count differs: %d vs %d", result.Stats.Intervals, result.Legacy.Intervals)
	}
	if result.Stats.GridStart != result.Legacy.GridStart ||
		result.Stats.GridEnd != result.Legacy.GridEnd ||
		result.Stats.IntervalSeconds != result.Legacy.IntervalSeconds {
		t.Fatal("legacy and current grids differ")
	}
	if !result.Alignment.Aligned || result.Alignment.IntervalCount != len(result.Intervals) {
		t.Fatalf("bad alignment: %+v", result.Alignment)
	}
	if len(result.Legacy.Evidence) != 1 || result.Legacy.Evidence["noiseFloor"] != result.Legacy.Flagged {
		t.Fatalf("legacy evidence should only describe its noise-floor rule: %+v", result.Legacy.Evidence)
	}
	frameSeconds := float64(result.Config.FrameSize) / float64(result.Metadata.SampleRate)
	if result.Stats.FlaggedSeconds >= float64(result.Stats.Flagged)*frameSeconds-1e-6 {
		t.Fatalf("overlapping red intervals were double-counted: %.3fs for %d frames",
			result.Stats.FlaggedSeconds, result.Stats.Flagged)
	}
	if result.Stats.FlaggedSeconds < 2.7 {
		t.Fatalf("red duration lost the repeated segments: %.3fs", result.Stats.FlaggedSeconds)
	}
}

func TestAnalysisDoesNotModifyOriginalOrSamples(t *testing.T) {
	raw, err := demo.Generate()
	if err != nil {
		t.Fatal(err)
	}
	wav, err := audio.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := sha256.Sum256(wav.Original)
	beforeSamples := append([]float64(nil), wav.Interleaved...)
	if _, err := Analyze(wav, "demo.wav", 0.55); err != nil {
		t.Fatal(err)
	}
	afterHash := sha256.Sum256(wav.Original)
	if beforeHash != afterHash {
		t.Fatal("original bytes changed during analysis")
	}
	if wav.SHA256 != hex.EncodeToString(afterHash[:]) {
		t.Fatal("original SHA-256 no longer matches")
	}
	if len(beforeSamples) != len(wav.Interleaved) {
		t.Fatal("analysis sample count changed")
	}
	for i := range beforeSamples {
		if beforeSamples[i] != wav.Interleaved[i] {
			t.Fatalf("analysis sample %d was modified", i)
		}
	}
}
