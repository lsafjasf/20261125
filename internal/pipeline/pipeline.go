package pipeline

import (
	"fmt"
	"math"

	"logshoulder/internal/colorlog"
	"logshoulder/internal/display"
)

// LegalWhiteCode is the full-range normalization of a 10-bit legal-white code
// value (940). The faulty path treats it as display white before decoding Log.
const LegalWhiteCode = 940.0 / 1023.0

// Row records one scene patch at every relevant point in both paths.
type Row struct {
	Patch
	RawCode            float64
	Code               float64
	SourceClipped      bool
	DecodedScene       float64
	LegacyNormalized   float64
	LegacyDisplayCode  float64
	FixedDisplayLinear float64
	FixedDisplayCode   float64
	DirectDisplayCode  float64
}

// Conversion is one exposure sample through the faulty and repaired paths.
type Conversion struct {
	Case     Case
	Info     colorlog.Info
	Shoulder display.Shoulder
	Rows     []Row
}

// Run applies both conversions to a case.
func Run(c Case, shoulder display.Shoulder) (Conversion, error) {
	if err := shoulder.Validate(); err != nil {
		return Conversion{}, err
	}
	info := c.Curve.Describe(LegalWhiteCode)
	rows := make([]Row, 0, len(c.Patches()))
	for _, patch := range c.Patches() {
		raw := c.Curve.Encode(patch.Scene)
		code := clamp01(raw)
		decoded := c.Curve.Decode(code)
		legacyNorm := clamp01(code / LegalWhiteCode)
		fixedLinear := shoulder.Apply(decoded)
		rows = append(rows, Row{
			Patch:              patch,
			RawCode:            raw,
			Code:               code,
			SourceClipped:      raw >= 1,
			DecodedScene:       decoded,
			LegacyNormalized:   legacyNorm,
			LegacyDisplayCode:  display.SRGBEncode(legacyNorm),
			FixedDisplayLinear: fixedLinear,
			FixedDisplayCode:   display.SRGBEncode(fixedLinear),
			DirectDisplayCode:  display.SRGBEncode(math.Max(0, patch.Scene)),
		})
	}
	return Conversion{Case: c, Info: info, Shoulder: shoulder, Rows: rows}, nil
}

// Finding states which stage loses highlight ordering.
type Finding struct {
	Stage                  string
	Explanation            string
	PrematureClips         int
	SourceClips            int
	RecoverableCodeLevels  int
	RecoverableSceneLevels int
	FirstClippedCode       float64
	LastSourceCode         float64
	LegalWhiteScene        float64
	EncoderCeilingScene    float64
}

// Diagnose separates encoder saturation from a display-side clamp. The signal
// is considered recoverably clipped only when the camera code is below 1.0, so
// the encoder still has information, while the legacy display path is pinned at
// display white.
func (c Conversion) Diagnose() Finding {
	codes := make(map[float64]struct{})
	scenes := make(map[float64]struct{})
	premature, sourceClips := 0, 0
	first := math.Inf(1)
	lastSource := 0.0
	for _, row := range c.Rows {
		if row.SourceClipped {
			sourceClips++
			continue
		}
		lastSource = math.Max(lastSource, row.Code)
		if row.RawCode > LegalWhiteCode && row.LegacyNormalized >= 1 {
			premature++
			codes[row.Code] = struct{}{}
			scenes[row.DecodedScene] = struct{}{}
			first = math.Min(first, row.Code)
		}
	}
	if math.IsInf(first, 1) {
		first = 0
	}
	if premature > 0 {
		return Finding{
			Stage:                  "display-input legal-white normalization (before inverse Log)",
			Explanation:            "940/1023 以上的有效相机码被除以 940/1023 后钳到 1；反 Log 与显示肩部均未执行",
			PrematureClips:         premature,
			SourceClips:            sourceClips,
			RecoverableCodeLevels:  len(codes),
			RecoverableSceneLevels: len(scenes),
			FirstClippedCode:       first,
			LastSourceCode:         lastSource,
			LegalWhiteScene:        c.Info.LegalWhiteScene,
			EncoderCeilingScene:    c.Info.CeilingScene,
		}
	}
	stage := "none in current samples"
	if sourceClips > 0 {
		stage = "camera encoder ceiling (irrecoverable)"
	}
	return Finding{
		Stage:               stage,
		PrematureClips:      premature,
		SourceClips:         sourceClips,
		FirstClippedCode:    first,
		LastSourceCode:      lastSource,
		LegalWhiteScene:     c.Info.LegalWhiteScene,
		EncoderCeilingScene: c.Info.CeilingScene,
	}
}

// Metrics are the before/after comparison values shown in the report.
type Metrics struct {
	SourceHighlightLevels int
	LegacyHighlightLevels int
	FixedHighlightLevels  int
	LegacyClippedFraction float64
	LegacyHighlightSpan   float64
	FixedHighlightSpan    float64
	LegacyLastGap         float64
	FixedLastGap          float64
	LegacyWindowMaxCode   float64
	FixedWindowMaxCode    float64
	LegacyShadowLift      float64
	FixedShadowError      float64
	LegacyMidtoneError    float64
	FixedMidtoneError     float64
	FixedStrictlyOrdered  bool
}

// Measure compares both paths. Display codes are quantized to 16 bits before
// counting levels, so a mathematically tiny difference cannot masquerade as a
// visible layer.
func (c Conversion) Measure() Metrics {
	var src, legacy, fixed []float64
	var legacyCodes, fixedCodes []float64
	var shadowLift, fixedShadowErr, legacyMidErr, fixedMidErr []float64
	for _, row := range c.Rows {
		switch row.Role {
		case Highlight:
			src = append(src, quantize16(row.Code))
			legacyCodes = append(legacyCodes, quantize16(row.LegacyDisplayCode))
			fixedCodes = append(fixedCodes, quantize16(row.FixedDisplayCode))
			legacy = append(legacy, row.LegacyDisplayCode)
			fixed = append(fixed, row.FixedDisplayCode)
		case Shadow:
			shadowLift = append(shadowLift, row.LegacyDisplayCode-row.DirectDisplayCode)
			fixedShadowErr = append(fixedShadowErr, row.FixedDisplayCode-row.DirectDisplayCode)
		case Midtone:
			legacyMidErr = append(legacyMidErr, row.LegacyDisplayCode-row.DirectDisplayCode)
			fixedMidErr = append(fixedMidErr, row.FixedDisplayCode-row.DirectDisplayCode)
		}
	}
	m := Metrics{
		SourceHighlightLevels: unique(src),
		LegacyHighlightLevels: unique(legacyCodes),
		FixedHighlightLevels:  unique(fixedCodes),
		LegacyClippedFraction: fractionAtOne(legacy),
		LegacyHighlightSpan:   span(legacy),
		FixedHighlightSpan:    span(fixed),
		LegacyLastGap:         lastGap(legacyCodes),
		FixedLastGap:          lastGap(fixedCodes),
		LegacyWindowMaxCode:   maxFloat(legacy),
		FixedWindowMaxCode:    maxFloat(fixed),
		LegacyShadowLift:      mean(shadowLift),
		FixedShadowError:      meanAbs(fixedShadowErr),
		LegacyMidtoneError:    meanAbs(legacyMidErr),
		FixedMidtoneError:     meanAbs(fixedMidErr),
		FixedStrictlyOrdered:  strictlyOrdered(fixed),
	}
	return m
}

// Explain anchors the diagnosis in the measured curve.
func (c Conversion) Explain() string {
	f := c.Diagnose()
	return fmt.Sprintf(
		"%s 在 code 1.000 处仍支持 %.2fx 场景，而 legal-white code %.4f 只对应 %.2fx；因此钳位从 %.2fx 以上开始，但反 Log 后的样本仍保有 %d 个不同层次",
		c.Info.ShortName, c.Info.CeilingScene, LegalWhiteCode,
		c.Info.LegalWhiteScene, c.Info.LegalWhiteScene, f.RecoverableSceneLevels,
	)
}

func quantize16(v float64) float64 {
	return math.Round(clamp01(v)*65535) / 65535
}

func unique(v []float64) int {
	set := make(map[float64]struct{}, len(v))
	for _, x := range v {
		set[x] = struct{}{}
	}
	return len(set)
}

func fractionAtOne(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	n := 0
	for _, x := range v {
		if x >= 1-1e-12 {
			n++
		}
	}
	return float64(n) / float64(len(v))
}

func span(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	lo, hi := v[0], v[0]
	for _, x := range v[1:] {
		lo = math.Min(lo, x)
		hi = math.Max(hi, x)
	}
	return hi - lo
}

func lastGap(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	return v[len(v)-1] - v[len(v)-2]
}

func maxFloat(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		m = math.Max(m, x)
	}
	return m
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func meanAbs(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += math.Abs(x)
	}
	return sum / float64(len(v))
}

func strictlyOrdered(v []float64) bool {
	for i := 1; i < len(v); i++ {
		if v[i] <= v[i-1] {
			return false
		}
	}
	return true
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
