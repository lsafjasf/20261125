package detect

import (
	"fmt"
)

// classify applies explicit, auditable thresholds. Each rule lists the
// features it used; the returned counter is the strongest competing
// explanation measured on the same samples.
func classify(f Features, nullNCC float64, sym float64) (Verdict, []string, Counter) {
	reasons := []string{}

	// Universal null gate: evidence must beat the shuffled control.
	nullGap := f.NCCMean - nullNCC
	if f.NCCMax < 0.45 || nullGap < 0.15 {
		// Ringing exception: codec pre/post-ring lives on a minority of
		// windows (the boundary transients), so the mean NCC is low, but the
		// conjunction of strong local NCC, full grid lock and block-boundary
		// onsets is specific to block artefacts.
		if f.LagSamples <= 1024 && f.GridFrac >= 0.75 && f.BlockDecay >= 0.10 &&
			f.NCCMax >= 0.75 && sym <= 0.12 {
			return VerdictRinging, []string{
				fmt.Sprintf("local NCC peaks %.3f on %.0f%% of windows (boundary transients)",
					f.NCCMax, 100*f.FracHighNCC),
				fmt.Sprintf("%d%% alignment grid lock to %d-sample blocks", int(100*f.GridFrac), f.GridBlock),
				fmt.Sprintf("block-boundary onset fraction %.2f and +/-lag symmetry %.3f",
					f.BlockDecay, sym),
			}, Counter{
				Kind:      "watermark_hypothesis",
				Detail:    "A copied watermark would show high NCC across whole fragments, not only block-boundary windows, and would not produce block-locked onset transients.",
				NCC:       offGridNCCHint(f),
				Threshold: fptr(0.75),
			}
		}
		c := Counter{
			Kind:      "shuffled_window_control",
			Detail:    "NCC does not exceed the temporally shuffled null by 0.15; repeat is not distinguishable from chance structure.",
			NCC:       nullNCC,
			Threshold: fptr(0.45),
		}
		return VerdictRejected, []string{
			fmt.Sprintf("max NCC %.3f vs null %.3f (gap %.3f)", f.NCCMax, nullNCC, nullGap),
		}, c
	}

	// Codec ringing: short, block-locked, symmetric in time, no rahmonics,
	// and accompanied by damped transients on block boundaries. The last
	// condition is what separates ringing from stationary periodic content
	// (which is also grid-locked but has flat intra-block energy).
	if f.LagSamples <= 1024 && f.GridFrac >= 0.75 && sym <= 0.12 &&
		f.Rahmonic < 0.5 && f.BlockDecay >= 0.10 {
		reasons = append(reasons,
			fmt.Sprintf("%d%% of alignment positions lock to %d-sample block grid", int(100*f.GridFrac), f.GridBlock),
			fmt.Sprintf("+/-lag NCC symmetry error %.3f (<=0.12)", sym),
			fmt.Sprintf("short lag %d samples and rahmonic score %.2f", f.LagSamples, f.Rahmonic),
			fmt.Sprintf("block-boundary onset fraction %.2f (>=0.10) indicates damped boundary transients", f.BlockDecay))
		return VerdictRinging, reasons, Counter{
			Kind:      "watermark_hypothesis",
			Detail:    "A true injected watermark would survive block-boundary shifts and break +/-lag symmetry; observed similarity vanishes off the grid.",
			NCC:       offGridNCCHint(f),
			Threshold: fptr(0.75),
		}
	}

	// Natural echo: decaying global delay chain, diffuse (non-localised)
	// support, spread across the recording and not block-locked. Rahmonic
	// harmonics are supporting evidence when the cepstrum window admits
	// them, not a hard gate (a single tap has no harmonics).
	if f.DecayT123 < 0.85 && f.OnsetLocalized < 0.5 &&
		f.SupportFraction > 0.5 && f.GridFrac < 0.75 && f.LagSamples > 1024 {
		reasons = append(reasons,
			fmt.Sprintf("delay-chain RMS decay ratio %.3f (<0.85)", f.DecayT123),
			fmt.Sprintf("evidence covers %.0f%% of the recording with only %.0f%% localised bursts (diffuse tap train)",
				100*f.SupportFraction, 100*f.OnsetLocalized),
			fmt.Sprintf("no block-grid lock (%.0f%%); rahmonic=%.2f", 100*f.GridFrac, f.Rahmonic))
		return VerdictEcho, reasons, Counter{
			Kind:      "watermark_hypothesis",
			Detail:    "A copied watermark fragment would keep near-equal gain across copies and cluster on discrete onsets instead of a diffuse decaying tap train.",
			NCC:       f.NCCMean,
			Threshold: fptr(0.95),
		}
	}

	// True repeat: localised occurrences, stable gain (low CV), no decay,
	// and no block-grid lock.
	if f.OnsetLocalized >= 0.5 && f.GainCV <= 0.35 && f.DecayT123 >= 0.85 && f.GridFrac < 0.75 {
		reasons = append(reasons,
			fmt.Sprintf("%.0f%% of evidence sits in localised fragment bursts", 100*f.OnsetLocalized),
			fmt.Sprintf("copy gain CV %.3f (<=0.35) means copies keep level", f.GainCV),
			fmt.Sprintf("no decay (ratio %.2f) and no block-grid lock (%.0f%%)", f.DecayT123, 100*f.GridFrac),
			fmt.Sprintf("NCC %.3f beats shuffled null %.3f by %.3f", f.NCCMean, nullNCC, nullGap))
		return VerdictWatermark, reasons, Counter{
			Kind:      "echo_or_ringing_control",
			Detail:    "Echo would decay monotonically and ring would lock to block boundaries; neither pattern reaches its threshold on these samples.",
			NCC:       nullNCC,
			Threshold: fptr(0.35),
		}
	}

	// Mixed/ambiguous: report honestly rather than forcing a label.
	reasons = append(reasons,
		fmt.Sprintf("discriminators conflict: localised=%.2f gainCV=%.3f decay=%.2f grid=%.2f rahmonic=%.2f",
			f.OnsetLocalized, f.GainCV, f.DecayT123, f.GridFrac, f.Rahmonic))
	return VerdictRejected, reasons, Counter{
		Kind:      "mixed_hypotheses",
		Detail:    "No single class clears its conjunction of thresholds; the interval is reported as inconclusive instead of overclaimed.",
		NCC:       f.NCCMean,
		Threshold: nil,
	}
}

func fptr(v float64) *float64 { return &v }

func offGridNCCHint(f Features) float64 {
	// Quantitative hint recorded in the counter: the fraction of evidence
	// NOT on the grid determines how much similarity would remain after a
	// block-boundary shift (the watermark test).
	return f.NCCMean * (1 - f.GridFrac)
}
