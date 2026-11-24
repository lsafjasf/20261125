package colorconv

import (
	"math"
	"strconv"
)

// Matrix selects the luma/chroma coefficient set.
//
// Both BT.601 and BT.709 share the form Y = Kr*R + (1-Kr-Kb)*G + Kb*B;
// only Kr/Kb differ. Using the wrong matrix on the inverse path is the
// classic "matrix order / matrix mismatch" rounding-and-hue defect.
type Matrix int

const (
	BT601 Matrix = iota // Kr=0.2990 Kb=0.1140 (SDTV / JFIF)
	BT709               // Kr=0.2126 Kb=0.0722 (HDTV)
)

func (m Matrix) Kr() float64 {
	if m == BT709 {
		return 0.2126
	}
	return 0.299
}

func (m Matrix) Kb() float64 {
	if m == BT709 {
		return 0.0722
	}
	return 0.114
}

func (m Matrix) Kg() float64 { return 1 - m.Kr() - m.Kb() }

func (m Matrix) String() string {
	if m == BT709 {
		return "BT.709"
	}
	return "BT.601"
}

// Range selects studio (limited) vs full (JFPEG/JPEG) code-value mapping.
type Range int

const (
	Limited Range = iota // 16..235 luma / 16..240 chroma at 8-bit
	Full                 // 0..255 at any depth
)

func (r Range) String() string {
	if r == Full {
		return "full"
	}
	return "limited"
}

// Spec fully describes one RGB<->YUV conversion for a given bit depth.
// All offsets and scale factors are expressed in the code-value domain of
// that depth, which the integer/fixed-point implementations consume.
type Spec struct {
	Matrix Matrix
	Range  Range
	Bits   int
}

func (s Spec) String() string {
	return s.Matrix.String() + "/" + s.Range.String() + "/" + strconv.Itoa(s.Bits) + "bit"
}

// MaxCode returns the peak code value (255 for 8-bit, 1023 for 10-bit).
func (s Spec) MaxCode() int { return (1 << s.Bits) - 1 }

// YMin/YMax are the luma excursion limits (limited range only).
func (s Spec) YMin() int {
	if s.Range == Full {
		return 0
	}
	return 16 << (s.Bits - 8)
}

func (s Spec) YMax() int {
	if s.Range == Full {
		return s.MaxCode()
	}
	return 235 << (s.Bits - 8)
}

// CMin/CMax are the chroma excursion limits (limited range only).
func (s Spec) CMin() int {
	if s.Range == Full {
		return 0
	}
	return 16 << (s.Bits - 8)
}

func (s Spec) CMax() int {
	if s.Range == Full {
		return s.MaxCode()
	}
	return 240 << (s.Bits - 8)
}

// COff is the chroma neutral (zero-color) code value, and also the limited
// range luma floor. It must be added/subtracted at the correct stage:
// before the matrix on decode, after the matrix on encode.
func (s Spec) COff() int { return 128 << (s.Bits - 8) }

// cNeutralNq returns the zero-chroma offset expressed at fixed-point scale
// 2^q. Full range is exactly max/2, which is a half code (127.5 at 8-bit,
// 511.5 at 10-bit); representing it only as the rounded integer 127 would
// inject a half-code bias into every chroma sample.
func (s Spec) cNeutralNq(q uint) int64 {
	if s.Range == Full {
		return int64(s.MaxCode()) << (q - 1)
	}
	return int64(s.COff()) << q
}

// YScale maps normalized [0,1] luma onto the luma excursion.
func (s Spec) YScale() float64 {
	if s.Range == Full {
		return float64(s.MaxCode())
	}
	return float64(s.YMax() - s.YMin())
}

// CScale maps normalized [-0.5,0.5] chroma onto the chroma excursion.
func (s Spec) CScale() float64 {
	if s.Range == Full {
		return float64(s.MaxCode())
	}
	return float64(s.CMax() - s.CMin())
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampF(v, lo, hi float64) float64 {
	return math.Min(math.Max(v, lo), hi)
}
