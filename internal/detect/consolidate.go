package detect

import "sort"

// consolidate merges cross-band duplicates of the same physical phenomenon:
//
//   - lag hypotheses whose lags agree within 2% are merged, keeping the
//     member with the most discriminative feature set and recording the
//     number of supporting bands;
//   - fragment families covering overlapping sample spans are merged the
//     same way (one watermark seen through several bands is one conclusion).
//
// Rejected findings are kept (they are part of the audit trail) but never
// displace a positive verdict.
func consolidate(in []Finding) []Finding {
	var lags, fams, rejected []Finding
	for _, f := range in {
		switch {
		case f.Label == "fragment_family":
			fams = append(fams, f)
		case f.Verdict == VerdictRejected:
			rejected = append(rejected, f)
		default:
			lags = append(lags, f)
		}
	}
	out := append(mergeLags(lags), mergeFamilies(fams)...)

	// Trim the rejected list to distinct lags per band for a readable trail.
	sort.Slice(rejected, func(i, j int) bool {
		if rejected[i].Band != rejected[j].Band {
			return rejected[i].Band < rejected[j].Band
		}
		return rejected[i].Features.NCCMax > rejected[j].Features.NCCMax
	})
	out = append(out, dedupRejected(rejected)...)
	return out
}

func mergeLags(in []Finding) []Finding {
	used := make([]bool, len(in))
	var out []Finding
	for i := range in {
		if used[i] {
			continue
		}
		group := []Finding{in[i]}
		used[i] = true
		for j := i + 1; j < len(in); j++ {
			if used[j] {
				continue
			}
			if in[i].Verdict != in[j].Verdict {
				continue
			}
			related := sameLag(in[i].Features.LagSamples, in[j].Features.LagSamples)
			if in[i].Verdict == VerdictRinging &&
				in[i].Features.GridFrac >= 0.75 && in[j].Features.GridFrac >= 0.75 {
				related = true // block-locked lags across bands are one artefact
			}
			if related {
				group = append(group, in[j])
				used[j] = true
			}
		}
		out = append(out, pickStrongest(group))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Features.LagSamples < out[j].Features.LagSamples
	})
	return out
}

func mergeFamilies(in []Finding) []Finding {
	used := make([]bool, len(in))
	var out []Finding
	for i := range in {
		if used[i] {
			continue
		}
		group := []Finding{in[i]}
		used[i] = true
		for j := i + 1; j < len(in); j++ {
			if used[j] {
				continue
			}
			if spansOverlap(in[i].Range, in[j].Range) && in[i].Verdict == in[j].Verdict {
				group = append(group, in[j])
				used[j] = true
			}
		}
		out = append(out, pickStrongest(group))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Range.Start < out[j].Range.Start
	})
	return out
}

func pickStrongest(group []Finding) Finding {
	best := group[0]
	bands := map[string]bool{best.Band: true}
	for _, f := range group[1:] {
		bands[f.Band] = true
		if scoreFinding(f) > scoreFinding(best) {
			best = f
		}
	}
	if len(group) > 1 {
		best.Label = best.Label + " (supported in " + itostr(len(bands)) + " bands)"
	}
	return best
}

func scoreFinding(f Finding) float64 {
	s := f.Features.NCCMean
	// A larger mutually-consistent family is stronger evidence than a
	// high mean NCC on a handful of copies.
	s += 0.15 * float64(len(f.Occurrences))
	if f.Verdict == VerdictEcho {
		s += 1 - f.Features.DecayT123
	}
	if f.Verdict == VerdictRinging {
		s += f.Features.GridFrac
	}
	return s
}

func sameLag(a, b int) bool {
	ref := a
	if ref == 0 {
		return b == 0
	}
	d := a - b
	if d < 0 {
		d = -d
	}
	return 100*d <= 2*ref
}

func spansOverlap(a, b Range) bool {
	return a.Start < b.End && b.Start < a.End
}

func dedupRejected(in []Finding) []Finding {
	seen := map[string]bool{}
	var out []Finding
	perBand := map[string]int{}
	for _, f := range in {
		key := f.Band + ":" + itostr(f.Features.LagSamples)
		if seen[key] {
			continue
		}
		seen[key] = true
		// Keep the audit trail compact: the three most informative rejected
		// lags per band (highest NCC) are enough to document negative results.
		if perBand[f.Band] >= 3 {
			continue
		}
		perBand[f.Band]++
		out = append(out, f)
	}
	return out
}
