package colorconv

// IntegerEncoder is the classic small-Q integer implementation (libyuv /
// IJG style, typically Q=8): same arithmetic as FixedEncoder with the low
// coefficient precision seen in production code. All intermediates are int64.
//
// Round=true uses symmetric round-half-away-from-zero. Round=false uses plain
// arithmetic right shift (floor for negative values), the naive truncation
// that biases saturated chroma samples.
//
// Scaling convention used throughout: a value tagged Nq is stored as
// value * 2^Q. Products of two Nq values are 2Nq and need one shift by Q.
type IntegerEncoder struct {
	Spec  Spec
	Q     uint
	Round bool
}

// NewNaiveInteger returns the deliberately uncorrected Q=8 floor-shift
// implementation whose saturated-color bias the test suite must expose.
func NewNaiveInteger(s Spec) IntegerEncoder {
	return IntegerEncoder{Spec: s, Q: 8, Round: false}
}

// NewRoundedInteger returns the same table precision with symmetric
// rounding correction terms at every shift site.
func NewRoundedInteger(s Spec) IntegerEncoder {
	return IntegerEncoder{Spec: s, Q: 8, Round: true}
}

func (e IntegerEncoder) shr(v int64, bits uint) int64 {
	if !e.Round {
		return v >> bits
	}
	half := int64(1) << (bits - 1)
	if v >= 0 {
		return (v + half) >> bits
	}
	return (v - half) >> bits
}

func (e IntegerEncoder) Encode(p RGB) YUV {
	s := e.Spec
	q := e.Q
	max := int64(s.MaxCode())

	kr := fq(s.Matrix.Kr(), q)
	kg := fq(s.Matrix.Kg(), q)
	kb := fq(s.Matrix.Kb(), q)

	// lumaNq = yNorm * max * 2^Q
	lumaNq := kr*int64(p.R) + kg*int64(p.G) + kb*int64(p.B)

	var yCode int64
	if s.Range == Full {
		yCode = e.shr(lumaNq, q)
	} else {
		yScale := fq(s.YScale()/float64(max), q) // 2Q product -> code
		yCode = int64(s.YMin()) + e.shr(lumaNq*yScale, 2*q)
	}

	// dbNq/drNq = (channel-yNorm) * max * 2^Q
	dbNq := int64(p.B)<<q - lumaNq
	drNq := int64(p.R)<<q - lumaNq

	var ub, vr int64 // multiplier mapping dbNq onto the chroma code excursion
	if s.Range == Full {
		ub = fq(0.5/(1-s.Matrix.Kb()), q)
		vr = fq(0.5/(1-s.Matrix.Kr()), q)
	} else {
		ub = fq(s.CScale()/float64(max)*0.5/(1-s.Matrix.Kb()), q)
		vr = fq(s.CScale()/float64(max)*0.5/(1-s.Matrix.Kr()), q)
	}
	neutralNq := s.cNeutralNq(q)
	uNq := neutralNq + e.shr(ub*dbNq, q)
	vNq := neutralNq + e.shr(vr*drNq, q)
	uCode := e.shr(uNq, q)
	vCode := e.shr(vNq, q)

	return YUV{
		Y: clamp(int(yCode), s.YMin(), s.YMax()),
		U: clamp(int(uCode), s.CMin(), s.CMax()),
		V: clamp(int(vCode), s.CMin(), s.CMax()),
	}
}

func (e IntegerEncoder) Decode(p YUV) RGB {
	s := e.Spec
	q := e.Q
	max := int64(s.MaxCode())
	neutralNq := s.cNeutralNq(q)

	// lumaNq = yNorm * max * 2^Q, i.e. normalized luma in code-sized units.
	var lumaNq int64
	if s.Range == Full {
		lumaNq = int64(p.Y) << q
	} else {
		invY := fq(float64(max)/s.YScale(), q)
		lumaNq = int64(p.Y-s.YMin()) * invY
	}

	// Recover dbNq/drNq = (channel-yNorm) * max * 2^Q from chroma codes.
	var invUb, invVr int64
	if s.Range == Full {
		invUb = fq(2*(1-s.Matrix.Kb()), q)
		invVr = fq(2*(1-s.Matrix.Kr()), q)
	} else {
		invUb = fq(2*(1-s.Matrix.Kb())*float64(max)/s.CScale(), q)
		invVr = fq(2*(1-s.Matrix.Kr())*float64(max)/s.CScale(), q)
	}
	// code (unscaled) * Nq coefficient is an Nq product: shift by Q once.
	dbNq := e.shr((int64(p.U)<<q-neutralNq)*invUb, q)
	drNq := e.shr((int64(p.V)<<q-neutralNq)*invVr, q)

	gb := fq(s.Matrix.Kb()/s.Matrix.Kg(), q)
	gr := fq(s.Matrix.Kr()/s.Matrix.Kg(), q)

	// Reconstruct normalized channels, all at Nq; one final shift by Q.
	rNq := lumaNq + drNq
	bNq := lumaNq + dbNq
	gNq := lumaNq - e.shr(gb*dbNq, q) - e.shr(gr*drNq, q)

	return RGB{
		R: clamp(int(e.shr(rNq, q)), 0, s.MaxCode()),
		G: clamp(int(e.shr(gNq, q)), 0, s.MaxCode()),
		B: clamp(int(e.shr(bNq, q)), 0, s.MaxCode()),
	}
}
