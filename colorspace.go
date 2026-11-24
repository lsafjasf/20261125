package main

import (
	"fmt"
	"math"
	"strings"
)

// RGB is a normalized (usually 0..1) linear or encoded pixel triplet.
type RGB [3]float64

type transferFunc int

const (
	transferLinear transferFunc = iota
	transferSRGB
	transferBT709
)

// Space describes a named RGB color space usable on a graph socket.
type Space struct {
	ID       string
	Label    string
	Transfer transferFunc
	toXYZ    mat3
	fromXYZ  mat3
}

var (
	matrix709 = primariesMatrix(0.64, 0.33, 0.30, 0.60, 0.15, 0.06, 0.3127, 0.3290)
	matrixP3  = primariesMatrix(0.680, 0.320, 0.265, 0.690, 0.150, 0.060, 0.3127, 0.3290)

	spaces  = map[string]*Space{}
	aliases = map[string]string{}
)

func registerSpace(id, label string, tr transferFunc, toXYZ mat3, alias ...string) {
	fromXYZ, ok := toXYZ.inverse()
	if !ok {
		panic("colorspace matrix not invertible: " + id)
	}
	s := &Space{ID: id, Label: label, Transfer: tr, toXYZ: toXYZ, fromXYZ: fromXYZ}
	spaces[id] = s
	aliases[canonicalSpaceName(id)] = id
	aliases[canonicalSpaceName(label)] = id
	for _, a := range alias {
		aliases[canonicalSpaceName(a)] = id
	}
}

func init() {
	registerSpace("srgb", "sRGB", transferSRGB, matrix709,
		"srgb-encoded", "iec61966-2-1")
	registerSpace("linear-srgb", "Linear sRGB", transferLinear, matrix709,
		"linear", "lin-srgb", "scene-linear", "linear rgb", "linearRGB")
	registerSpace("bt709", "BT.709", transferBT709, matrix709,
		"rec709", "rec.709", "bt.709", "itu709")
	registerSpace("linear-bt709", "Linear BT.709", transferLinear, matrix709,
		"linear-rec709", "linear709")
	registerSpace("p3", "Display P3", transferSRGB, matrixP3,
		"display-p3", "dci-p3-d65", "p3d65")
	registerSpace("linear-p3", "Linear Display P3", transferLinear, matrixP3,
		"linear-display-p3", "linear-p3")
}

// canonicalSpaceName folds case and separators so "BT.709", "bt709" and
// "BT 709" resolve to the same identifier.
func canonicalSpaceName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ResolveSpace looks a color space up by label or alias.
func ResolveSpace(name string) (*Space, error) {
	key := canonicalSpaceName(name)
	if id, ok := aliases[key]; ok {
		return spaces[id], nil
	}
	return nil, fmt.Errorf("未知色彩空间 %q（支持 sRGB / Linear sRGB / BT.709 / Linear BT.709 / Display P3）", name)
}

// SpaceLabel returns the canonical display label, falling back to the raw name.
func SpaceLabel(name string) string {
	if s, err := ResolveSpace(name); err == nil {
		return s.Label
	}
	return name
}

func srgbDecode(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func srgbEncode(v float64) float64 {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

func bt709Decode(v float64) float64 {
	if v < 0.081 {
		return v / 4.5
	}
	return math.Pow((v+0.099)/1.099, 1/0.45)
}

func bt709Encode(v float64) float64 {
	if v < 0.018 {
		return 4.5 * v
	}
	return 1.099*math.Pow(v, 0.45) - 0.099
}

// toLinearRGB removes the transfer curve, leaving linear-light values.
func (s *Space) toLinearRGB(c RGB) RGB {
	switch s.Transfer {
	case transferSRGB:
		return RGB{srgbDecode(c[0]), srgbDecode(c[1]), srgbDecode(c[2])}
	case transferBT709:
		return RGB{bt709Decode(c[0]), bt709Decode(c[1]), bt709Decode(c[2])}
	default:
		return c
	}
}

func (s *Space) fromLinearRGB(c RGB) RGB {
	switch s.Transfer {
	case transferSRGB:
		return RGB{srgbEncode(c[0]), srgbEncode(c[1]), srgbEncode(c[2])}
	case transferBT709:
		return RGB{bt709Encode(c[0]), bt709Encode(c[1]), bt709Encode(c[2])}
	default:
		return c
	}
}

// ConvertRGB converts one triplet between two named spaces, going through
// linear light and XYZ primaries.
func ConvertRGB(fromName, toName string, c RGB) (RGB, error) {
	src, err := ResolveSpace(fromName)
	if err != nil {
		return c, err
	}
	dst, err := ResolveSpace(toName)
	if err != nil {
		return c, err
	}
	linear := src.toLinearRGB(c)
	xyz := src.toXYZ.mul([3]float64(linear))
	dstLinear := dst.fromXYZ.mul(xyz)
	return dst.fromLinearRGB(RGB(dstLinear)), nil
}

// ConvertFrame re-tags and transforms every pixel of a frame into dst space.
func ConvertFrame(f *Frame, dstName string) error {
	dst, err := ResolveSpace(dstName)
	if err != nil {
		return err
	}
	src, err := ResolveSpace(f.Space)
	if err != nil {
		return err
	}
	if src.ID == dst.ID {
		f.Space = dst.Label
		return nil
	}
	primaries := dst.fromXYZ.mulMat(src.toXYZ)
	for i, px := range f.Pix {
		lin := src.toLinearRGB(px)
		convertedLin := primaries.mul([3]float64(lin))
		f.Pix[i] = dst.fromLinearRGB(RGB(convertedLin))
	}
	f.Space = dst.Label
	return nil
}
