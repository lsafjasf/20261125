// Package cbarcheck audits broadcast colour bars and grayscale ramps for
// range, matrix, and bit-depth compliance. It deliberately separates a
// legal saturated colour from an illegal chroma code.
package cbarcheck

import "fmt"

const Schema = "cbarcheck/v1"

// RangeMode describes how luma and chroma codes use the available word.
type RangeMode string

const (
	RangeLimited RangeMode = "limited"
	RangeFull    RangeMode = "full"
)

// Matrix is the luma/chroma matrix used to derive the expected samples.
type Matrix string

const (
	MatrixBT601  Matrix = "bt601"
	MatrixBT709  Matrix = "bt709"
	MatrixBT2020 Matrix = "bt2020"
)

// BitDepth is the number of meaningful bits per component.
type BitDepth int

const (
	Depth8  BitDepth = 8
	Depth10 BitDepth = 10
	Depth12 BitDepth = 12
)

// Config is one complete range/matrix/depth hypothesis.
type Config struct {
	Range  RangeMode `json:"range"`
	Matrix Matrix    `json:"matrix"`
	Depth  BitDepth  `json:"depth"`
}

func (c Config) Validate() error {
	if c.Range != RangeLimited && c.Range != RangeFull {
		return fmt.Errorf("unsupported range %q", c.Range)
	}
	if c.Matrix != MatrixBT601 && c.Matrix != MatrixBT709 && c.Matrix != MatrixBT2020 {
		return fmt.Errorf("unsupported matrix %q", c.Matrix)
	}
	if c.Depth != Depth8 && c.Depth != Depth10 && c.Depth != Depth12 {
		return fmt.Errorf("unsupported bit depth %d", c.Depth)
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("%s/%s/%d-bit", c.Range, c.Matrix, c.Depth)
}

// Configs returns every supported range/matrix/depth hypothesis in stable
// order. Callers may reorder the returned slice without changing the package.
func Configs() []Config {
	var out []Config
	for _, r := range []RangeMode{RangeLimited, RangeFull} {
		for _, m := range []Matrix{MatrixBT601, MatrixBT709, MatrixBT2020} {
			for _, d := range []BitDepth{Depth8, Depth10, Depth12} {
				out = append(out, Config{Range: r, Matrix: m, Depth: d})
			}
		}
	}
	return out
}

// RGB is a nonlinear R'G'B' triplet in the inclusive 0..1 range. This is the
// domain in which conventional broadcast colour bars are specified.
type RGB struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
}

// Sample holds component code values. For a pixel these are raw integer
// codes stored as float64 so reports can show fractional means.
type Sample struct {
	Y  float64 `json:"y"`
	Cb float64 `json:"cb"`
	Cr float64 `json:"cr"`
}

type RegionKind string

const (
	RegionColorBar RegionKind = "colorbar"
	RegionGray     RegionKind = "grayscale"
)

// Region is an axis-aligned rectangle with a known expected R'G'B' value.
// X1 and Y1 are exclusive.
type Region struct {
	Index int        `json:"index"`
	Name  string     `json:"name"`
	Kind  RegionKind `json:"kind"`
	X0    int        `json:"x0"`
	Y0    int        `json:"y0"`
	X1    int        `json:"x1"`
	Y1    int        `json:"y1"`
	RGB   RGB        `json:"rgb"`
}

func (r Region) Width() int  { return r.X1 - r.X0 }
func (r Region) Height() int { return r.Y1 - r.Y0 }

// Layout describes a known test pattern and all regions that must be covered.
type Layout struct {
	Name    string   `json:"name"`
	Width   int      `json:"width"`
	Height  int      `json:"height"`
	Regions []Region `json:"regions"`
}

// Frame is a planar YCbCr 4:4:4 frame. All three planes have Width*Height
// entries, and samples are stored as unsigned integer codes.
type Frame struct {
	Width  int
	Height int
	Depth  BitDepth
	Y      []uint16
	Cb     []uint16
	Cr     []uint16
}

func (f *Frame) Validate() error {
	if f == nil {
		return fmt.Errorf("nil frame")
	}
	if f.Width <= 0 || f.Height <= 0 {
		return fmt.Errorf("invalid frame size %dx%d", f.Width, f.Height)
	}
	if f.Depth != Depth8 && f.Depth != Depth10 && f.Depth != Depth12 {
		return fmt.Errorf("unsupported frame depth %d", f.Depth)
	}
	n := f.Width * f.Height
	if len(f.Y) != n || len(f.Cb) != n || len(f.Cr) != n {
		return fmt.Errorf("plane sizes Y=%d Cb=%d Cr=%d, want %d", len(f.Y), len(f.Cb), len(f.Cr), n)
	}
	max := (1 << f.Depth) - 1
	for i, v := range f.Y {
		if int(v) > max {
			return fmt.Errorf("Y[%d]=%d exceeds %d-bit range", i, v, f.Depth)
		}
		if int(f.Cb[i]) > max || int(f.Cr[i]) > max {
			return fmt.Errorf("chroma[%d] exceeds %d-bit range", i, f.Depth)
		}
	}
	return nil
}

// Thresholds are expressed in 8-bit code-equivalent units. A value of 2
// therefore means two 8-bit codes, eight 10-bit codes, or 32 12-bit codes.
type Thresholds struct {
	Luma   float64 `json:"lumaCode8"`
	Chroma float64 `json:"chromaCode8"`
}

func DefaultThresholds() Thresholds {
	return Thresholds{Luma: 2, Chroma: 2}
}

func (t Thresholds) Validate() error {
	if t.Luma < 0 || t.Chroma < 0 {
		return fmt.Errorf("thresholds must be non-negative")
	}
	return nil
}
