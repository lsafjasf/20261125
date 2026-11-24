package colorconv

// Encoder is the common surface exercised by the comparison harness.
type Encoder interface {
	Encode(RGB) YUV
	Decode(YUV) RGB
}

// MatrixMismatchEncoder encodes with one matrix and decodes with another.
// This models switching between the two RGB/YUV formula sets without
// carrying the matrix tag along: averages stay close, but saturated hues
// swing visibly.
type MatrixMismatchEncoder struct {
	EncodeSpec Spec
	DecodeSpec Spec
}

func (e MatrixMismatchEncoder) Encode(p RGB) YUV {
	return FloatEncoder{e.EncodeSpec}.Encode(p)
}

func (e MatrixMismatchEncoder) Decode(p YUV) RGB {
	return FloatEncoder{e.DecodeSpec}.Decode(p)
}

// OffsetlessEncoder computes a correct limited-range matrix but forgets the
// 16/128 offsets: encode emits excursion-only values, decode treats the raw
// excursion as full-range code values. This is the offset-stage ordering bug
// that shifts every output (and clips near black/white).
type OffsetlessEncoder struct{ Spec Spec }

func (e OffsetlessEncoder) Encode(p RGB) YUV {
	s := e.Spec
	yuv := FloatEncoder{s}.Encode(p)
	yuv.Y -= s.YMin()
	yuv.U -= s.COff()
	yuv.V -= s.COff()
	return yuv
}

func (e OffsetlessEncoder) Decode(p YUV) RGB {
	s := e.Spec
	full := s
	full.Range = Full
	return FloatEncoder{full}.Decode(YUV{
		Y: clamp(p.Y, 0, s.MaxCode()),
		U: clamp(p.U, 0, s.MaxCode()),
		V: clamp(p.V, 0, s.MaxCode()),
	})
}

// RangeConfusionEncoder encodes limited range but decodes as full (or vice
// versa), the other half of the range/offset ordering defect.
type RangeConfusionEncoder struct {
	EncodeRange Range
	DecodeRange Range
	Spec        Spec
}

func (e RangeConfusionEncoder) Encode(p RGB) YUV {
	s := e.Spec
	s.Range = e.EncodeRange
	return FloatEncoder{s}.Encode(p)
}

func (e RangeConfusionEncoder) Decode(p YUV) RGB {
	s := e.Spec
	s.Range = e.DecodeRange
	return FloatEncoder{s}.Decode(p)
}

// SwappedUVEncoder keeps a correct matrix but transposes U and V on the
// decode path, representing a channel-order (matrix application order) bug.
type SwappedUVEncoder struct{ Spec Spec }

func (e SwappedUVEncoder) Encode(p RGB) YUV {
	return FloatEncoder{e.Spec}.Encode(p)
}

func (e SwappedUVEncoder) Decode(p YUV) RGB {
	return FloatEncoder{e.Spec}.Decode(YUV{p.Y, p.V, p.U})
}
