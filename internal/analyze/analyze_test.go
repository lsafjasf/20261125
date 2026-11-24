package analyze

import (
	"math/rand"
	"path/filepath"
	"testing"

	"surrounddiag/internal/fixture"
	"surrounddiag/internal/wav"
)

func TestWideFieldLowCorrelationIsNotInversion(t *testing.T) {
	path := writeStereo(t, func(rng *rand.Rand) (float64, float64) {
		return 0.15 * rng.NormFloat64(), 0.15 * rng.NormFloat64()
	})
	file := readWAV(t, path)
	result, err := Analyze(file, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if hasCode(result, "polarity_inversion_candidate") {
		t.Fatalf("wide decorrelated field was flagged as inversion: %+v", result.Findings)
	}
	if result.WideField.MirrorPairs != 0 {
		t.Fatalf("unexpected mirror pairs: %+v", result.WideField)
	}
}

func TestMirrorPolarityRequiresWaveformEvidence(t *testing.T) {
	path := writeStereo(t, func(rng *rand.Rand) (float64, float64) {
		value := 0.15 * rng.NormFloat64()
		return value, -value
	})
	file := readWAV(t, path)
	result, err := Analyze(file, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "polarity_inversion_candidate") {
		t.Fatalf("mirror waveform not detected: %+v", result.Findings)
	}
}

func TestSampleLevelDelay(t *testing.T) {
	const delay = 7
	path := writeStereoWithFrames(t, 2*48000, func(frame int, rng *rand.Rand) (float64, float64) {
		left := 0.15 * rng.NormFloat64()
		if frame < delay {
			return left, 0
		}
		return left, 0 // overwritten below by writeStereoWithFrames' history
	}, func(leftHistory []float64, frame int) float64 {
		if frame < delay {
			return 0
		}
		return leftHistory[frame-delay]
	})
	result, err := Analyze(readWAV(t, path), DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range result.Pairs {
		if pair.AChannel == "FL" && pair.BChannel == "FR" {
			if pair.MedianLag != delay {
				t.Fatalf("lag=%d want %d (pairs=%+v)", pair.MedianLag, delay, result.Pairs)
			}
			return
		}
	}
	t.Fatalf("FL/FR pair missing: %+v", result.Pairs)
}

func TestMatchPreservesOddAndNegativeLags(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	left := make([]float64, 12000)
	for i := range left {
		left[i] = rng.NormFloat64()
	}
	for _, want := range []int{-9, -1, 0, 1, 7, 31} {
		right := make([]float64, len(left))
		for i := range right {
			source := i - want
			if source >= 0 && source < len(left) {
				right[i] = left[source]
			}
		}
		match := Match(left, right, 0, len(left), 64, 8)
		if match.LagSamples != want {
			t.Fatalf("lag=%d want %d (r=%.4f)", match.LagSamples, want, match.Correlation)
		}
		if match.Correlation < 0.95 {
			t.Fatalf("lag %d confidence too low: %.4f", want, match.Correlation)
		}
	}
}

func TestReferenceFixtureFindsInjectedFaults(t *testing.T) {
	dir := t.TempDir()
	truth, err := fixture.Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	export := readWAV(t, truth.Export)
	reference := readWAV(t, truth.Source)
	result, err := AnalyzeReference(export, reference, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if result.WideField.MirrorPairs != 0 {
		t.Fatalf("wide field false positive: %+v", result.WideField)
	}
	requireFinding(t, result, "reference_polarity_inversion", []string{"FC"}, 2.0, 5.0)
	requireFinding(t, result, "reference_channel_swap_or_misroute", []string{"BL", "BR"}, 0, 8)
	requireFinding(t, result, "reference_surround_loss", []string{"BL"}, 5, 8)
	requireFinding(t, result, "reference_surround_loss", []string{"BR"}, 5, 8)
	requireFinding(t, result, "reference_low_frequency_loss", []string{"LFE"}, 6, 8)
	requireFinding(t, result, "reference_sample_delay", []string{"FL"}, 0, 8)
}

func writeStereo(t *testing.T, sample func(*rand.Rand) (float64, float64)) string {
	t.Helper()
	return writeStereoWithFrames(t, 2*48000, func(frame int, rng *rand.Rand) (float64, float64) {
		return sample(rng)
	}, nil)
}

func writeStereoWithFrames(t *testing.T, frames int, sample func(int, *rand.Rand) (float64, float64), right func([]float64, int) float64) string {
	t.Helper()
	rng := rand.New(rand.NewSource(99))
	left := make([]float64, frames)
	interleaved := make([]float64, frames*2)
	for frame := 0; frame < frames; frame++ {
		leftValue, rightValue := sample(frame, rng)
		left[frame] = leftValue
		if right != nil {
			rightValue = right(left, frame)
		}
		interleaved[frame*2] = leftValue
		interleaved[frame*2+1] = rightValue
	}
	path := filepath.Join(t.TempDir(), "stereo.wav")
	if err := wav.WritePCM24(path, 48000, 2, interleaved); err != nil {
		t.Fatal(err)
	}
	return path
}

func readWAV(t *testing.T, path string) *wav.File {
	t.Helper()
	file, err := wav.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func hasCode(result *Report, code string) bool {
	for _, finding := range result.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func requireFinding(t *testing.T, result *Report, code string, channels []string, start, end float64) {
	t.Helper()
	for _, finding := range result.Findings {
		if finding.Code != code || !containsAll(finding.Channels, channels) {
			continue
		}
		if finding.Start <= start+0.05 && finding.End >= end-0.05 {
			return
		}
	}
	t.Fatalf("missing finding %s %v [%.1f,%.1f]; got %+v", code, channels, start, end, result.Findings)
}

func containsAll(values, wanted []string) bool {
	for _, want := range wanted {
		found := false
		for _, value := range values {
			if value == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
