package sdrquant

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
)

// DitherMode controls the pre-quantization random signal.
type DitherMode string

const (
	DitherNone        DitherMode = "none"
	DitherRectangular DitherMode = "rectangular"
	DitherTPDF        DitherMode = "tpdf"
)

// ShapingMode controls post-quantization error feedback.
type ShapingMode string

const (
	ShapingNone              ShapingMode = "none"
	ShapingFloydSteinberg    ShapingMode = "floyd-steinberg"
	ShapingAtkinson          ShapingMode = "atkinson"
	ShapingSierraLite        ShapingMode = "sierra-lite"
	ShapingJarvisJudiceNinke ShapingMode = "jarvis-judice-ninke"
	ShapingStucki            ShapingMode = "stucki"
)

// TransferMode selects the encoding applied before dithering. SDR output is
// display-referred; use TransferSRGB when the float source is scene-linear.
type TransferMode string

const (
	TransferNone TransferMode = "none"
	TransferSRGB TransferMode = "srgb"
)

// Config controls the complete quantizer. Values are in output LSB units.
type Config struct {
	Bits           int
	Dither         DitherMode
	DitherStrength float64
	Shaping        ShapingMode
	Seed           int64
	Serpentine     bool
	Transfer       TransferMode
}

// DefaultConfig fixes dark-gradient banding while retaining a reproducible
// noise field. Use DitherNone plus ShapingNone to inspect the baseline bands.
func DefaultConfig() Config {
	return Config{
		Bits:           8,
		Dither:         DitherTPDF,
		DitherStrength: 1.0,
		Shaping:        ShapingFloydSteinberg,
		Seed:           1,
		Serpentine:     true,
		Transfer:       TransferNone,
	}
}

// Result contains both the SDR output and the quantities needed to audit it.
// Residual, Feedback, Dither, and ShapedError are row-major and interleaved,
// measured in output LSB units. Encoded is the source after transfer mapping.
type Result struct {
	Width          int
	Height         int
	Channels       int
	Bits           int
	DitherMode     DitherMode
	DitherStrength float64
	ShapingMode    ShapingMode
	Serpentine     bool
	Seed           int64
	TransferMode   TransferMode
	Quant          []uint8
	Encoded        *FloatImage
	Residual       []float32
	Feedback       []float32
	Dither         []float32
	ShapedError    []float32
	ClippedPixels  int
}

// Quantize runs transfer encoding, pre-quantization dither, error-feedback
// noise shaping, and integer rounding in one scan. The feedback error is
// captured after quantization and before diffusion, so it can be visualized
// independently from the low-bit residual.
func Quantize(src *FloatImage, cfg Config) (*Result, error) {
	if src == nil || src.Width <= 0 || src.Height <= 0 {
		return nil, fmt.Errorf("invalid source image")
	}
	if src.Channels != 1 && src.Channels != 3 {
		return nil, fmt.Errorf("unsupported channel count %d", src.Channels)
	}
	expected := int64(src.Width) * int64(src.Height) * int64(src.Channels)
	if expected <= 0 || expected > int64(^uint(0)>>1) || int64(len(src.Pix)) != expected {
		return nil, fmt.Errorf("source pixel buffer does not match dimensions")
	}
	if cfg.Bits == 0 {
		cfg.Bits = 8
	}
	if cfg.Bits < 1 || cfg.Bits > 8 {
		return nil, fmt.Errorf("bits must be between 1 and 8")
	}
	if cfg.Dither == "" {
		cfg.Dither = DitherNone
	}
	if cfg.Shaping == "" {
		cfg.Shaping = ShapingNone
	}
	if cfg.Transfer == "" {
		cfg.Transfer = TransferNone
	}
	if err := validateDither(cfg.Dither); err != nil {
		return nil, err
	}
	if err := validateTransfer(cfg.Transfer); err != nil {
		return nil, err
	}
	taps, err := diffusionKernel(cfg.Shaping)
	if err != nil {
		return nil, err
	}
	if cfg.DitherStrength < 0 || math.IsNaN(cfg.DitherStrength) || math.IsInf(cfg.DitherStrength, 0) {
		return nil, fmt.Errorf("dither strength must be finite and non-negative")
	}

	n := src.Width * src.Height * src.Channels
	result := &Result{
		Width:          src.Width,
		Height:         src.Height,
		Channels:       src.Channels,
		Bits:           cfg.Bits,
		DitherMode:     cfg.Dither,
		DitherStrength: cfg.DitherStrength,
		ShapingMode:    cfg.Shaping,
		Serpentine:     cfg.Serpentine,
		Seed:           cfg.Seed,
		TransferMode:   cfg.Transfer,
		Quant:          make([]uint8, n),
		Residual:       make([]float32, n),
		Feedback:       make([]float32, n),
		Dither:         make([]float32, n),
		ShapedError:    make([]float32, n),
	}
	encoded, clipped, err := applyTransfer(src, cfg.Transfer)
	if err != nil {
		return nil, err
	}
	result.Encoded = encoded
	result.ClippedPixels = clipped

	maxLevel := float64((uint64(1) << cfg.Bits) - 1)
	feedback := make([]float64, n)
	rng := rand.New(rand.NewSource(cfg.Seed))

	for y := 0; y < src.Height; y++ {
		reverse := cfg.Serpentine && y%2 == 1
		for step := 0; step < src.Width; step++ {
			x := step
			if reverse {
				x = src.Width - 1 - step
			}
			base := (y*src.Width + x) * src.Channels
			for c := 0; c < src.Channels; c++ {
				idx := base + c
				v := float64(encoded.Pix[idx]) * maxLevel
				fb := feedback[idx]
				d := ditherSample(rng, cfg.Dither) * cfg.DitherStrength
				decision := v + fb + d
				q := math.Floor(decision + 0.5)
				q = math.Max(0, math.Min(maxLevel, q))
				shaped := v + fb - q
				result.Quant[idx] = uint8(q)
				result.Residual[idx] = float32(v - q)
				result.Feedback[idx] = float32(fb)
				result.Dither[idx] = float32(d)
				result.ShapedError[idx] = float32(shaped)
				diffuse(feedback, src.Width, src.Height, src.Channels, x, y, c, shaped, reverse, taps)
			}
		}
	}
	return result, nil
}

func validateDither(mode DitherMode) error {
	switch mode {
	case DitherNone, DitherRectangular, DitherTPDF:
		return nil
	default:
		return fmt.Errorf("unknown dither mode %q", mode)
	}
}

func validateTransfer(mode TransferMode) error {
	switch mode {
	case TransferNone, TransferSRGB:
		return nil
	default:
		return fmt.Errorf("unknown transfer mode %q", mode)
	}
}

// ParseDither accepts stable CLI spellings.
func ParseDither(s string) (DitherMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "off":
		return DitherNone, nil
	case "rect", "rectangular", "uniform":
		return DitherRectangular, nil
	case "tpdf", "triangular":
		return DitherTPDF, nil
	default:
		return "", fmt.Errorf("dither must be none, rectangular, or tpdf")
	}
}

// ParseShaping accepts stable CLI spellings.
func ParseShaping(s string) (ShapingMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "off":
		return ShapingNone, nil
	case "fs", "floyd", "floyd-steinberg":
		return ShapingFloydSteinberg, nil
	case "atkinson":
		return ShapingAtkinson, nil
	case "sierra", "sierra-lite":
		return ShapingSierraLite, nil
	case "jjn", "jarvis", "jarvis-judice-ninke":
		return ShapingJarvisJudiceNinke, nil
	case "stucki":
		return ShapingStucki, nil
	default:
		return "", fmt.Errorf("shaping must be none, floyd-steinberg, atkinson, sierra-lite, jarvis-judice-ninke, or stucki")
	}
}

// ParseTransfer accepts stable CLI spellings.
func ParseTransfer(s string) (TransferMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "display", "display-referred":
		return TransferNone, nil
	case "srgb", "linear", "linear-srgb":
		return TransferSRGB, nil
	default:
		return "", fmt.Errorf("transfer must be none/display or srgb/linear")
	}
}

func ditherSample(rng *rand.Rand, mode DitherMode) float64 {
	switch mode {
	case DitherRectangular:
		return rng.Float64() - 0.5
	case DitherTPDF:
		return (rng.Float64() - 0.5) + (rng.Float64() - 0.5)
	default:
		return 0
	}
}

func applyTransfer(src *FloatImage, mode TransferMode) (*FloatImage, int, error) {
	out := src.Clone()
	if mode == TransferNone {
		for i, v := range out.Pix {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				out.Pix[i] = 0
			}
		}
		return out, countClippedPixels(src), nil
	}
	if mode != TransferSRGB {
		return nil, 0, fmt.Errorf("unsupported transfer %q", mode)
	}
	for i, v := range src.Pix {
		out.Pix[i] = float32(srgbEncode(float64(v)))
	}
	return out, countClippedPixels(src), nil
}

func srgbEncode(v float64) float64 {
	v = Clamp01(v)
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1.0/2.4) - 0.055
}

func countClippedPixels(im *FloatImage) int {
	count := 0
	for y := 0; y < im.Height; y++ {
		for x := 0; x < im.Width; x++ {
			clipped := false
			for c := 0; c < im.Channels; c++ {
				v := im.At(x, y, c)
				if v < 0 || v > 1 || math.IsNaN(float64(v)) {
					clipped = true
				}
			}
			if clipped {
				count++
			}
		}
	}
	return count
}

type diffusionTap struct {
	dx, dy int
	weight float64
}

func diffusionKernel(mode ShapingMode) ([]diffusionTap, error) {
	switch mode {
	case ShapingNone:
		return nil, nil
	case ShapingFloydSteinberg:
		return []diffusionTap{
			{1, 0, 7.0 / 16}, {-1, 1, 3.0 / 16}, {0, 1, 5.0 / 16}, {1, 1, 1.0 / 16},
		}, nil
	case ShapingAtkinson:
		return []diffusionTap{
			{1, 0, 1.0 / 8}, {2, 0, 1.0 / 8},
			{-1, 1, 1.0 / 8}, {0, 1, 1.0 / 8}, {1, 1, 1.0 / 8},
			{0, 2, 1.0 / 8},
		}, nil
	case ShapingSierraLite:
		return []diffusionTap{
			{1, 0, 2.0 / 4}, {-1, 1, 1.0 / 4}, {0, 1, 1.0 / 4},
		}, nil
	case ShapingJarvisJudiceNinke:
		return []diffusionTap{
			{1, 0, 7.0 / 48}, {2, 0, 5.0 / 48},
			{-2, 1, 3.0 / 48}, {-1, 1, 5.0 / 48}, {0, 1, 7.0 / 48}, {1, 1, 5.0 / 48}, {2, 1, 3.0 / 48},
			{-2, 2, 1.0 / 48}, {-1, 2, 3.0 / 48}, {0, 2, 5.0 / 48}, {1, 2, 3.0 / 48}, {2, 2, 1.0 / 48},
		}, nil
	case ShapingStucki:
		return []diffusionTap{
			{1, 0, 8.0 / 42}, {2, 0, 4.0 / 42},
			{-2, 1, 2.0 / 42}, {-1, 1, 4.0 / 42}, {0, 1, 8.0 / 42}, {1, 1, 4.0 / 42}, {2, 1, 2.0 / 42},
			{-2, 2, 1.0 / 42}, {-1, 2, 2.0 / 42}, {0, 2, 4.0 / 42}, {1, 2, 2.0 / 42}, {2, 2, 1.0 / 42},
		}, nil
	default:
		return nil, fmt.Errorf("unknown shaping mode %q", mode)
	}
}

func diffuse(feedback []float64, width, height, channels, x, y, c int, err float64, reverse bool, taps []diffusionTap) {
	for _, tap := range taps {
		tx := x + tap.dx
		if reverse {
			tx = x - tap.dx
		}
		ty := y + tap.dy
		if tx < 0 || tx >= width || ty < 0 || ty >= height {
			continue
		}
		idx := (ty*width+tx)*channels + c
		feedback[idx] += err * tap.weight
	}
}
