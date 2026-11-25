package icc

import "math"

// lutQuantization compares each encoded LUT entry against reference at the
// uniform ICC node position x_i = i/(n-1). The reference is typically the
// analytic TRC (gamma/para) that the LUT was sampled from; when no analytic
// partner exists the u16Fixed16 rounding grid alone is reported.
func lutQuantization(lut *TRC, reference *TRC) *QuantStats {
	if lut == nil || lut.Kind != TRCLUT || len(lut.Entries) < 2 {
		return nil
	}
	n := len(lut.Entries)
	q := &QuantStats{Present: true, Entries: n}
	var sumSq, maxAbs float64
	for i, stored := range lut.Entries {
		x := float64(i) / float64(n-1)
		var ideal float64
		if reference != nil {
			ideal, _ = reference.Eval(x)
		} else {
			// No analytic reference: bound by one half of a u16 code step.
			ideal = stored
		}
		d := math.Abs(stored - ideal)
		sumSq += d * d
		if d > maxAbs {
			maxAbs = d
		}
	}
	q.MaxAbs = maxAbs
	q.RMS = math.Sqrt(sumSq / float64(n))
	return q
}
