package mixloud

import (
	"errors"
	"math"
)

// Audio is a channel-major, normalized floating-point PCM signal.
type Audio struct {
	SampleRate int
	Channels   [][]float64
}

func NewAudio(sampleRate, channels, frames int) (*Audio, error) {
	if sampleRate <= 0 {
		return nil, errors.New("sample rate must be positive")
	}
	if channels <= 0 {
		return nil, errors.New("channel count must be positive")
	}
	if frames < 0 {
		return nil, errors.New("frame count must be non-negative")
	}
	out := &Audio{SampleRate: sampleRate, Channels: make([][]float64, channels)}
	for ch := range out.Channels {
		out.Channels[ch] = make([]float64, frames)
	}
	return out, nil
}

func (a *Audio) Frames() int {
	if a == nil || len(a.Channels) == 0 {
		return 0
	}
	return len(a.Channels[0])
}

func (a *Audio) NumChannels() int {
	if a == nil {
		return 0
	}
	return len(a.Channels)
}

func (a *Audio) Duration() float64 {
	if a == nil || a.SampleRate <= 0 {
		return 0
	}
	return float64(a.Frames()) / float64(a.SampleRate)
}

func (a *Audio) Clone() *Audio {
	if a == nil {
		return nil
	}
	out := &Audio{SampleRate: a.SampleRate, Channels: make([][]float64, len(a.Channels))}
	for ch := range a.Channels {
		out.Channels[ch] = append([]float64(nil), a.Channels[ch]...)
	}
	return out
}

// Mono returns the equal-weight downmix used for mono-compatibility checks.
func (a *Audio) Mono() []float64 {
	n := a.Frames()
	if n == 0 || len(a.Channels) == 0 {
		return nil
	}
	out := make([]float64, n)
	scale := 1 / float64(len(a.Channels))
	for _, ch := range a.Channels {
		for i := 0; i < n; i++ {
			out[i] += ch[i] * scale
		}
	}
	return out
}

func db2gain(db float64) float64 {
	return math.Pow(10, db/20)
}

func gain2db(gain float64) float64 {
	if gain <= 0 || math.IsNaN(gain) {
		return math.Inf(-1)
	}
	return 20 * math.Log10(gain)
}

func safeDB(x float64) float64 {
	if x <= 1e-12 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(x)
}

func finiteOrNegInf(v float64) float64 {
	if math.IsNaN(v) {
		return math.Inf(-1)
	}
	return v
}
