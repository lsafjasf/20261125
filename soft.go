package main

import "math"

// softKnee is the fraction of the boundary chroma where compression starts.
const softKnee = 0.85

// compressChroma maps an input chroma c at fixed (L, hue) onto the gamut.
// Below knee*boundary the color is untouched. Above it a rational soft
// rolloff takes over: C1-continuous at the knee, asymptotically bounded by
// the boundary chroma, and strictly monotonic. Hue and lightness are never
// altered, so no neon hue can jump to the opposite side of the hue wheel.
func compressChroma(c, boundary float64) float64 {
	k := softKnee * boundary
	if c <= k || boundary <= 0 {
		return c
	}
	out := k + (c-k)*(boundary-k)/((boundary-k)+(c-k))
	if math.IsNaN(out) {
		out = k
	}
	if out > boundary {
		out = boundary
	}
	return out
}

// softCompressLab maps an in- or out-of-gamut Lab color into the gamut with
// hue-preserving soft chroma compression at constant lightness. If the given
// lightness cannot hold any chroma for this hue (extreme L near the tips),
// lightness is eased toward L=50 until the hue plane becomes feasible.
func (g Gamut) softCompressLab(src Lab) Lab {
	p := labToLCh(src)
	boundary := g.maxChroma(p.L, p.H)
	L := p.L
	if boundary <= 1e-9 {
		// The lightness plane cannot hold this hue; ease to L=50,
		// which is always feasible for a regular RGB gamut.
		L = 50
		boundary = g.maxChroma(L, p.H)
	}
	mapped := LCh{L: L, C: compressChroma(p.C, boundary), H: p.H}
	if !g.containsLCh(mapped) {
		mapped.C = g.maxChroma(mapped.L, mapped.H)
	}
	return lchToLab(mapped)
}
