package main

import (
	"fmt"

	"icccmp/icc"
)

func printText(r *icc.Report) {
	printSummary("A", r.LeftSummary)
	printSummary("B", r.RightSummary)

	fmt.Println("== White point ==")
	w := r.WhitePoint
	fmt.Printf("  A (%s): X=%.5f Y=%.5f Z=%.5f\n", w.LeftSource, w.LeftXYZ.X, w.LeftXYZ.Y, w.LeftXYZ.Z)
	fmt.Printf("  B (%s): X=%.5f Y=%.5f Z=%.5f\n", w.RightSource, w.RightXYZ.X, w.RightXYZ.Y, w.RightXYZ.Z)
	fmt.Printf("  delta E76 = %.6f\n", w.DeltaE76)

	fmt.Println("== Matrix columns (rXYZ/gXYZ/bXYZ) ==")
	for _, m := range r.Matrix {
		if !m.LeftOK || !m.RightOK {
			fmt.Printf("  %s: unavailable (left ok=%v right ok=%v)\n", m.Channel, m.LeftOK, m.RightOK)
			continue
		}
		fmt.Printf("  %s: max |delta| = %.6f\n", m.Channel, m.MaxAbsDiff)
		fmt.Printf("      A = (%.5f, %.5f, %.5f)\n", m.LeftXYZ.X, m.LeftXYZ.Y, m.LeftXYZ.Z)
		fmt.Printf("      B = (%.5f, %.5f, %.5f)\n", m.RightXYZ.X, m.RightXYZ.Y, m.RightXYZ.Z)
	}

	fmt.Println("== Tone reproduction curves ==")
	for _, c := range r.Curves {
		fmt.Printf("  %s: A[%s] vs B[%s]\n", c.Channel, c.LeftForm, c.RightForm)
		if !c.Comparable {
			fmt.Printf("      not comparable: %s\n", c.Reason)
		} else {
			fmt.Printf("      pointwise over %d points: max=%.6f rms=%.6f mean|d|=%.6f\n",
				c.Stats.Samples, c.Stats.MaxAbs, c.Stats.RMS, c.Stats.MeanAbs)
		}
		printQuant("A", c.LeftQuant)
		printQuant("B", c.RightQuant)
	}

	printUnsupported("A", r.Unsupported.Left)
	printUnsupported("B", r.Unsupported.Right)
}

func printSummary(label string, s icc.ProfileSummary) {
	fmt.Printf("== Profile %s ==\n", label)
	fmt.Printf("  file: %s (%d bytes)\n", s.Path, s.Size)
	fmt.Printf("  version: %s  class: %s  colorSpace: %s  PCS: %s  tags: %d\n",
		s.Version, s.Class, s.ColorSpace, s.PCS, s.TagCount)
	for _, issue := range s.Issues {
		fmt.Printf("  [issue] %s\n", issue)
	}
	for sig, issues := range s.TagIssues {
		for _, issue := range issues {
			fmt.Printf("  [issue] tag %s: %s\n", sig, issue)
		}
	}
}

func printQuant(side string, q *icc.QuantStats) {
	if q == nil || !q.Present {
		return
	}
	fmt.Printf("      %s LUT quantization (%d entries): max=%.6f rms=%.6f\n",
		side, q.Entries, q.MaxAbs, q.RMS)
}

func printUnsupported(side string, tags []icc.UnsupportedTag) {
	fmt.Printf("== Unsupported tags in %s ==\n", side)
	if len(tags) == 0 {
		fmt.Println("  none")
		return
	}
	for _, t := range tags {
		typeName := t.Type
		if typeName == "" {
			typeName = "not parsed"
		}
		fmt.Printf("  %s  offset=%d size=%d type=%s [outside matrix/TRC subset]\n",
			t.Signature, t.Offset, t.Size, typeName)
	}
}
