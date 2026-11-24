package main

import (
	"fmt"

	"colorconv/colorconv"
)

// Declared error budget for a full RGB->YUV->RGB round trip, in code-value
// (LSB) units of the target bit depth. It covers the two quantizations
// (encode + decode) plus fixed-point coefficient resolution:
//
//	8-bit : max 2 LSB, mean <= 0.5 LSB, |bias| <= 0.25 LSB
//	10-bit: max 6 LSB, mean <= 1.5 LSB, |bias| <= 0.75 LSB
//
// The 10-bit LSB budget is 4x the 8-bit one because a 10-bit code unit is
// 1/4 of an 8-bit unit, so both describe the same physical color difference.
func maxTolerance(bits int) int {
	if bits == 10 {
		return 6
	}
	return 2
}

func meanTolerance(bits int) float64 {
	if bits == 10 {
		return 1.5
	}
	return 0.5
}

func biasTolerance(bits int) float64 {
	if bits == 10 {
		return 0.75
	}
	return 0.25
}

// WithinTolerance is the single pass/fail predicate used everywhere.
func WithinTolerance(s colorconv.Spec, rt colorconv.Stats) bool {
	b := rt.Bias
	if b < 0 {
		b = -b
	}
	return rt.MaxAbs <= maxTolerance(s.Bits) &&
		rt.MeanAbs <= meanTolerance(s.Bits) &&
		b <= biasTolerance(s.Bits)
}

func bugSection() error {
	samples8 := colorconv.Samples(8)
	spec := colorconv.Spec{Matrix: colorconv.BT709, Range: colorconv.Limited, Bits: 8}

	other := spec
	other.Matrix = colorconv.BT601

	type buggy struct {
		name string
		enc  colorconv.Encoder
	}
	cases := []buggy{
		{"matrix mismatch 709enc/601dec",
			colorconv.MatrixMismatchEncoder{EncodeSpec: spec, DecodeSpec: other}},
		{"limited offsets dropped (full decode)",
			colorconv.OffsetlessEncoder{Spec: spec}},
		{"range confusion limited->full",
			colorconv.RangeConfusionEncoder{Spec: spec, EncodeRange: colorconv.Limited, DecodeRange: colorconv.Full}},
		{"U/V swapped on decode",
			colorconv.SwappedUVEncoder{Spec: spec}},
	}

	fmt.Println("## defect controls (BT.709 limited 8-bit, expect FAIL)")
	fmt.Printf("%-34s %8s %8s %8s  %s\n", "combination", "rtMax", "rtMean", "rtRMS", "verdict")
	for _, c := range cases {
		rt := colorconv.RoundTrip(c.enc, samples8)
		verdict := "FAIL"
		if WithinTolerance(spec, rt) {
			verdict = "PASS"
		}
		fmt.Printf("%-34s %8d %8.3f %8.3f  %s\n",
			c.name, rt.MaxAbs, rt.MeanAbs, rt.RMS, verdict)
	}
	fmt.Println()
	return nil
}
