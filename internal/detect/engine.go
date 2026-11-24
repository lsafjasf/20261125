package detect

import (
	"math/rand/v2"
	"time"

	"watermarkaudit/internal/dsp"
	"watermarkaudit/internal/wavio"
)

// Analyze runs the full audit on a parsed WAV. The raw mono buffer is never
// modified; band isolation uses zero-threshold FFT masks and is strictly an
// analysis view. No denoising, spectral subtraction or smoothing is applied.
func Analyze(w *wavio.WAV, bands []dsp.Band, cfg Config) *Report {
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9e3779b97f4a7c15))
	rep := &Report{
		SampleRate:    w.SampleRate,
		TotalSamples:  len(w.Mono),
		DurationSec:   float64(len(w.Mono)) / float64(w.SampleRate),
		Preprocessing: "none: raw PCM downmix only; Hann windowing and zero-threshold FFT band masks are analysis views, no denoising/smoothing applied",
	}
	for _, b := range bands {
		rep.Bands = append(rep.Bands, BandSpec{Name: b.Name, Lo: b.Lo, Hi: b.Hi})
	}

	// Global negative control used by every verdict.
	globalNull := ShuffledNull(w.Mono, cfg.MinLag+64, cfg.AlignWindow, cfg.AlignHop, 200, rng)
	rep.NullBaseline = globalNull

	seenFamilies := map[string]bool{}
	for _, band := range bands {
		xs := dsp.Bandpass(w.Mono, w.SampleRate, band.Lo, band.Hi)

		// Path A: discrete fragment families (irregular watermark schedule).
		for _, family := range locateRepeats(xs, cfg) {
			finding := familyFinding(xs, family, band, cfg, globalNull, rng)
			key := finding.RangeKey()
			if seenFamilies[key] {
				continue
			}
			seenFamilies[key] = true
			rep.Findings = append(rep.Findings, finding)
		}

		// Path B: lag hypotheses from fused discovery (periodic watermark,
		// echo chains and codec ringing all live here).
		for _, hyp := range discoverLags(xs, cfg) {
			f, pos, rmap := extractFeatures(xs, hyp.lag, cfg)
			if len(rmap) == 0 {
				continue
			}
			sym := symmetryScore(xs, hyp.lag, cfg.AlignWindow, cfg.AlignHop)
			null := ShuffledNull(xs, hyp.lag, cfg.AlignWindow, cfg.AlignHop, 200, rng)
			verdict, reasons, counter := classify(f, null, sym)
			corrCI := MeanCIZ(rmap, cfg.BootstrapIters, rng)
			gains, _ := gainRatios(xs, pos, hyp.lag, cfg.AlignWindow)
			gainCI := MeanCI(gains, cfg.BootstrapIters, rng)
			counter.SampleRange = Range{Start: 0, End: len(xs)}
			rngRange := Range{Start: pos[0], End: pos[len(pos)-1] + cfg.AlignWindow}
			rep.Findings = append(rep.Findings, Finding{
				Verdict:        verdict,
				Label:          hyp.source,
				Band:           band.Name,
				FragmentLen:    cfg.AlignWindow,
				Range:          rngRange,
				Features:       withSeconds(f, w.SampleRate),
				Confidence:     corrCI,
				GainCI:         gainCI,
				Counterexample: counter,
				Reasons:        reasons,
			})
		}
	}
	if len(rep.Findings) == 0 {
		rep.Findings = []Finding{}
	}
	rep.Findings = consolidate(rep.Findings)
	rep.Summary = map[Verdict]int{}
	for _, f := range rep.Findings {
		rep.Summary[f.Verdict]++
	}
	return rep
}

func familyFinding(xs []float64, fam []Occurrence, band dsp.Band, cfg Config, null float64, rng *rand.Rand) Finding {
	rs := make([]float64, len(fam))
	gs := make([]float64, len(fam))
	for i, o := range fam {
		rs[i] = o.NCC
		gs[i] = o.GainVsAnchor
	}
	rng2 := rand.New(rand.NewPCG(uint64(fam[0].Range.Start), uint64(fam[len(fam)-1].Range.End)))
	_ = rng2
	meanR, _ := MeanStd(rs)
	meanG, gsd := MeanStd(gs)
	maxR := rs[0]
	for _, v := range rs {
		if v > maxR {
			maxR = v
		}
	}
	cv := 0.0
	if meanG != 0 {
		cv = gsd / meanG
	}
	f := Features{
		NCCMean:         meanR,
		NCCMax:          maxR,
		FracHighNCC:     fracAbove(rs, cfg.HighNCCThresh),
		GainMean:        meanG,
		GainCV:          cv,
		DecayT123:       familyDecay(xs, fam),
		OnsetLocalized:  1,
		SupportFraction: float64(fam[len(fam)-1].Range.End-fam[0].Range.Start) / float64(len(xs)),
		NullNCCMean:     null,
	}
	verdict, reasons, counter := classify(f, null, 1.0)
	counter.SampleRange = Range{Start: fam[0].Range.Start, End: fam[len(fam)-1].Range.End}
	for i := range fam {
		fam[i].Band = band.Name
	}
	return Finding{
		Verdict:        verdict,
		Label:          "fragment_family",
		Band:           band.Name,
		FragmentLen:    fam[0].Range.End - fam[0].Range.Start,
		Range:          Range{Start: fam[0].Range.Start, End: fam[len(fam)-1].Range.End},
		Occurrences:    fam,
		Features:       f,
		Confidence:     MeanCIZ(rs, cfg.BootstrapIters, rng),
		GainCI:         MeanCI(gs, cfg.BootstrapIters, rng),
		Counterexample: counter,
		Reasons:        reasons,
	}
}

func familyDecay(xs []float64, fam []Occurrence) float64 {
	if len(fam) < 2 {
		return 1
	}
	first := dsp.RMS(xs, fam[0].Range.Start, fam[0].Range.End)
	sum, n := 0.0, 0
	for _, o := range fam[1:] {
		v := dsp.RMS(xs, o.Range.Start, o.Range.End)
		if first > 1e-9 {
			sum += v / first
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return sum / float64(n)
}

func fracAbove(x []float64, t float64) float64 {
	c := 0
	for _, v := range x {
		if v >= t {
			c++
		}
	}
	return float64(c) / float64(len(x))
}

func withSeconds(f Features, sr int) Features {
	f.LagSeconds = float64(f.LagSamples) / float64(sr)
	return f
}
