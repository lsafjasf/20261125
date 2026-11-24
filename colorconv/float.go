package colorconv

import "math"

// RGB is one pixel in the code-value domain of the spec's bit depth.
type RGB struct{ R, G, B int }

// YUV is one pixel in the code-value domain of the spec's bit depth.
type YUV struct{ Y, U, V int }

// FloatEncoder is the float64 reference. Every step (offset removal,
// matrix multiply, range de-normalization, quantization) is explicit so the
// integer versions have an unambiguous target to be compared against.
type FloatEncoder struct{ Spec Spec }

func (e FloatEncoder) Encode(p RGB) YUV {
	s := e.Spec
	max := float64(s.MaxCode())
	rn, gn, bn := float64(p.R)/max, float64(p.G)/max, float64(p.B)/max

	y := s.Matrix.Kr()*rn + s.Matrix.Kg()*gn + s.Matrix.Kb()*bn
	pb := (bn - y) / (2 * (1 - s.Matrix.Kb()))
	pr := (rn - y) / (2 * (1 - s.Matrix.Kr()))

	var yv, uv, vv float64
	if s.Range == Full {
		yv = max * y
		uv = max/2 + max*pb
		vv = max/2 + max*pr
	} else {
		yv = float64(s.YMin()) + s.YScale()*y
		uv = float64(s.COff()) + s.CScale()*pb
		vv = float64(s.COff()) + s.CScale()*pr
	}
	return YUV{
		Y: clamp(int(math.Round(yv)), s.YMin(), s.YMax()),
		U: clamp(int(math.Round(uv)), s.CMin(), s.CMax()),
		V: clamp(int(math.Round(vv)), s.CMin(), s.CMax()),
	}
}

func (e FloatEncoder) Decode(p YUV) RGB {
	s := e.Spec
	max := float64(s.MaxCode())

	var yn, pbn, prn float64
	if s.Range == Full {
		yn = clampF(float64(p.Y)/max, 0, 1)
		pbn = (float64(p.U) - max/2) / max
		prn = (float64(p.V) - max/2) / max
	} else {
		yn = clampF((float64(p.Y)-float64(s.YMin()))/s.YScale(), 0, 1)
		pbn = (float64(p.U) - float64(s.COff())) / s.CScale()
		prn = (float64(p.V) - float64(s.COff())) / s.CScale()
	}

	kr, kb, kg := s.Matrix.Kr(), s.Matrix.Kb(), s.Matrix.Kg()
	rn := yn + 2*(1-kr)*prn
	bn := yn + 2*(1-kb)*pbn
	gn := yn - 2*kb*(1-kb)/kg*pbn - 2*kr*(1-kr)/kg*prn

	return RGB{
		R: clamp(int(math.Round(clampF(rn, 0, 1)*max)), 0, s.MaxCode()),
		G: clamp(int(math.Round(clampF(gn, 0, 1)*max)), 0, s.MaxCode()),
		B: clamp(int(math.Round(clampF(bn, 0, 1)*max)), 0, s.MaxCode()),
	}
}
