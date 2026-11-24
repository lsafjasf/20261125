package main

import (
	"flag"
	"fmt"
	"os"

	"colorconv/colorconv"
)

type variant struct {
	name string
	note string
	make func(s colorconv.Spec) colorconv.Encoder
}

func main() {
	strict := flag.Bool("strict", false, "exit non-zero when any row violates the error budget")
	flag.Parse()

	specs := []colorconv.Spec{
		{Matrix: colorconv.BT601, Range: colorconv.Limited, Bits: 8},
		{Matrix: colorconv.BT709, Range: colorconv.Limited, Bits: 8},
		{Matrix: colorconv.BT601, Range: colorconv.Full, Bits: 8},
		{Matrix: colorconv.BT709, Range: colorconv.Full, Bits: 8},
		{Matrix: colorconv.BT601, Range: colorconv.Limited, Bits: 10},
		{Matrix: colorconv.BT709, Range: colorconv.Limited, Bits: 10},
		{Matrix: colorconv.BT601, Range: colorconv.Full, Bits: 10},
		{Matrix: colorconv.BT709, Range: colorconv.Full, Bits: 10},
	}

	variants := []variant{
		{"float64", "reference", func(s colorconv.Spec) colorconv.Encoder { return colorconv.FloatEncoder{Spec: s} }},
		{"fixed Q16 round", "symmetric rounding", func(s colorconv.Spec) colorconv.Encoder {
			return colorconv.FixedEncoder{Spec: s, Q: 16, Round: true}
		}},
		{"fixed Q12 round", "symmetric rounding", func(s colorconv.Spec) colorconv.Encoder {
			return colorconv.FixedEncoder{Spec: s, Q: 12, Round: true}
		}},
		{"integer Q8 round", "correction terms", func(s colorconv.Spec) colorconv.Encoder {
			return colorconv.NewRoundedInteger(s)
		}},
		{"integer Q8 floor", "naive truncation", func(s colorconv.Spec) colorconv.Encoder {
			return colorconv.NewNaiveInteger(s)
		}},
	}

	exit := 0
	for _, bits := range []int{8, 10} {
		samples := colorconv.Samples(bits)
		saturated := colorconv.Saturated(bits)
		fmt.Printf("## %d-bit corpus: %d pixels (%d saturated)\n\n",
			bits, len(samples), len(saturated))
		fmt.Printf("%-26s %-18s %8s %8s %8s %8s | %8s %8s\n",
			"variant", "spec", "rtMax", "rtMean", "rtRMS", "bias", "fwdMax", "fwdMean")
		for _, sp := range specs {
			if sp.Bits != bits {
				continue
			}
			ref := colorconv.FloatEncoder{Spec: sp}
			for _, v := range variants {
				enc := v.make(sp)
				rt := colorconv.RoundTrip(enc, samples)
				fd := colorconv.ForwardDrift(enc, ref, samples)
				mark := ""
				if !WithinTolerance(sp, rt) {
					mark = "  FAIL"
					if *strict {
						exit = 1
					}
				}
				fmt.Printf("%-26s %-18s %8d %8.3f %8.3f %8.3f | %8d %8.3f%s\n",
					v.name, label(sp), rt.MaxAbs, rt.MeanAbs, rt.RMS,
					rt.Bias, fd.MaxAbs, fd.MeanAbs, mark)
			}
		}

		fmt.Printf("\nsaturated pure colors only (round-trip RGB error):\n")
		fmt.Printf("%-26s %-18s %8s %8s %8s %8s\n",
			"variant", "spec", "rtMax", "rtMean", "rtRMS", "bias")
		for _, sp := range specs {
			if sp.Bits != bits {
				continue
			}
			for _, v := range variants {
				rt := colorconv.RoundTrip(v.make(sp), saturated)
				fmt.Printf("%-26s %-18s %8d %8.3f %8.3f %8.3f\n",
					v.name, label(sp), rt.MaxAbs, rt.MeanAbs, rt.RMS, rt.Bias)
			}
		}
		fmt.Println()
	}

	if err := bugSection(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(exit)
}

func label(s colorconv.Spec) string {
	return fmt.Sprintf("%s/%s/%db", s.Matrix, s.Range, s.Bits)
}
