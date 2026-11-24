package cbarcheck

import (
	"fmt"
	"math"
)

// LegalBounds gives the code-domain limits for one target configuration.
// Chroma bounds include the standard limited-range footroom/headroom.
type LegalBounds struct {
	YMin    float64 `json:"yMin"`
	YMax    float64 `json:"yMax"`
	CbMin   float64 `json:"cbMin"`
	CbMax   float64 `json:"cbMax"`
	CrMin   float64 `json:"crMin"`
	CrMax   float64 `json:"crMax"`
	CodeMax float64 `json:"codeMax"`
}

func Bounds(cfg Config) LegalBounds {
	if cfg.Range == RangeLimited {
		scale := 1 << (cfg.Depth - 8)
		return LegalBounds{
			YMin: float64(16 * scale), YMax: float64(235 * scale),
			CbMin: float64(16 * scale), CbMax: float64(240 * scale),
			CrMin: float64(16 * scale), CrMax: float64(240 * scale),
			CodeMax: float64((int(1) << cfg.Depth) - 1),
		}
	}
	max := float64((int(1) << cfg.Depth) - 1)
	return LegalBounds{
		YMin: 0, YMax: max,
		CbMin: 0, CbMax: max,
		CrMin: 0, CrMax: max,
		CodeMax: max,
	}
}

func coefficients(m Matrix) (kr, kg, kb float64, err error) {
	switch m {
	case MatrixBT601:
		return 0.299, 0.587, 0.114, nil
	case MatrixBT709:
		return 0.2126, 0.7152, 0.0722, nil
	case MatrixBT2020:
		return 0.2627, 0.6780, 0.0593, nil
	default:
		return 0, 0, 0, fmt.Errorf("unsupported matrix %q", m)
	}
}

// Normalized is the matrix-domain Y'PbPr triplet. Pb and Pr are bipolar,
// normally in the inclusive -0.5..0.5 range for legal RGB input.
type Normalized struct {
	Y  float64
	Pb float64
	Pr float64
}

func FromRGB(rgb RGB, m Matrix) (Normalized, error) {
	kr, kg, kb, err := coefficients(m)
	if err != nil {
		return Normalized{}, err
	}
	y := kr*rgb.R + kg*rgb.G + kb*rgb.B
	pb := (rgb.B - y) / (2 * (1 - kb))
	pr := (rgb.R - y) / (2 * (1 - kr))
	return Normalized{Y: y, Pb: pb, Pr: pr}, nil
}

// DecodeNormalized interprets raw codes under cfg without clipping. No
// clipping is important: out-of-range codes must remain visible to the audit.
func DecodeNormalized(s Sample, cfg Config) (Normalized, error) {
	if err := cfg.Validate(); err != nil {
		return Normalized{}, err
	}
	if cfg.Range == RangeLimited {
		scale := float64(int(1) << (cfg.Depth - 8))
		return Normalized{
			Y:  (s.Y - 16*scale) / (219 * scale),
			Pb: (s.Cb - 128*scale) / (224 * scale),
			Pr: (s.Cr - 128*scale) / (224 * scale),
		}, nil
	}
	max := float64((int(1) << cfg.Depth) - 1)
	return Normalized{
		Y:  s.Y / max,
		Pb: (s.Cb - max/2) / max,
		Pr: (s.Cr - max/2) / max,
	}, nil
}

// Encode converts normalized matrix components to floating-point codes.
func Encode(n Normalized, cfg Config) (Sample, error) {
	if err := cfg.Validate(); err != nil {
		return Sample{}, err
	}
	if cfg.Range == RangeLimited {
		scale := float64(int(1) << (cfg.Depth - 8))
		return Sample{
			Y:  16*scale + n.Y*219*scale,
			Cb: 128*scale + n.Pb*224*scale,
			Cr: 128*scale + n.Pr*224*scale,
		}, nil
	}
	max := float64((int(1) << cfg.Depth) - 1)
	return Sample{
		Y:  n.Y * max,
		Cb: (n.Pb + 0.5) * max,
		Cr: (n.Pr + 0.5) * max,
	}, nil
}

// EncodeRGB is the common path used for expected samples and synthetic
// frames. The returned sample has integer-valued codes.
func EncodeRGB(rgb RGB, cfg Config) (Sample, error) {
	n, err := FromRGB(rgb, cfg.Matrix)
	if err != nil {
		return Sample{}, err
	}
	s, err := Encode(n, cfg)
	if err != nil {
		return Sample{}, err
	}
	max := float64((int(1) << cfg.Depth) - 1)
	s.Y = math.Round(clamp(s.Y, 0, max))
	s.Cb = math.Round(clamp(s.Cb, 0, max))
	s.Cr = math.Round(clamp(s.Cr, 0, max))
	return s, nil
}

// CodeError8 converts a normalized component error into 8-bit
// code-equivalent units under cfg.
func CodeError8(delta float64, cfg Config, luma bool) float64 {
	if cfg.Range == RangeFull {
		return math.Abs(delta) * 255
	}
	if luma {
		return math.Abs(delta) * 219
	}
	return math.Abs(delta) * 224
}

func codeErrorAtDepth(delta float64, cfg Config, luma bool) float64 {
	return CodeError8(delta, cfg, luma) * float64(int(1)<<(cfg.Depth-8))
}

func inRange(v, min, max float64) bool {
	return v >= min && v <= max
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
