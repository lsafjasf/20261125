package sdrquant

import (
	"fmt"
	"math"
)

// SyntheticConfig describes a dark, smooth test gradient. Step is an optional
// real discontinuity in the source, useful for proving that classification
// does not mistake source edges for quantization contours.
type SyntheticConfig struct {
	Width   int
	Height  int
	Min     float64
	Max     float64
	Texture float64
	Step    float64
	StepAt  float64
}

// GenerateDarkGradient creates a high-bit-depth grayscale ramp with an
// optional genuine source step.
func GenerateDarkGradient(cfg SyntheticConfig) (*FloatImage, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("invalid synthetic size %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Max <= cfg.Min {
		return nil, fmt.Errorf("synthetic maximum must exceed minimum")
	}
	if cfg.StepAt <= 0 || cfg.StepAt >= 1 {
		cfg.StepAt = 0.75
	}
	im, _ := NewFloatImage(cfg.Width, cfg.Height, 1)
	for y := 0; y < cfg.Height; y++ {
		v := float64(y) / math.Max(1, float64(cfg.Height-1))
		for x := 0; x < cfg.Width; x++ {
			u := float64(x) / math.Max(1, float64(cfg.Width-1))
			value := cfg.Min + (cfg.Max-cfg.Min)*math.Pow(u, 1.15)
			value += cfg.Texture * 0.5 * math.Sin(2*math.Pi*v)
			if cfg.Step != 0 && u >= cfg.StepAt {
				value += cfg.Step
			}
			im.Set(x, y, 0, float32(value))
		}
	}
	return im, nil
}
