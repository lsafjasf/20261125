package main

import (
	"errors"
	"math"
)

// Rec. ITU-R BT.2100 HLG constants.
const (
	HLGA = 0.17883277
	HLGB = 0.28466892 // 1 - 4*HLGA
	HLGC = 0.55991073

	Kr2020 = 0.2627
	Kg2020 = 0.6780
	Kb2020 = 0.0593
)

// GammaLaw selects how nominal display peak maps to HLG system gamma.
type GammaLaw string

const (
	GammaAuto     GammaLaw = "auto"
	GammaStandard GammaLaw = "standard" // 1.2 + 0.42*log10(Lw/1000)
	GammaExtended GammaLaw = "extended" // BT.2100 extended-range power law
	GammaManual   GammaLaw = "manual"
)

// OOTFMode selects the display-side opto-optical transfer implementation.
type OOTFMode string

const (
	// OOTFBT2100 applies alpha * Ys^(gamma-1) * Es to every component.
	OOTFBT2100 OOTFMode = "bt2100"
	// OOTFScenePower uses alpha * Es^gamma per channel. Gray patches are
	// identical to BT.2100, while saturated highlights expose the missing
	// luminance-dependent channel coupling.
	OOTFScenePower OOTFMode = "scene-power"
	// OOTFLinearScene skips the system-gamma stage after inverse OETF.
	OOTFLinearScene OOTFMode = "linear-scene"
)

// RGB holds either HLG code values, scene-linear values, or nit values.
type RGB struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
}

func (v RGB) addScaled(other RGB, scale float64) RGB {
	return RGB{v.R + other.R*scale, v.G + other.G*scale, v.B + other.B*scale}
}

func (v RGB) scaled(scale float64) RGB {
	return RGB{v.R * scale, v.G * scale, v.B * scale}
}

func (v RGB) maxChannel() float64 {
	return math.Max(v.R, math.Max(v.G, v.B))
}

func (v RGB) clamped(low, high float64) RGB {
	clamp := func(x float64) float64 { return math.Max(low, math.Min(high, x)) }
	return RGB{clamp(v.R), clamp(v.G), clamp(v.B)}
}

// Luma2020 computes linear-light Rec.2020 luminance.
func Luma2020(v RGB) float64 {
	return Kr2020*v.R + Kg2020*v.G + Kb2020*v.B
}

// OETF encodes a scene-linear value in [0,1] to HLG E-prime.
func OETF(scene float64) float64 {
	if scene <= 1.0/12.0 {
		return math.Sqrt(3.0 * math.Max(0, scene))
	}
	return HLGA*math.Log(12.0*scene-HLGB) + HLGC
}

// InverseOETF decodes HLG E-prime to scene-linear light. Values above 1 are
// intentionally accepted because production HLG signals may contain super-white.
func InverseOETF(code float64) float64 {
	if code <= 0.5 {
		return code * code / 3.0
	}
	return (math.Exp((code-HLGC)/HLGA) + HLGB) / 12.0
}

// SystemGamma returns the HLG system gamma for a nominal peak in nits.
// A positive manual gamma always wins, making fixed-gamma mistakes reproducible.
func SystemGamma(peakNits, manualGamma float64, law GammaLaw) (float64, error) {
	if manualGamma > 0 {
		return manualGamma, nil
	}
	if peakNits <= 0 {
		return 0, errors.New("peak nits must be positive")
	}
	standard := 1.2 + 0.42*math.Log10(peakNits/1000.0)
	switch law {
	case GammaStandard:
		return standard, nil
	case GammaExtended:
		return 1.2 * math.Pow(1.111, math.Log2(peakNits/1000.0)), nil
	case GammaAuto:
		if peakNits >= 400 && peakNits <= 2000 {
			return standard, nil
		}
		return 1.2 * math.Pow(1.111, math.Log2(peakNits/1000.0)), nil
	default:
		return 0, errors.New("unknown gamma law: " + string(law))
	}
}

// Config is the adjustable HLG display pipeline.
type Config struct {
	Label         string   `json:"label"`
	PeakNits      float64  `json:"peak_nits"`
	AlphaNits     float64  `json:"alpha_nits"`
	BlackNits     float64  `json:"black_nits"`
	ManualGamma   float64  `json:"manual_gamma,omitempty"`
	GammaLaw      GammaLaw `json:"gamma_law"`
	OOTF          OOTFMode `json:"ootf"`
	ReferenceNits float64  `json:"reference_nits"`
}

func DefaultConfig(peakNits float64) Config {
	return Config{
		Label:         formatNits(peakNits),
		PeakNits:      peakNits,
		AlphaNits:     peakNits,
		BlackNits:     0,
		GammaLaw:      GammaAuto,
		OOTF:          OOTFBT2100,
		ReferenceNits: 1000,
	}
}

func (c Config) Validate() error {
	if c.PeakNits <= 0 {
		return errors.New("peak nits must be positive")
	}
	if c.AlphaNits < 0 || c.BlackNits < 0 {
		return errors.New("alpha and black nits must be non-negative")
	}
	if c.BlackNits >= c.PeakNits {
		return errors.New("black nits must be below peak nits")
	}
	if c.ReferenceNits <= 0 {
		return errors.New("reference nits must be positive")
	}
	if _, err := SystemGamma(c.PeakNits, c.ManualGamma, c.GammaLaw); err != nil {
		return err
	}
	switch c.OOTF {
	case OOTFBT2100, OOTFScenePower, OOTFLinearScene:
		return nil
	default:
		return errors.New("unknown OOTF mode: " + string(c.OOTF))
	}
}

func (c Config) Gamma() float64 {
	gamma, _ := SystemGamma(c.PeakNits, c.ManualGamma, c.GammaLaw)
	return gamma
}

func (c Config) Alpha() float64 {
	if c.AlphaNits > 0 {
		return c.AlphaNits
	}
	return c.PeakNits
}

// Beta is the standard HLG black-lift signal offset.
func (c Config) Beta() float64 {
	if c.BlackNits <= 0 {
		return 0
	}
	gamma := c.Gamma()
	return math.Sqrt(3 * math.Pow(c.BlackNits/c.PeakNits, 1/gamma))
}

// Sample contains every meaningful intermediate and absolute display value.
type Sample struct {
	Code           RGB     `json:"code"`
	AdjustedCode   RGB     `json:"adjusted_code"`
	Scene          RGB     `json:"scene"`
	SceneLuma      float64 `json:"scene_luma"`
	Gamma          float64 `json:"gamma"`
	AlphaNits      float64 `json:"alpha_nits"`
	Beta           float64 `json:"beta"`
	RawNits        RGB     `json:"raw_nits"`
	RawLumaNits    float64 `json:"raw_luma_nits"`
	DisplayNits    RGB     `json:"display_nits"`
	DisplayLuma    float64 `json:"display_luma_nits"`
	OverPeak       bool    `json:"over_peak"`
	ClippedChannel bool    `json:"clipped_channel"`
}

// Decode runs beta lift, inverse HLG OETF, selectable OOTF and hard peak clip.
func (c Config) Decode(code RGB) Sample {
	gamma := c.Gamma()
	alpha := c.Alpha()
	beta := c.Beta()

	adjusted := code.scaled(1-beta).addScaled(RGB{1, 1, 1}, beta)
	adjusted = adjusted.clamped(0, math.Inf(1))
	scene := RGB{InverseOETF(adjusted.R), InverseOETF(adjusted.G), InverseOETF(adjusted.B)}
	ys := Luma2020(scene)

	var raw RGB
	switch c.OOTF {
	case OOTFScenePower:
		raw = RGB{
			alpha * math.Pow(scene.R, gamma),
			alpha * math.Pow(scene.G, gamma),
			alpha * math.Pow(scene.B, gamma),
		}
	case OOTFLinearScene:
		raw = scene.scaled(alpha)
	default:
		factor := 0.0
		if ys > 1e-14 {
			factor = alpha * math.Pow(ys, gamma-1)
		}
		raw = scene.scaled(factor)
	}

	display := raw.clamped(0, c.PeakNits)
	rawLuma := Luma2020(raw)
	displayLuma := Luma2020(display)
	tolerance := 1e-7 * math.Max(1, c.PeakNits)
	over := raw.maxChannel() > c.PeakNits+tolerance
	channelClip := raw.R > c.PeakNits+tolerance || raw.G > c.PeakNits+tolerance || raw.B > c.PeakNits+tolerance

	return Sample{
		Code: code, AdjustedCode: adjusted, Scene: scene, SceneLuma: ys,
		Gamma: gamma, AlphaNits: alpha, Beta: beta,
		RawNits: raw, RawLumaNits: rawLuma,
		DisplayNits: display, DisplayLuma: displayLuma,
		OverPeak: over, ClippedChannel: channelClip,
	}
}

func (c Config) DecodeGray(code float64) Sample {
	return c.Decode(RGB{code, code, code})
}
