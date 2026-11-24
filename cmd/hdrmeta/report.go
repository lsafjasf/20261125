package main

import (
	"fmt"
	"io"
	"strings"

	"hdrmeta/pkg/analyze"
	"hdrmeta/pkg/pq"
)

func writeReport(w io.Writer, seq analyze.SequenceStats, paths []string, cfg analyze.Config) {
	depth := seq.BitDepth
	fmt.Fprintln(w, "================ HDR static metadata review report ================")
	fmt.Fprintf(w, "Frames analyzed : %d (bit depth %d)\n", seq.FrameCount, depth)
	fmt.Fprintf(w, "Highlight gate  : %.0f nits | qualified area >= %.2f%% of frame\n",
		cfg.HighlightNits, cfg.MinAreaRatio*100)
	fmt.Fprintf(w, "Clip point      : %.0f nits = code %d (of %d)\n",
		seq.ClipPointNits, seq.ClipPointCode, pq.MaxCode(depth))

	fmt.Fprintln(w, "\n-- Per-frame luminance (nits) --")
	fmt.Fprintf(w, "%-28s %10s %10s %10s %10s %12s\n",
		"frame", "max", "mean", "p99.9", "p99.99", "clip%")
	for _, st := range seq.Frames {
		fmt.Fprintf(w, "%-28s %10.2f %10.2f %10.2f %10.2f %11.3f%%\n",
			baseName(st.Name), st.MaxNits, st.MeanNits,
			st.P99_9Nits, st.P99_99Nits, st.ClipRatio*100)
	}

	fmt.Fprintln(w, "\n-- Highlight regions (above gate) --")
	for _, st := range seq.Frames {
		if len(st.Regions) == 0 {
			fmt.Fprintf(w, "%-28s none\n", baseName(st.Name))
			continue
		}
		for _, r := range st.Regions {
			tag := "qualified"
			if !r.Qualified {
				tag = "speckle/subtitle"
			}
			fmt.Fprintf(w,
				"%-28s #%d size=%6d (%6.3f%%) peak=%8.2f nits (code %4d) mean=%8.2f box=[%d,%d-%d,%d] %s\n",
				baseName(st.Name), r.ID, r.Size, r.AreaRatio*100,
				r.PeakNits, r.PeakCode, r.MeanNits,
				r.MinX, r.MinY, r.MaxX, r.MaxY, tag)
		}
	}

	cll := analyze.Recommendation(seq.MaxCLLNits)
	fall := analyze.Recommendation(seq.MaxFALLNits)
	robust := analyze.Recommendation(seq.RobustCLL)
	region := analyze.Recommendation(seq.RegionCLL)

	fmt.Fprintln(w, "\n-- Static metadata candidates --")
	fmt.Fprintf(w, "strict MaxCLL (max of frame maxima)        : %8.2f nits -> report %d nits (code %d)\n",
		seq.MaxCLLNits, cll, pq.NitsToCode(float64(cll), depth))
	fmt.Fprintf(w, "strict MaxFALL (max of frame averages)     : %8.2f nits -> report %d nits (code %d)\n",
		seq.MaxFALLNits, fall, pq.NitsToCode(float64(fall), depth))
	fmt.Fprintf(w, "robust MaxCLL (p99.9, speckle resistant)   : %8.2f nits -> report %d nits (code %d)\n",
		seq.RobustCLL, robust, pq.NitsToCode(float64(robust), depth))
	fmt.Fprintf(w, "area-qualified region MaxCLL (>= %d px)    : %8.2f nits -> report %d nits (code %d)\n",
		seq.QualifiedArea, seq.RegionCLL, region, pq.NitsToCode(float64(region), depth))

	fmt.Fprintln(w, "\n-- Clip relationship --")
	fmt.Fprintf(w, "pixels above %.0f nits (hard-clipped by a %.0f-nit display): %d total\n",
		seq.ClipPointNits, seq.ClipPointNits, seq.TotalClip)
	fmt.Fprintf(w, "code %d is the first code whose nits exceed the clip point; ",
		seq.ClipPointCode+1)
	fmt.Fprintf(w, "every higher code is clipped to %.0f nits on that display.\n", seq.ClipPointNits)

	fmt.Fprintln(w, "\nInputs:")
	for _, p := range paths {
		fmt.Fprintln(w, "  "+p)
	}
}

func baseName(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
