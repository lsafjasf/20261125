package lut3d

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestLookupHandCalculatedNonUniform(t *testing.T) {
	lut, err := ParseFile("testdata/valid_nonuniform.cube")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Input (0.3, 0.7, 1.3) maps to (u,v,w) = (0.25, 0.5, 0.75).
	//
	// Corner weights:
	//   000 0.09375   100 0.03125   010 0.09375   110 0.03125
	//   001 0.28125   101 0.09375   011 0.28125   111 0.09375
	//
	// R = sum(weight * corner R) = 19.09375
	// G = sum(weight * corner G) = 4.25
	// B = sum(weight * corner B) = 4.5
	got := lut.Sample(0.3, 0.7, 1.3)
	want := RGB{R: 19.09375, G: 4.25, B: 4.5}
	assertRGB(t, got, want, 1e-12)
}

func TestLookupIncomeUnitIdentity(t *testing.T) {
	lut, err := ParseFile("testdata/valid_unit.cube")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	tests := []RGB{
		{R: 0.25, G: 0.5, B: 0.75},
		{R: 0, G: 0, B: 0},
		{R: 1, G: 1, B: 1},
		{R: -2, G: 0.4, B: 3},
	}
	wants := []RGB{
		{R: 0.25, G: 0.5, B: 0.75},
		{R: 0, G: 0, B: 0},
		{R: 1, G: 1, B: 1},
		{R: 0, G: 0.4, B: 1},
	}
	for i, in := range tests {
		assertRGB(t, lut.Lookup(in), wants[i], 1e-12)
	}
}

func TestLookupExactGridNodesUseEntryOrder(t *testing.T) {
	lut, err := ParseFile("testdata/valid_nonuniform.cube")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	for i, entry := range lut.Entries {
		r := i % lut.Size
		g := (i / lut.Size) % lut.Size
		b := i / (lut.Size * lut.Size)
		in := RGB{
			R: lut.Domain[0].Min + float64(r)*lut.Step[0],
			G: lut.Domain[1].Min + float64(g)*lut.Step[1],
			B: lut.Domain[2].Min + float64(b)*lut.Step[2],
		}
		assertRGB(t, lut.Sample(in.R, in.G, in.B), entry, 1e-12)
	}
}

func TestLookupThreeByThree(t *testing.T) {
	const size = 3
	var b strings.Builder
	fmt.Fprintf(&b, "UNIT\nLUT_3D_SIZE %d\n", size)
	for blue := 0; blue < size; blue++ {
		for green := 0; green < size; green++ {
			for red := 0; red < size; red++ {
				fmt.Fprintf(&b, "%g %g %g\n",
					float64(red)/float64(size-1),
					float64(green)/float64(size-1),
					float64(blue)/float64(size-1),
				)
			}
		}
	}
	lut, err := Parse(strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	in := RGB{R: 0.2, G: 0.4, B: 0.8}
	assertRGB(t, lut.Lookup(in), in, 1e-12)
}

func TestLookupNaNDoesNotPanic(t *testing.T) {
	lut, err := ParseFile("testdata/valid_unit.cube")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	got := lut.Sample(math.NaN(), 0.5, 0.5)
	if !math.IsNaN(got.R) || !math.IsNaN(got.G) || !math.IsNaN(got.B) {
		t.Fatalf("Sample(NaN) = %#v, want all NaN", got)
	}
}

func assertRGB(t *testing.T, got, want RGB, tolerance float64) {
	t.Helper()
	if math.Abs(got.R-want.R) > tolerance ||
		math.Abs(got.G-want.G) > tolerance ||
		math.Abs(got.B-want.B) > tolerance {
		t.Fatalf("RGB = (%.12g, %.12g, %.12g), want (%.12g, %.12g, %.12g)",
			got.R, got.G, got.B, want.R, want.G, want.B)
	}
}
