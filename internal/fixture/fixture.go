package fixture

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"

	"surrounddiag/internal/wav"
)

type Problem struct {
	Code     string   `json:"code"`
	Channels []string `json:"channels"`
	Start    float64  `json:"start_seconds"`
	End      float64  `json:"end_seconds"`
	Detail   string   `json:"detail"`
}

type Truth struct {
	SampleRate int       `json:"sample_rate"`
	Duration   float64   `json:"duration_seconds"`
	Source     string    `json:"source"`
	Export     string    `json:"export"`
	Problems   []Problem `json:"problems"`
	Controls   []string  `json:"controls"`
}

func Generate(dir string) (*Truth, error) {
	const (
		rate     = 48000
		channels = 6
		duration = 8.0
	)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	frames := int(rate * duration)
	source := make([]float64, frames*channels)
	rng := rand.New(rand.NewSource(20261125))
	dialogue := make([]float64, frames)
	for frame := 0; frame < frames; frame++ {
		t := float64(frame) / rate
		envelope := speechEnvelope(t)
		if envelope > 0 {
			var value float64
			for harmonic := 1; harmonic <= 7; harmonic++ {
				frequency := 125 * float64(harmonic)
				weight := 1 / float64(harmonic)
				if harmonic == 3 || harmonic == 4 {
					weight *= 1.8
				}
				value += weight * math.Sin(2*math.Pi*frequency*t)
			}
			dialogue[frame] = 0.055 * envelope * value
		}
		low := 0.28 * lowEnvelope(t) * math.Sin(2*math.Pi*43*t)
		leftWide := 0.12 * rng.NormFloat64()
		rightWide := 0.12 * rng.NormFloat64()
		bl := 0.10 * rng.NormFloat64() * effectEnvelope(t, 2.0, 4.0, 6.0, 7.2)
		br := 0.10 * rng.NormFloat64() * effectEnvelope(t, 2.5, 4.5, 6.2, 7.5)
		set(source, frame, channels, 0, leftWide+dialogue[frame]*0.20)
		set(source, frame, channels, 1, rightWide+dialogue[frame]*0.20)
		set(source, frame, channels, 2, dialogue[frame])
		set(source, frame, channels, 3, low)
		set(source, frame, channels, 4, bl)
		set(source, frame, channels, 5, br)
	}
	exported := make([]float64, len(source))
	for frame := 0; frame < frames; frame++ {
		t := float64(frame) / rate
		flFrame := frame - 3
		if flFrame < 0 {
			flFrame = 0
		}
		set(exported, frame, channels, 0, get(source, flFrame, channels, 0))
		set(exported, frame, channels, 1, get(source, frame, channels, 1))
		center := get(source, frame, channels, 2)
		if t >= 2.0 && t < 5.0 {
			center = -center
		}
		set(exported, frame, channels, 2, center)
		lfe := get(source, frame, channels, 3)
		if t >= 6.0 {
			lfe *= 0.05
		}
		set(exported, frame, channels, 3, lfe)
		leftSurround := get(source, frame, channels, 4)
		rightSurround := get(source, frame, channels, 5)
		if t >= 5.0 {
			leftSurround *= 0.07
			rightSurround *= 0.07
		}
		set(exported, frame, channels, 4, rightSurround)
		set(exported, frame, channels, 5, leftSurround)
	}
	sourcePath := filepath.Join(dir, "master-5.1.wav")
	exportPath := filepath.Join(dir, "export-5.1.wav")
	if err := wav.WritePCM24(sourcePath, rate, channels, source); err != nil {
		return nil, err
	}
	if err := wav.WritePCM24(exportPath, rate, channels, exported); err != nil {
		return nil, err
	}
	truth := &Truth{
		SampleRate: rate, Duration: duration, Source: sourcePath, Export: exportPath,
		Problems: []Problem{
			{Code: "polarity_inversion", Channels: []string{"FC"}, Start: 2, End: 5, Detail: "FC 样点极性翻转"},
			{Code: "channel_swap", Channels: []string{"BL", "BR"}, Start: 0, End: 8, Detail: "BL/BR 内容互换"},
			{Code: "surround_attenuation", Channels: []string{"BL", "BR"}, Start: 5, End: 8, Detail: "环绕衰减约 23 dB"},
			{Code: "lfe_low_frequency_loss", Channels: []string{"LFE"}, Start: 6, End: 8, Detail: "LFE 低频衰减约 26 dB"},
			{Code: "sample_delay", Channels: []string{"FL"}, Start: 0, End: 8, Detail: "FL 相对母版延迟 3 samples"},
		},
		Controls: []string{
			"FL/FR 使用独立去相关宽声场，不应因低相关或负相关被误判为反相",
			"未故障时段的反相检测不得扩散到整段",
		},
	}
	data, err := json.MarshalIndent(truth, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "truth.json"), append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	return truth, nil
}

func set(data []float64, frame, channels, channel int, value float64) {
	data[frame*channels+channel] = value
}

func get(data []float64, frame, channels, channel int) float64 {
	return data[frame*channels+channel]
}

func speechEnvelope(t float64) float64 {
	for _, phrase := range [][2]float64{{0.25, 2.75}, {3.15, 5.65}, {6.10, 7.75}} {
		if t >= phrase[0] && t < phrase[1] {
			edge := math.Min(t-phrase[0], phrase[1]-t)
			return math.Min(1, edge/0.04)
		}
	}
	return 0
}

func lowEnvelope(t float64) float64 {
	if t < 0.2 || t > 7.9 {
		return 0
	}
	return 0.65 + 0.35*math.Sin(2*math.Pi*0.5*t)*math.Sin(2*math.Pi*0.5*t)
}

func effectEnvelope(t float64, ranges ...float64) float64 {
	for i := 0; i+1 < len(ranges); i += 2 {
		if t >= ranges[i] && t < ranges[i+1] {
			edge := math.Min(t-ranges[i], ranges[i+1]-t)
			return math.Min(1, edge/0.08)
		}
	}
	return 0.03
}

func VerifyTruth(truth *Truth) error {
	if truth.SampleRate <= 0 || truth.Duration <= 0 {
		return fmt.Errorf("invalid truth metadata")
	}
	return nil
}
