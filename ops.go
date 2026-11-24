package lutdiff

import "math"

// GammaA is LUT A: a per-channel nonlinear gamma curve. Because the curve is
// nonlinear, moving it before or after a channel-mixing saturation matrix
// changes the result.
const GammaA = 0.68

// SaturationB is LUT B's Rec.709-luma saturation gain. Values above 1 push
// highly saturated colors out of gamut, where clamping makes order matter
// even more near boundaries.
const SaturationB = 1.45

// Luma coefficients (Rec.709 / sRGB).
var luma709 = RGB{0.2126, 0.7152, 0.0722}

// TransformA is the continuous design function behind LUT A.
func TransformA(c RGB) RGB {
	return RGB{
		math.Pow(Clamp01(c[0]), GammaA),
		math.Pow(Clamp01(c[1]), GammaA),
		math.Pow(Clamp01(c[2]), GammaA),
	}
}

// TransformB is the continuous design function behind LUT B: luma-relative
// saturation followed by gamut clamping.
func TransformB(c RGB) RGB {
	luma := luma709[0]*c[0] + luma709[1]*c[1] + luma709[2]*c[2]
	s := SaturationB
	return RGB{
		Clamp01(luma + s*(c[0]-luma)),
		Clamp01(luma + s*(c[1]-luma)),
		Clamp01(luma + s*(c[2]-luma)),
	}
}

// Pipeline bundles the two baked 3D LUTs.
type Pipeline struct {
	A *LUT3D
	B *LUT3D
}

// NewPipeline builds both LUTs at gridSize^3 resolution. A 33^3 grid is the
// common desktop color-grading size.
func NewPipeline(gridSize int) *Pipeline {
	return &Pipeline{
		A: BuildLUT(gridSize, TransformA),
		B: BuildLUT(gridSize, TransformB),
	}
}

// AB applies A first and B second: result = B(A(c)).
func (p *Pipeline) AB(c RGB) RGB {
	return p.B.Apply(p.A.Apply(c))
}

// BA applies B first and A second: result = A(B(c)).
func (p *Pipeline) BA(c RGB) RGB {
	return p.A.Apply(p.B.Apply(c))
}

// ContinuousAB evaluates the design functions directly, without LUT
// quantization or interpolation.
func ContinuousAB(c RGB) RGB { return TransformB(TransformA(c)) }

// ContinuousBA is the reversed continuous composition.
func ContinuousBA(c RGB) RGB { return TransformA(TransformB(c)) }

// Group labels for the mandated test-sample families.
const (
	GroupNeutral   = "中性色"
	GroupSaturated = "高饱和色"
	GroupBoundary  = "接近边界值"
)

// Sample is a named input color with its test families.
type Sample struct {
	ID     string
	Name   string
	Groups []string
	Color  RGB
}

// DefaultSamples returns neutral, highly saturated and boundary-near inputs.
// Several samples intentionally belong to more than one family.
func DefaultSamples() []Sample {
	return []Sample{
		{"N0", "纯黑", []string{GroupNeutral, GroupBoundary}, RGB{0, 0, 0}},
		{"N1", "深中性灰 18%", []string{GroupNeutral}, RGB{0.18, 0.18, 0.18}},
		{"N2", "中中性灰 50%", []string{GroupNeutral}, RGB{0.5, 0.5, 0.5}},
		{"N3", "浅中性灰 80%", []string{GroupNeutral}, RGB{0.8, 0.8, 0.8}},
		{"N4", "纯白", []string{GroupNeutral, GroupBoundary}, RGB{1, 1, 1}},

		{"S1", "鲜红", []string{GroupSaturated}, RGB{0.86, 0.06, 0.10}},
		{"S2", "翠绿", []string{GroupSaturated}, RGB{0.05, 0.80, 0.18}},
		{"S3", "蓝紫", []string{GroupSaturated}, RGB{0.20, 0.10, 0.92}},
		{"S4", "橙黄", []string{GroupSaturated}, RGB{0.96, 0.55, 0.04}},
		{"S5", "青色", []string{GroupSaturated}, RGB{0.03, 0.85, 0.85}},
		{"S6", "品红", []string{GroupSaturated}, RGB{0.85, 0.05, 0.70}},

		{"E1", "近黑微暖", []string{GroupBoundary}, RGB{0.018, 0.010, 0.014}},
		{"E2", "暗部高饱和", []string{GroupBoundary, GroupSaturated}, RGB{0.34, 0.015, 0.025}},
		{"E3", "单通道近零", []string{GroupBoundary, GroupSaturated}, RGB{0.55, 0.020, 0.12}},
		{"E4", "近白偏暖", []string{GroupBoundary}, RGB{0.992, 0.965, 0.935}},
		{"E5", "高光青色", []string{GroupBoundary, GroupSaturated}, RGB{0.22, 0.990, 0.995}},
		{"E6", "次通道触顶", []string{GroupBoundary, GroupSaturated}, RGB{0.995, 0.82, 0.10}},
	}
}

// Region is a bitmask of input-region families used by the cube sweep.
type Region uint8

const (
	RegionNeutralBit Region = 1 << iota
	RegionSaturatedBit
	RegionBoundaryBit
)

// Classify assigns an input color to zero or more analysis regions.
//
//   - neutral:   channel spread <= 0.04
//   - saturated: HSV saturation >= 0.70 and value >= 0.20
//   - boundary:  any channel <= 0.03 or >= 0.97
func Classify(c RGB) Region {
	lo := math.Min(c[0], math.Min(c[1], c[2]))
	hi := math.Max(c[0], math.Max(c[1], c[2]))
	spread := hi - lo

	var r Region
	if spread <= 0.04 {
		r |= RegionNeutralBit
	}
	if hi >= 0.20 && spread/hi >= 0.70 {
		r |= RegionSaturatedBit
	}
	if lo <= 0.03 || hi >= 0.97 {
		r |= RegionBoundaryBit
	}
	return r
}
