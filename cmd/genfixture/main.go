// Command genfixture writes synthetic ICC profiles used by the test suite
// into testdata/. Pure standard library.
package main

import (
	"math"
	"os"
	"path/filepath"

	"icccmp/icc/fixture"
)

func main() {
	dir := filepath.Join("..", "testdata")
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}

	d50 := [3]float64{0.9642, 1.0, 0.8249}

	// v2 profile: s15Fixed16 matrix, curv gamma and one LUT channel.
	v2 := fixture.New(2, 4).
		AddXYZ("wtpt", d50[0], d50[1], d50[2]).
		AddXYZ("rXYZ", 0.43607, 0.22249, 0.01392).
		AddXYZ("gXYZ", 0.38515, 0.71687, 0.09708).
		AddXYZ("bXYZ", 0.14307, 0.06061, 0.71410).
		AddGamma("rTRC", 2.2).
		AddGamma("gTRC", 2.2).
		AddLUT("bTRC", gammaLUT(2.2, 256))
	write(filepath.Join(dir, "v2_ok.icc"), v2.Build())

	// v4 profile: same primaries/white point, parametric sRGB-ish curves.
	v4 := fixture.New(4, 3).
		AddXYZ("wtpt", d50[0], d50[1], d50[2]).
		AddXYZ("rXYZ", 0.43607, 0.22249, 0.01392).
		AddXYZ("gXYZ", 0.38515, 0.71687, 0.09708).
		AddXYZ("bXYZ", 0.14307, 0.06061, 0.71410).
		AddPara("rTRC", 3, sRGBPara()).
		AddPara("gTRC", 3, sRGBPara()).
		AddPara("bTRC", 3, sRGBPara()).
		AddRaw("chad", chadTag())
	write(filepath.Join(dir, "v4_ok.icc"), v4.Build())

	// Duplicate tag: rTRC appears twice in the tag table.
	dup := fixture.New(2, 4).
		AddXYZ("wtpt", d50[0], d50[1], d50[2]).
		AddXYZ("rXYZ", 0.43, 0.22, 0.01).
		AddXYZ("gXYZ", 0.38, 0.71, 0.09).
		AddXYZ("bXYZ", 0.14, 0.06, 0.71).
		AddGamma("rTRC", 2.2).
		AddGamma("gTRC", 2.2).
		AddGamma("bTRC", 2.2).
		Duplicate("rTRC")
	write(filepath.Join(dir, "v2_dup.icc"), dup.Build())

	// Wrong length: XYZ tag declares 18 bytes instead of 20.
	badLen := fixture.New(4, 3).
		AddXYZ("wtpt", d50[0], d50[1], d50[2]).
		AddXYZ("rXYZ", 0.43, 0.22, 0.01).
		AddXYZ("gXYZ", 0.38, 0.71, 0.09).
		AddXYZ("bXYZ", 0.14, 0.06, 0.71).
		AddGamma("rTRC", 2.2).
		AddGamma("gTRC", 2.2).
		AddGamma("bTRC", 2.2).
		SetWrongLength("rXYZ", 18)
	write(filepath.Join(dir, "v4_badlen.icc"), badLen.Build())

	// Illegal curve: non-monotonic LUT (a later entry is smaller).
	lut := gammaLUT(2.2, 16)
	lut[10], lut[9] = lut[9], lut[10]
	illegal := fixture.New(2, 4).
		AddXYZ("wtpt", d50[0], d50[1], d50[2]).
		AddXYZ("rXYZ", 0.43, 0.22, 0.01).
		AddXYZ("gXYZ", 0.38, 0.71, 0.09).
		AddXYZ("bXYZ", 0.14, 0.06, 0.71).
		AddLUT("rTRC", lut).
		AddGamma("gTRC", 2.2).
		AddGamma("bTRC", 2.2)
	write(filepath.Join(dir, "v2_illegal.icc"), illegal.Build())

	// Out-of-range tag and unsupported tags in both versions.
	oor := fixture.New(4, 3).
		AddXYZ("wtpt", d50[0], d50[1], d50[2]).
		AddXYZ("rXYZ", 0.43, 0.22, 0.01).
		AddXYZ("gXYZ", 0.38, 0.71, 0.09).
		AddXYZ("bXYZ", 0.14, 0.06, 0.71).
		AddGamma("rTRC", 2.2).
		AddGamma("gTRC", 2.2).
		AddGamma("bTRC", 2.2).
		AddRaw("cprt", textTag("synthetic")).
		AddRaw("dmnd", descTag("display")).
		SetOffset("cprt", 1<<20)
	write(filepath.Join(dir, "v4_oor.icc"), oor.Build())
}

func gammaLUT(gamma float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		x := float64(i) / float64(n-1)
		out[i] = math.Pow(x, gamma)
	}
	return out
}

// sRGBPara returns g,a,b,c,d for parametricCurveType function type 3.
func sRGBPara() []float64 {
	return []float64{2.4, 1.0 / 1.055, 0.055 / 1.055, 1.0 / 12.92, 0.04045}
}

func textTag(s string) []byte {
	p := make([]byte, 8+len(s)+1)
	copy(p[0:4], "text")
	copy(p[8:], s)
	return p
}

func descTag(s string) []byte {
	// textDescriptionType (v2): 4 type + 4 reserved + 4 count + ASCII + NUL.
	p := make([]byte, 12+len(s)+1)
	copy(p[0:4], "desc")
	putU32(p[8:], uint32(len(s)+1))
	copy(p[12:], s)
	return p
}

func chadTag() []byte {
	// chromaticAdaptation tag: 9 s15Fixed16 values, identity matrix.
	p := make([]byte, 8+9*4)
	copy(p[0:4], "sf32")
	id := []float64{1, 0, 0, 0, 1, 0, 0, 0, 1}
	for i, v := range id {
		putS15(p[8+4*i:], v)
	}
	return p
}

func putU32(p []byte, v uint32) {
	p[0] = byte(v >> 24)
	p[1] = byte(v >> 16)
	p[2] = byte(v >> 8)
	p[3] = byte(v)
}

func putS15(p []byte, v float64) {
	putU32(p, uint32(int32(v*65536.0+0.5)))
}

func write(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(err)
	}
}
