package colorconv

import "math/rand"

// Samples builds the shared test corpus for a bit depth:
//   - all channel extrema (0 / max), i.e. saturated pure colors + black/white
//   - ramps and mid-gray
//   - a coarse regular grid over the whole cube
//   - deterministic pseudo-random samples
func Samples(bits int) []RGB {
	max := (1 << bits) - 1
	var out []RGB

	// 8 corners: every channel at an extreme -> saturated pure colors.
	for _, r := range []int{0, max} {
		for _, g := range []int{0, max} {
			for _, b := range []int{0, max} {
				out = append(out, RGB{r, g, b})
			}
		}
	}

	// Edge midpoints and grayscale ramp keep average-color behavior visible.
	mid := max / 2
	for _, v := range []int{0, max / 4, mid, max * 3 / 4, max} {
		out = append(out, RGB{v, v, v})
	}
	out = append(out,
		RGB{max, 0, 0}, RGB{0, max, 0}, RGB{0, 0, max},
		RGB{max, max, 0}, RGB{max, 0, max}, RGB{0, max, max},
		RGB{max, mid, 0}, RGB{0, mid, max}, RGB{mid, 0, max},
	)

	// Regular coarse grid (stride keeps 10-bit tractable).
	step := max / 8
	for r := 0; r <= max; r += step {
		for g := 0; g <= max; g += step {
			for b := 0; b <= max; b += step {
				out = append(out, RGB{r, g, b})
			}
		}
	}

	// Deterministic random fill.
	rng := rand.New(rand.NewSource(int64(bits * 7919)))
	for i := 0; i < 2000; i++ {
		out = append(out, RGB{
			R: rng.Intn(max + 1),
			G: rng.Intn(max + 1),
			B: rng.Intn(max + 1),
		})
	}
	return out
}

// Stats summarizes an error distribution in code-value units.
type Stats struct {
	MaxAbs  int
	MeanAbs float64
	RMS     float64
	Count   int
	Bias    float64 // mean SIGNED error: nonzero => systematic rounding bias
}

// RoundTrip runs encode then decode and measures RGB reconstruction error
// against the original pixels.
func RoundTrip(enc Encoder, samples []RGB) Stats {
	var sum, sqSum, signed float64
	var maxAbs, n int
	for _, in := range samples {
		out := enc.Decode(enc.Encode(in))
		for _, d := range [3]int{
			out.R - in.R,
			out.G - in.G,
			out.B - in.B,
		} {
			signed += float64(d)
			ad := d
			if ad < 0 {
				ad = -ad
			}
			if ad > maxAbs {
				maxAbs = ad
			}
			sum += float64(ad)
			sqSum += float64(ad) * float64(ad)
			n++
		}
	}
	return Stats{
		MaxAbs:  maxAbs,
		MeanAbs: sum / float64(n),
		RMS:     sqrt(sqSum / float64(n)),
		Count:   n,
		Bias:    signed / float64(n),
	}
}

// ForwardDrift compares an implementation's YUV output against the float64
// reference, isolating encode-side rounding from the full round trip.
func ForwardDrift(enc Encoder, ref FloatEncoder, samples []RGB) Stats {
	var sum, sqSum, signed float64
	var maxAbs, n int
	for _, in := range samples {
		got := enc.Encode(in)
		want := ref.Encode(in)
		for _, d := range [3]int{
			got.Y - want.Y,
			got.U - want.U,
			got.V - want.V,
		} {
			signed += float64(d)
			ad := d
			if ad < 0 {
				ad = -ad
			}
			if ad > maxAbs {
				maxAbs = ad
			}
			sum += float64(ad)
			sqSum += float64(ad) * float64(ad)
			n++
		}
	}
	return Stats{
		MaxAbs:  maxAbs,
		MeanAbs: sum / float64(n),
		RMS:     sqrt(sqSum / float64(n)),
		Count:   n,
		Bias:    signed / float64(n),
	}
}

// Saturated returns only the high-saturation pure colors (cube corners and
// edge midpoints), the samples on which truncation bias is most visible.
func Saturated(bits int) []RGB {
	max := (1 << bits) - 1
	mid := max / 2
	return []RGB{
		{max, 0, 0}, {0, max, 0}, {0, 0, max},
		{max, max, 0}, {max, 0, max}, {0, max, max},
		{max, mid, 0}, {0, mid, max}, {mid, 0, max},
		{max, max, max}, {0, 0, 0},
	}
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 24; i++ {
		z = (z + x/z) / 2
	}
	return z
}
