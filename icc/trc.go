package icc

import (
	"math"
	"strconv"
)

// Eval applies the TRC to input x in [0,1], returning the encoded value.
// Returns ok=false when the curve failed validation and cannot be evaluated.
func (t *TRC) Eval(x float64) (float64, bool) {
	if t == nil {
		return 0, false
	}
	if x <= 0 {
		x = 0
	}
	if x >= 1 {
		x = 1
	}

	switch t.Kind {
	case TRCIdentity:
		return x, true
	case TRCGamma:
		return math.Pow(x, t.Gamma), true
	case TRCLUT:
		return evalLUT(t.Entries, x), true
	case TRCPara:
		return evalPara(t.ParaType, t.Params, x), true
	default:
		return 0, false
	}
}

func evalLUT(entries []float64, x float64) float64 {
	n := len(entries)
	if n == 0 {
		return x
	}
	if n == 1 {
		return math.Pow(x, entries[0])
	}
	pos := x * float64(n-1)
	lo := int(math.Floor(pos))
	if lo >= n-1 {
		return entries[n-1]
	}
	frac := pos - float64(lo)
	return entries[lo]*(1-frac) + entries[lo+1]*frac
}

// evalPara implements parametricCurveType formulas (ICC.1:2010, 6.A.4).
func evalPara(ft int, p []float64, x float64) float64 {
	g := p[0]
	switch ft {
	case 0:
		return math.Pow(x, g)
	case 1:
		a, b := p[1], p[2]
		if x >= -b/a {
			return math.Pow(max(a*x+b, 0), g)
		}
		return 0
	case 2:
		a, b, c, d := p[1], p[2], p[3], p[4]
		if x >= -b/a {
			return math.Pow(max(a*x+b, 0), g) + c
		}
		return c*x + d
	case 3:
		a, b, c, d := p[1], p[2], p[3], p[4]
		if x >= d {
			return math.Pow(max(a*x+b, 0), g)
		}
		return c * x
	default: // type 4
		a, b, c, d, e, f := p[1], p[2], p[3], p[4], p[5], p[6]
		if x >= d {
			return math.Pow(max(a*x+b, 0), g) + e
		}
		return c*x + f
	}
}

// Describe renders a short human-readable form of the curve.
func (t *TRC) Describe() string {
	if t == nil {
		return "unavailable"
	}
	switch t.Kind {
	case TRCIdentity:
		return "identity"
	case TRCGamma:
		return "gamma " + strconv.FormatFloat(t.Gamma, 'g', -1, 64)
	case TRCLUT:
		return "LUT " + itoa(len(t.Entries)) + " entries"
	case TRCPara:
		return "parametric type " + itoa(t.ParaType)
	default:
		return "unknown"
	}
}
