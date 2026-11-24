package demo

import (
	"math"
	"math/rand"

	"forensicaudio/internal/audio"
)

const (
	SampleRate = 48000
	Channels   = 2
	Duration   = 22.0
)

// Generate builds a deterministic stereo PCM16 WAV. It intentionally contains
// benign noise-floor drift, a phase splice, and an exact copied segment.
func Generate() ([]byte, error) {
	n := int(SampleRate * Duration)
	samples := make([]float64, n*Channels)
	rng := rand.New(rand.NewSource(20261125))
	phaseDriftL, phaseDriftR := 0.0, 0.0
	const toneHz = 523.25
	for i := 0; i < n; i++ {
		t := float64(i) / SampleRate
		floor := 0.010 + 0.004*math.Sin(2*math.Pi*t/7.5)
		if t >= 6.2 && t < 10.4 {
			floor += 0.030 * smoothStep(t, 6.2, 6.22) * (1 - smoothStep(t, 10.38, 10.4))
		}
		if t >= 17.0 {
			floor += 0.012 * smoothStep(t, 17.0, 19.2)
		}
		left := floor * (0.65*rng.NormFloat64() + 0.20*math.Sin(phaseDriftL))
		right := floor * (0.65*rng.NormFloat64() - 0.20*math.Cos(phaseDriftR))
		phaseDriftL += 2 * math.Pi * 0.7 / SampleRate
		phaseDriftR += 2 * math.Pi * 0.9 / SampleRate
		tone := 0.075 * math.Sin(2*math.Pi*toneHz*t)
		if t >= 8.55 {
			tone = -tone
		}
		if t < 18.2 {
			left += tone
			right += 0.82 * tone
		}
		samples[i*Channels] = clampSample(left)
		samples[i*Channels+1] = clampSample(right)
	}
	copyExact(samples, 2.0, 3.4, 15.0)
	return audio.EncodePCM16(Channels, SampleRate, samples)
}

func smoothStep(v, lo, hi float64) float64 {
	if hi <= lo {
		if v >= hi {
			return 1
		}
		return 0
	}
	x := (v - lo) / (hi - lo)
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	return x * x * (3 - 2*x)
}

func copyExact(samples []float64, srcStart, srcEnd, dstStart float64) {
	src0 := int(srcStart * SampleRate)
	src1 := int(srcEnd * SampleRate)
	dst0 := int(dstStart * SampleRate)
	if src0 < 0 || src1 > len(samples)/Channels || src1 <= src0 {
		return
	}
	count := src1 - src0
	if (dst0+count)*Channels > len(samples) {
		return
	}
	copy(samples[dst0*Channels:(dst0+count)*Channels], samples[src0*Channels:(src0+count)*Channels])
}

func clampSample(v float64) float64 {
	return math.Max(-0.98, math.Min(0.98, v))
}
