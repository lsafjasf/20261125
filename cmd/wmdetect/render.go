package main

import (
	"fmt"
	"strings"

	"watermarkaudit/internal/detect"
)

func renderText(r *detect.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "watermark audit: %s\n", r.File)
	fmt.Fprintf(&b, "samples=%d rate=%d duration=%.3fs\n", r.TotalSamples, r.SampleRate, r.DurationSec)
	fmt.Fprintf(&b, "preprocessing: %s\n", r.Preprocessing)
	fmt.Fprintf(&b, "global shuffled-null NCC baseline: %.4f\n\n", r.NullBaseline)
	fmt.Fprintf(&b, "summary: watermark=%d echo=%d ringing=%d rejected=%d\n\n",
		r.Summary[detect.VerdictWatermark], r.Summary[detect.VerdictEcho],
		r.Summary[detect.VerdictRinging], r.Summary[detect.VerdictRejected])
	if len(r.Findings) == 0 {
		b.WriteString("no repeat hypotheses survived the null gate; no watermark evidence.\n")
		return b.String()
	}
	for i, f := range r.Findings {
		fmt.Fprintf(&b, "[%d] %s  band=%s source=%s\n", i+1, f.Verdict, nz(f.Band), f.Label)
		fmt.Fprintf(&b, "    sample range: [%d, %d)  fragment_len=%d  lag=%d (%.4fs)\n",
			f.Range.Start, f.Range.End, f.FragmentLen, f.Features.LagSamples, f.Features.LagSeconds)
		fmt.Fprintf(&b, "    NCC mean=%.4f max=%.4f high-frac=%.2f | 95%% CI [%.4f, %.4f]\n",
			f.Features.NCCMean, f.Features.NCCMax, f.Features.FracHighNCC, f.Confidence.Low, f.Confidence.High)
		fmt.Fprintf(&b, "    gain mean=%.4f cv=%.4f | gain 95%% CI [%.4f, %.4f]\n",
			f.Features.GainMean, f.Features.GainCV, f.GainCI.Low, f.GainCI.High)
		fmt.Fprintf(&b, "    decay=%.3f rahmonic=%.3f grid=%.2f(block=%d) blockdecay=%.2f localised=%.2f null=%.4f\n",
			f.Features.DecayT123, f.Features.Rahmonic, f.Features.GridFrac, f.Features.GridBlock,
			f.Features.BlockDecay, f.Features.OnsetLocalized, f.Features.NullNCCMean)
		for _, o := range f.Occurrences {
			fmt.Fprintf(&b, "    occurrence [%d,%d) ncc=%.3f gain=%.3f jitter=%d\n",
				o.Range.Start, o.Range.End, o.NCC, o.GainVsAnchor, o.OffsetJitter)
		}
		b.WriteString("    reasons:\n")
		for _, rs := range f.Reasons {
			fmt.Fprintf(&b, "      - %s\n", rs)
		}
		fmt.Fprintf(&b, "    counterexample (%s) range=[%d,%d) counter_ncc=%.4f threshold=%v\n",
			f.Counterexample.Kind, f.Counterexample.SampleRange.Start, f.Counterexample.SampleRange.End,
			f.Counterexample.NCC, ftxp(f.Counterexample.Threshold))
		fmt.Fprintf(&b, "      detail: %s\n\n", f.Counterexample.Detail)
	}
	return b.String()
}

func nz(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func ftxp(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.3f", *v)
}
