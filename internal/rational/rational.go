// Package rational provides exact rational arithmetic for timeline math.
// All event times are kept as exact fractions of a second so that
// frame-level and sample-level comparisons never accumulate float error.
package rational

import "fmt"

// Rat is an exact rational number Num/Den, always normalized (Den > 0,
// gcd(|Num|, Den) == 1).
type Rat struct {
	Num int64
	Den int64
}

func gcd(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

// New returns the normalized fraction num/den. den must be non-zero.
func New(num, den int64) Rat {
	if den == 0 {
		panic("rational: zero denominator")
	}
	if den < 0 {
		num, den = -num, -den
	}
	g := gcd(num, den)
	return Rat{Num: num / g, Den: den / g}
}

// FromInt returns n as a rational.
func FromInt(n int64) Rat { return Rat{Num: n, Den: 1} }

// Zero is the additive identity.
func Zero() Rat { return Rat{} }

func (r Rat) Add(o Rat) Rat { return New(r.Num*o.Den+o.Num*r.Den, r.Den*o.Den) }
func (r Rat) Sub(o Rat) Rat { return New(r.Num*o.Den-o.Num*r.Den, r.Den*o.Den) }
func (r Rat) Mul(o Rat) Rat { return New(r.Num*o.Num, r.Den*o.Den) }

func (r Rat) Div(o Rat) Rat {
	if o.Num == 0 {
		panic("rational: division by zero")
	}
	return New(r.Num*o.Den, r.Den*o.Num)
}

func (r Rat) Neg() Rat { return Rat{Num: -r.Num, Den: r.Den} }

func (r Rat) Abs() Rat {
	if r.Num < 0 {
		return r.Neg()
	}
	return r
}

// Cmp returns -1, 0 or +1.
func (r Rat) Cmp(o Rat) int {
	l, rr := r.Num*o.Den, o.Num*r.Den
	switch {
	case l < rr:
		return -1
	case l > rr:
		return 1
	default:
		return 0
	}
}

func (r Rat) IsZero() bool { return r.Num == 0 }

func (r Rat) Float64() float64 { return float64(r.Num) / float64(r.Den) }

// RoundInt64 rounds to the nearest integer, halves away from zero.
func (r Rat) RoundInt64() int64 {
	q := r.Num / r.Den
	rem := r.Num % r.Den
	if rem < 0 {
		rem = -rem
	}
	if 2*rem >= r.Den {
		if r.Num >= 0 {
			q++
		} else {
			q--
		}
	}
	return q
}

func (r Rat) String() string {
	if r.Den == 1 {
		return fmt.Sprintf("%d", r.Num)
	}
	return fmt.Sprintf("%d/%d", r.Num, r.Den)
}
