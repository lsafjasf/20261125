package mixloud

import "math"

type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	x1, x2     float64
	y1, y2     float64
}

func (f *biquad) process(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1 = f.x1, x
	f.y2, f.y1 = f.y1, y
	return y
}

func (f *biquad) reset() {
	f.x1, f.x2, f.y1, f.y2 = 0, 0, 0, 0
}

type kWeight struct {
	first  biquad
	second biquad
}

func newKWeight(sampleRate int) *kWeight {
	k := &kWeight{}
	if sampleRate == 48000 {
		k.first = biquad{
			b0: 1.53512485958697,
			b1: -2.69169618940638,
			b2: 1.19839281085285,
			a1: -1.69065929318241,
			a2: 0.73248077421585,
		}
		k.second = biquad{
			b0: 1,
			b1: -2,
			b2: 1,
			a1: -1.99004745483399,
			a2: 0.99007225036621,
		}
		return k
	}

	k.first = highShelf(sampleRate, 1681.974450955533, 3.999843853973347, 0.7071752369554196)
	k.second = highPass(sampleRate, 38.13547087602189, 0.5003270373238773)
	return k
}

func highPass(fs int, freq, q float64) biquad {
	w0 := 2 * math.Pi * freq / float64(fs)
	cw, sw := math.Cos(w0), math.Sin(w0)
	alpha := sw / (2 * q)
	a0 := 1 + alpha
	return biquad{
		b0: (1 + cw) / 2 / a0,
		b1: -(1 + cw) / a0,
		b2: (1 + cw) / 2 / a0,
		a1: -2 * cw / a0,
		a2: (1 - alpha) / a0,
	}
}

func highShelf(fs int, freq, gainDB, q float64) biquad {
	a := math.Pow(10, gainDB/40)
	w0 := 2 * math.Pi * freq / float64(fs)
	cw, sw := math.Cos(w0), math.Sin(w0)
	alpha := sw / (2 * q)
	root := 2 * math.Sqrt(a) * alpha
	a0 := (a + 1) - (a-1)*cw + root
	return biquad{
		b0: a * ((a + 1) + (a-1)*cw + root) / a0,
		b1: -2 * a * ((a - 1) + (a+1)*cw) / a0,
		b2: a * ((a + 1) + (a-1)*cw - root) / a0,
		a1: 2 * ((a - 1) - (a+1)*cw) / a0,
		a2: ((a + 1) - (a-1)*cw - root) / a0,
	}
}

func applyKWeight(a *Audio) [][]float64 {
	out := make([][]float64, len(a.Channels))
	for ch, samples := range a.Channels {
		filter := newKWeight(a.SampleRate)
		out[ch] = make([]float64, len(samples))
		for i, x := range samples {
			out[ch][i] = filter.second.process(filter.first.process(x))
		}
	}
	return out
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := (p / 100) * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}
