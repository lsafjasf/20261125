package lut3d

import "math"

// RGB is a three-component color sample. Field order is R, G, B.
type RGB struct {
	R float64
	G float64
	B float64
}

// AxisDomain is one closed axis interval.
type AxisDomain struct {
	Min float64
	Max float64
}

// Step returns the distance between adjacent grid nodes on this axis.
func (d AxisDomain) Step(size int) float64 {
	return (d.Max - d.Min) / float64(size-1)
}

// LUT is a fully validated, immutable-by-convention 3D lookup table.
//
// Entries use the CUBE ordering: red varies fastest, then green, then blue.
// For a size N table, (r,g,b) is stored at r + N*(g + N*b).
type LUT struct {
	Title   string
	Size    int
	Domain  [3]AxisDomain
	Step    [3]float64
	Entries []RGB
}

func clamp01(v float64) float64 {
	switch {
	case math.IsNaN(v):
		return math.NaN()
	case v <= 0:
		return 0
	case v >= 1:
		return 1
	default:
		return v
	}
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func (l *LUT) at(r, g, b int) RGB {
	return l.Entries[r+l.Size*(g+l.Size*b)]
}

// Sample is a component-wise convenience wrapper around Lookup.
func (l *LUT) Sample(r, g, b float64) RGB {
	return l.Lookup(RGB{R: r, G: g, B: b})
}

// Lookup performs trilinear interpolation after mapping each component from its
// closed domain to [0,1]. Inputs outside the domain clamp to the nearest face.
// A NaN component produces a NaN result rather than an out-of-range index.
func (l *LUT) Lookup(in RGB) RGB {
	u := clamp01((in.R - l.Domain[0].Min) / (l.Domain[0].Max - l.Domain[0].Min))
	v := clamp01((in.G - l.Domain[1].Min) / (l.Domain[1].Max - l.Domain[1].Min))
	w := clamp01((in.B - l.Domain[2].Min) / (l.Domain[2].Max - l.Domain[2].Min))
	if math.IsNaN(u) || math.IsNaN(v) || math.IsNaN(w) {
		return RGB{R: math.NaN(), G: math.NaN(), B: math.NaN()}
	}

	last := l.Size - 1
	r0, r1, fr := cell(u, last, l.Size)
	g0, g1, fg := cell(v, last, l.Size)
	b0, b1, fb := cell(w, last, l.Size)

	c000 := l.at(r0, g0, b0)
	c100 := l.at(r1, g0, b0)
	c010 := l.at(r0, g1, b0)
	c110 := l.at(r1, g1, b0)
	c001 := l.at(r0, g0, b1)
	c101 := l.at(r1, g0, b1)
	c011 := l.at(r0, g1, b1)
	c111 := l.at(r1, g1, b1)

	c00 := RGB{
		R: lerp(c000.R, c100.R, fr),
		G: lerp(c000.G, c100.G, fr),
		B: lerp(c000.B, c100.B, fr),
	}
	c10 := RGB{
		R: lerp(c010.R, c110.R, fr),
		G: lerp(c010.G, c110.G, fr),
		B: lerp(c010.B, c110.B, fr),
	}
	c01 := RGB{
		R: lerp(c001.R, c101.R, fr),
		G: lerp(c001.G, c101.G, fr),
		B: lerp(c001.B, c101.B, fr),
	}
	c11 := RGB{
		R: lerp(c011.R, c111.R, fr),
		G: lerp(c011.G, c111.G, fr),
		B: lerp(c011.B, c111.B, fr),
	}
	c0 := RGB{
		R: lerp(c00.R, c10.R, fg),
		G: lerp(c00.G, c10.G, fg),
		B: lerp(c00.B, c10.B, fg),
	}
	c1 := RGB{
		R: lerp(c01.R, c11.R, fg),
		G: lerp(c01.G, c11.G, fg),
		B: lerp(c01.B, c11.B, fg),
	}
	return RGB{
		R: lerp(c0.R, c1.R, fb),
		G: lerp(c0.G, c1.G, fb),
		B: lerp(c0.B, c1.B, fb),
	}
}

func cell(u float64, last, size int) (int, int, float64) {
	if u <= 0 {
		return 0, 0, 0
	}
	if u >= 1 {
		return last, last, 0
	}
	pos := u * float64(last)
	i := int(math.Floor(pos))
	if i >= last {
		return last, last, 0
	}
	return i, i + 1, pos - float64(i)
}
