// Package pipeline applies and audits the Log-to-display path.
package pipeline

import (
	"fmt"
	"math"

	"logshoulder/internal/colorlog"
)

// Role describes how a patch is used in the comparison.
type Role string

const (
	Shadow    Role = "shadow"
	Midtone   Role = "midtone"
	Highlight Role = "highlight"
)

// Patch is a known scene-linear value in an exposure sample.
type Patch struct {
	Label string
	Role  Role
	Scene float64
}

// Case is one exposure/curve combination. Scene values are linear radiance
// relative to the camera's 18% grey anchor.
type Case struct {
	Name       string
	Curve      colorlog.Curve
	EV         float64
	Shadows    []float64
	Midtone    float64
	Highlights []float64
}

// Cases returns three deliberately different exposures on three known curves.
// The values stay below each curve's encoder ceiling; any saturation therefore
// belongs to the conversion path, not to the source encoding.
func Cases() []Case {
	return []Case{
		{
			Name:       "low-key / S-Log3 / EV -1",
			Curve:      colorlog.Curve{Kind: colorlog.SLog3},
			EV:         -1,
			Shadows:    []float64{0.0025, 0.005, 0.01},
			Midtone:    0.09,
			Highlights: []float64{0.5, 1, 2, 4, 8, 16, 24, 32},
		},
		{
			Name:       "base / LogC3 / EV 0",
			Curve:      colorlog.Curve{Kind: colorlog.LogC3},
			EV:         0,
			Shadows:    []float64{0.005, 0.01, 0.02},
			Midtone:    0.18,
			Highlights: []float64{0.625, 1.25, 2.5, 5, 10, 20, 30, 40},
		},
		{
			Name:       "high-key / ACEScct / EV +1",
			Curve:      colorlog.Curve{Kind: colorlog.ACEScct},
			EV:         1,
			Shadows:    []float64{0.01, 0.02, 0.04},
			Midtone:    0.36,
			Highlights: []float64{2, 4, 8, 16, 32, 64, 96, 128},
		},
	}
}

// Patches flattens a case into labelled scene values.
func (c Case) Patches() []Patch {
	out := make([]Patch, 0, len(c.Shadows)+1+len(c.Highlights))
	for i, scene := range c.Shadows {
		out = append(out, Patch{
			Label: fmt.Sprintf("shadow-%d", i+1),
			Role:  Shadow,
			Scene: scene,
		})
	}
	out = append(out, Patch{Label: "18%-anchor", Role: Midtone, Scene: c.Midtone})
	for i, scene := range c.Highlights {
		out = append(out, Patch{
			Label: fmt.Sprintf("window-%d", i+1),
			Role:  Highlight,
			Scene: scene,
		})
	}
	return out
}

// ExposureScale returns the linear multiplier represented by EV.
func (c Case) ExposureScale() float64 {
	return math.Pow(2, c.EV)
}
