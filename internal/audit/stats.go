package audit

import "math"

type Accumulator struct {
	hist  [256]uint64
	total uint64
	min   uint8
	max   uint8
}

func NewAccumulator() *Accumulator {
	return &Accumulator{min: 255}
}

func (a *Accumulator) Add(v uint8) {
	a.hist[v]++
	a.total++
	if v < a.min {
		a.min = v
	}
	if v > a.max {
		a.max = v
	}
}

func (a *Accumulator) Merge(other *Accumulator) {
	if other == nil || other.total == 0 {
		return
	}
	for i, n := range other.hist {
		a.hist[i] += n
	}
	a.total += other.total
	if other.min < a.min {
		a.min = other.min
	}
	if other.max > a.max {
		a.max = other.max
	}
}

func (a *Accumulator) Total() uint64 {
	return a.total
}

func (a *Accumulator) Histogram() [256]uint64 {
	return a.hist
}

func (a *Accumulator) Stats(width, height, frames int) SampleStats {
	s := SampleStats{
		Samples:   a.total,
		Width:     width,
		Height:    height,
		Frames:    frames,
		Min:       int(a.min),
		Max:       int(a.max),
		Percent:   a.percentiles(),
		Fractions: a.fractions(),
	}
	if a.total == 0 {
		s.Min = 0
		s.Max = 0
	}
	for i, n := range a.hist {
		if n > 0 {
			s.Distinct++
		}
		if i < 16 {
			s.Counts.LT16 += n
		}
		if i == 16 {
			s.Counts.EQ16 = n
		}
		if i >= 16 && i <= 235 {
			s.Counts.IN += n
		}
		if i == 235 {
			s.Counts.EQ235 = n
		}
		if i > 235 {
			s.Counts.GT235 += n
		}
		if i == 0 {
			s.Counts.EQ0 = n
		}
		if i == 255 {
			s.Counts.EQ255 = n
		}
		if i < 64 {
			s.Counts.Below64 += n
		}
	}
	return s
}

func (a *Accumulator) fractions() Fractions {
	if a.total == 0 {
		return Fractions{}
	}
	f := func(n uint64) float64 {
		return float64(n) / float64(a.total)
	}
	var c CodeCounts
	for i, n := range a.hist {
		switch {
		case i < 16:
			c.LT16 += n
		case i == 16:
			c.EQ16 += n
		case i == 235:
			c.EQ235 += n
		case i > 16 && i < 235:
			c.IN += n
		case i > 235:
			c.GT235 += n
		}
		if i == 0 {
			c.EQ0 = n
		}
		if i == 255 {
			c.EQ255 = n
		}
		if i < 64 {
			c.Below64 += n
		}
	}
	return Fractions{
		LT16:    f(c.LT16),
		EQ16:    f(c.EQ16),
		EQ235:   f(c.EQ235),
		GT235:   f(c.GT235),
		IN:      f(c.IN + c.EQ16 + c.EQ235),
		Outside: f(c.LT16 + c.GT235),
		EQ0:     f(c.EQ0),
		EQ255:   f(c.EQ255),
		Below64: f(c.Below64),
	}
}

func (a *Accumulator) percentiles() Percentiles {
	return Percentiles{
		P001:  a.quantile(0.001),
		P01:   a.quantile(0.01),
		P05:   a.quantile(0.05),
		P25:   a.quantile(0.25),
		P50:   a.quantile(0.50),
		P75:   a.quantile(0.75),
		P95:   a.quantile(0.95),
		P99:   a.quantile(0.99),
		P999:  a.quantile(0.999),
		P9999: a.quantile(0.9999),
	}
}

func (a *Accumulator) quantile(p float64) int {
	if a.total == 0 {
		return 0
	}
	rank := uint64(math.Ceil(p * float64(a.total)))
	if rank == 0 {
		rank = 1
	}
	if rank > a.total {
		rank = a.total
	}
	var seen uint64
	for i, n := range a.hist {
		seen += n
		if seen >= rank {
			return i
		}
	}
	return 255
}
