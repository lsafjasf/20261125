package cbarcheck

import (
	"fmt"
	"math"
)

type Pattern string

const (
	PatternEBU75    Pattern = "ebu75"
	PatternSMPTE100 Pattern = "smpte100"
)

var barNames = []string{
	"white", "yellow", "cyan", "green", "magenta", "red", "blue", "black",
}

var barRGB = []RGB{
	{R: 1, G: 1, B: 1},
	{R: 1, G: 1, B: 0},
	{R: 0, G: 1, B: 1},
	{R: 0, G: 1, B: 0},
	{R: 1, G: 0, B: 1},
	{R: 1, G: 0, B: 0},
	{R: 0, G: 0, B: 1},
	{R: 0, G: 0, B: 0},
}

// BarsLayout builds a conventional vertical colour-bar field over a
// grayscale ramp. The upper two thirds contain eight bars; the lower third
// contains eleven neutral steps from 0% to 100%.
func BarsLayout(pattern Pattern, width, height int) (Layout, error) {
	if width < 8 || height < 3 {
		return Layout{}, fmt.Errorf("layout must be at least 8x3, got %dx%d", width, height)
	}
	level := 0
	switch pattern {
	case PatternEBU75:
		level = 75
	case PatternSMPTE100:
		level = 100
	default:
		return Layout{}, fmt.Errorf("unsupported pattern %q", pattern)
	}

	splitY := height * 2 / 3
	layout := Layout{Name: string(pattern) + "-bars-gray11", Width: width, Height: height}
	for i, name := range barNames {
		x0 := i * width / len(barNames)
		x1 := (i + 1) * width / len(barNames)
		rgb := barRGB[i]
		rgb.R *= float64(level) / 100
		rgb.G *= float64(level) / 100
		rgb.B *= float64(level) / 100
		layout.Regions = append(layout.Regions, Region{
			Index: len(layout.Regions),
			Name:  fmt.Sprintf("%s%d", name, level),
			Kind:  RegionColorBar,
			X0:    x0, Y0: 0, X1: x1, Y1: splitY,
			RGB: rgb,
		})
	}
	const steps = 11
	for i := 0; i < steps; i++ {
		x0 := i * width / steps
		x1 := (i + 1) * width / steps
		v := float64(i) / (steps - 1)
		layout.Regions = append(layout.Regions, Region{
			Index: len(layout.Regions),
			Name:  fmt.Sprintf("gray%02d", i*10),
			Kind:  RegionGray,
			X0:    x0, Y0: splitY, X1: x1, Y1: height,
			RGB: RGB{R: v, G: v, B: v},
		})
	}
	if err := layout.Validate(); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

func (l Layout) Validate() error {
	if l.Width <= 0 || l.Height <= 0 {
		return fmt.Errorf("invalid layout size %dx%d", l.Width, l.Height)
	}
	if len(l.Regions) == 0 {
		return fmt.Errorf("layout has no regions")
	}
	seen := make([]int, l.Width*l.Height)
	for _, r := range l.Regions {
		if r.X0 < 0 || r.Y0 < 0 || r.X1 > l.Width || r.Y1 > l.Height ||
			r.X0 >= r.X1 || r.Y0 >= r.Y1 {
			return fmt.Errorf("region %q has invalid rectangle [%d,%d)-[%d,%d)",
				r.Name, r.X0, r.Y0, r.X1, r.Y1)
		}
		if err := validateRGB(r.RGB); err != nil {
			return fmt.Errorf("region %q: %w", r.Name, err)
		}
		for y := r.Y0; y < r.Y1; y++ {
			for x := r.X0; x < r.X1; x++ {
				i := y*l.Width + x
				if seen[i] != 0 {
					return fmt.Errorf("regions overlap at (%d,%d)", x, y)
				}
				seen[i] = 1
			}
		}
	}
	for y := 0; y < l.Height; y++ {
		for x := 0; x < l.Width; x++ {
			if seen[y*l.Width+x] == 0 {
				return fmt.Errorf("layout leaves pixel (%d,%d) uncovered", x, y)
			}
		}
	}
	return nil
}

func validateRGB(rgb RGB) error {
	for _, v := range []float64{rgb.R, rgb.G, rgb.B} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return fmt.Errorf("RGB values must be finite and in 0..1")
		}
	}
	return nil
}

// GenerateFrame renders every pixel of layout under cfg. The result is a
// useful reference frame as well as a deterministic fixture generator.
func GenerateFrame(layout Layout, cfg Config) (*Frame, error) {
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	n := layout.Width * layout.Height
	f := &Frame{
		Width: layout.Width, Height: layout.Height, Depth: cfg.Depth,
		Y: make([]uint16, n), Cb: make([]uint16, n), Cr: make([]uint16, n),
	}
	for _, r := range layout.Regions {
		s, err := EncodeRGB(r.RGB, cfg)
		if err != nil {
			return nil, err
		}
		for y := r.Y0; y < r.Y1; y++ {
			for x := r.X0; x < r.X1; x++ {
				i := y*layout.Width + x
				f.Y[i] = uint16(s.Y)
				f.Cb[i] = uint16(s.Cb)
				f.Cr[i] = uint16(s.Cr)
			}
		}
	}
	return f, nil
}

func regionIndex(layout Layout) ([]int, error) {
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	idx := make([]int, layout.Width*layout.Height)
	for i, r := range layout.Regions {
		for y := r.Y0; y < r.Y1; y++ {
			for x := r.X0; x < r.X1; x++ {
				idx[y*layout.Width+x] = i
			}
		}
	}
	return idx, nil
}
