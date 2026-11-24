// Package colorlog implements the camera log transfer functions used by the
// diagnostic. Values are normalized scene-linear radiance and normalized log
// code, both on a 0..1 scale unless stated otherwise.
package colorlog

import (
	"fmt"
	"math"
)

// Kind identifies one of the published log transfer functions.
type Kind int

const (
	SLog3 Kind = iota
	LogC3
	ACEScct
)

// Curve is a camera log encoding and its inverse.
type Curve struct {
	Kind Kind
}

// Info contains the anchors used to explain where a display-side clamp cuts
// into a curve that is still carrying valid highlight information.
type Info struct {
	Name            string
	ShortName       string
	Encoding        string
	BlackCode       float64
	MidGrayCode     float64
	CeilingCode     float64
	CeilingScene    float64
	LegalWhiteCode  float64
	LegalWhiteScene float64
}

// Profiles returns the three known curves used by the exposure comparison.
func Profiles() []Curve {
	return []Curve{{Kind: SLog3}, {Kind: LogC3}, {Kind: ACEScct}}
}

// Encode maps scene-linear radiance to normalized log code. The toe is included,
// so black does not encode as zero code.
func (c Curve) Encode(scene float64) float64 {
	scene = math.Max(0, scene)
	switch c.Kind {
	case SLog3:
		if scene >= 0.01125 {
			return (math.Log10((scene+0.01)/0.19)*261.5 + 420.0) / 1023.0
		}
		return (scene*(171.2102946929-95.0)/0.01125 + 95.0) / 1023.0
	case LogC3:
		const (
			cut = 0.010591
			a   = 5.555556
			b   = 0.052272
			cc  = 0.247190
			d   = 0.385537
			e   = 5.367655
			f   = 0.092809
		)
		if scene > cut {
			return cc*math.Log10(a*scene+b) + d
		}
		return e*scene + f
	case ACEScct:
		if scene > 0.0078125 {
			return (math.Log2(scene) + 9.72) / 17.52
		}
		return 10.5402377416545*scene + 0.0729055341958355
	default:
		panic(fmt.Sprintf("unknown log curve %d", c.Kind))
	}
}

// Decode maps normalized log code back to scene-linear radiance.
func (c Curve) Decode(code float64) float64 {
	code = math.Max(0, code)
	switch c.Kind {
	case SLog3:
		cut := 171.2102946929 / 1023.0
		if code >= cut {
			return math.Pow(10, (code*1023.0-420.0)/261.5)*0.19 - 0.01
		}
		return (code*1023.0 - 95.0) * 0.01125 / (171.2102946929 - 95.0)
	case LogC3:
		const (
			cut = 0.149658
			a   = 5.555556
			b   = 0.052272
			cc  = 0.247190
			d   = 0.385537
			e   = 5.367655
			f   = 0.092809
		)
		if code > cut {
			return (math.Pow(10, (code-d)/cc) - b) / a
		}
		return (code - f) / e
	case ACEScct:
		if code > 0.155251141552511 {
			return math.Pow(2, code*17.52-9.72)
		}
		return (code - 0.0729055341958355) / 10.5402377416545
	default:
		panic(fmt.Sprintf("unknown log curve %d", c.Kind))
	}
}

// Name returns the manufacturer-facing name of the transfer function.
func (c Curve) Name() string {
	switch c.Kind {
	case SLog3:
		return "Sony S-Log3"
	case LogC3:
		return "ARRI LogC3 (EI 800)"
	case ACEScct:
		return "ACEScct"
	default:
		return "unknown"
	}
}

// ShortName returns a compact identifier for reports.
func (c Curve) ShortName() string {
	switch c.Kind {
	case SLog3:
		return "S-Log3"
	case LogC3:
		return "LogC3"
	case ACEScct:
		return "ACEScct"
	default:
		return "unknown"
	}
}

// Describe reports the curve anchors and the scene value at which a 10-bit
// legal-white clamp starts discarding code values.
func (c Curve) Describe(legalWhiteCode float64) Info {
	return Info{
		Name:            c.Name(),
		ShortName:       c.ShortName(),
		Encoding:        c.encoding(),
		BlackCode:       c.Encode(0),
		MidGrayCode:     c.Encode(0.18),
		CeilingCode:     1,
		CeilingScene:    c.Decode(1),
		LegalWhiteCode:  legalWhiteCode,
		LegalWhiteScene: c.Decode(legalWhiteCode),
	}
}

func (c Curve) encoding() string {
	switch c.Kind {
	case SLog3:
		return "piecewise Sony S-Log3; 18% Gray -> 420/1023"
	case LogC3:
		return "piecewise ARRI LogC3 EI 800; 18% Gray -> 0.391"
	case ACEScct:
		return "piecewise ACEScct; 18% Gray -> 0.414"
	default:
		return "unknown"
	}
}
