package mixloud

import (
	"errors"
	"math"
	"sort"
)

type windowCalc struct {
	window Window
	energy float64
}

func (o Options) normalized() Options {
	d := DefaultOptions()
	if o.WindowMS <= 0 {
		o.WindowMS = d.WindowMS
	}
	if o.HopMS <= 0 {
		o.HopMS = d.HopMS
	}
	if o.HopMS > o.WindowMS {
		o.HopMS = o.WindowMS
	}
	if o.SilenceDBFS == 0 {
		o.SilenceDBFS = d.SilenceDBFS
	}
	if o.MinSilenceSec <= 0 {
		o.MinSilenceSec = d.MinSilenceSec
	}
	if o.DialogueThreshold <= 0 {
		o.DialogueThreshold = d.DialogueThreshold
	}
	if o.AdJumpLU <= 0 {
		o.AdJumpLU = d.AdJumpLU
	}
	if o.AdMinSec <= 0 {
		o.AdMinSec = d.AdMinSec
	}
	if o.PhaseThreshold == 0 {
		o.PhaseThreshold = d.PhaseThreshold
	}
	if o.PeakLimitDBFS == 0 {
		o.PeakLimitDBFS = d.PeakLimitDBFS
	}
	if o.DialogueDuckDB == 0 {
		o.DialogueDuckDB = d.DialogueDuckDB
	}
	return o
}

func AnalyzeScheme(a *Audio, scheme, label string, opts Options, dialogueStem *Audio) (*SchemeReport, error) {
	if a == nil || a.Frames() == 0 {
		return nil, errors.New("audio is empty")
	}
	opts = opts.normalized()
	if dialogueStem != nil && (dialogueStem.Frames() != a.Frames() || dialogueStem.SampleRate != a.SampleRate) {
		return nil, errors.New("dialogue stem must have matching sample rate and frame count")
	}

	filtered := applyKWeight(a)
	calcs := buildWindows(a, filtered, opts, dialogueStem)
	windows := make([]Window, len(calcs))
	for i, item := range calcs {
		windows[i] = item.window
	}

	silence, ads, phase, bursts := DetectSpecial(a, calcs, opts)
	if silence == nil {
		silence = []Event{}
	}
	if ads == nil {
		ads = []Event{}
	}
	if phase == nil {
		phase = []Event{}
	}
	if bursts == nil {
		bursts = []Event{}
	}
	metrics := summarize(a, calcs, silence, scheme, label, opts, dialogueStem != nil)

	report := &SchemeReport{
		Metrics:         metrics,
		SilenceSegments: silence,
		AdIntrusions:    ads,
		PhaseInversions: phase,
		BurstEvents:     bursts,
	}
	if opts.IncludeWindows {
		report.Windows = windows
	}
	return report, nil
}

func buildWindows(a *Audio, filtered [][]float64, opts Options, dialogueStem *Audio) []windowCalc {
	fs := a.SampleRate
	n := a.Frames()
	winFrames := int(math.Round(float64(fs) * float64(opts.WindowMS) / 1000))
	hopFrames := int(math.Round(float64(fs) * float64(opts.HopMS) / 1000))
	if winFrames <= 0 || winFrames > n {
		winFrames = n
	}
	if hopFrames <= 0 {
		hopFrames = winFrames
	}

	type span struct{ start, end int }
	var spans []span
	for start := 0; start+winFrames <= n; start += hopFrames {
		spans = append(spans, span{start, start + winFrames})
	}
	if n >= winFrames {
		last := span{n - winFrames, n}
		if len(spans) == 0 || spans[len(spans)-1].start != last.start {
			spans = append(spans, last)
		}
	} else if len(spans) == 0 {
		spans = append(spans, span{0, n})
	}

	weights := channelWeights(a.NumChannels())
	calcs := make([]windowCalc, 0, len(spans))
	for idx, sp := range spans {
		w := Window{
			Index:    idx,
			StartSec: float64(sp.start) / float64(fs),
			EndSec:   float64(sp.end) / float64(fs),
		}
		length := sp.end - sp.start
		var energy, rawEnergy, peak float64
		var clipped, nearClip int64
		for ch := 0; ch < a.NumChannels(); ch++ {
			var weightedSum float64
			for i := sp.start; i < sp.end; i++ {
				y := filtered[ch][i]
				weightedSum += y * y
				x := a.Channels[ch][i]
				ax := math.Abs(x)
				rawEnergy += x * x
				if ax > peak {
					peak = ax
				}
				if ax >= 0.999 {
					clipped++
				}
				if ax >= 0.99 {
					nearClip++
				}
			}
			energy += weights[ch] * weightedSum / float64(length)
		}

		frameSamples := float64(length * a.NumChannels())
		rms := math.Sqrt(rawEnergy / frameSamples)
		w.RMSDBFS = floatPtr(safeDB(rms))
		w.PeakDBFS = floatPtr(safeDB(peak))
		w.ClippedSamples = clipped
		w.NearClipSamples = nearClip
		if energy > 1e-12 {
			w.LoudnessLUFS = floatPtr(-0.691 + 10*math.Log10(energy))
		}
		if a.NumChannels() == 2 {
			corr, monoDB, center := stereoFeatures(a, sp.start, sp.end)
			w.Correlation = floatPtr(corr)
			w.MonoCompatibilityDB = floatPtr(monoDB)
			w.CenterShare = floatPtr(center)
			w.PhaseInverted = corr <= opts.PhaseThreshold && monoDB <= -6
		}
		if dialogueStem != nil {
			w.DialogueScore = stemDialogueScore(dialogueStem, sp.start, sp.end)
			w.Dialogue = w.DialogueScore >= opts.DialogueThreshold
		} else {
			score, modulation, center := heuristicDialogueScore(a, sp.start, sp.end, w.CenterShare)
			w.DialogueScore = score
			w.Dialogue = score >= opts.DialogueThreshold && modulation >= 0.25
			if a.NumChannels() == 2 {
				w.Dialogue = w.Dialogue && center >= 0.25
			}
		}
		w.Silence = w.RMSDBFS == nil || *w.RMSDBFS < opts.SilenceDBFS
		calcs = append(calcs, windowCalc{window: w, energy: energy})
	}
	return calcs
}

func channelWeights(channels int) []float64 {
	w := make([]float64, channels)
	for ch := range w {
		w[ch] = 1
		if (channels == 6 || channels == 8) && ch == 3 {
			w[ch] = 0 // LFE is excluded by BS.1770.
		}
		if channels == 8 && (ch == 4 || ch == 5) {
			w[ch] = 1.4142135623730951
		}
	}
	return w
}

func stereoFeatures(a *Audio, start, end int) (correlation, monoCompatDB, centerShare float64) {
	var sumL2, sumR2, sumLR, mid2, side2 float64
	for i := start; i < end; i++ {
		l, r := a.Channels[0][i], a.Channels[1][i]
		sumL2 += l * l
		sumR2 += r * r
		sumLR += l * r
		mid := (l + r) / 2
		side := (l - r) / 2
		mid2 += mid * mid
		side2 += side * side
	}
	correlation = sumLR / math.Sqrt(sumL2*sumR2)
	if sumL2 == 0 || sumR2 == 0 || math.IsNaN(correlation) {
		correlation = 0
	}
	stereoEnergy := (sumL2 + sumR2) / 2
	monoEnergy := (sumL2 + sumR2 + 2*sumLR) / 4
	monoCompatDB = 10 * math.Log10(monoEnergy/stereoEnergy)
	if monoEnergy <= 1e-20 {
		monoCompatDB = math.Inf(-1)
	}
	total := mid2 + side2
	if total > 1e-20 {
		centerShare = mid2 / total
	}
	return correlation, monoCompatDB, centerShare
}

func integratedLoudness(calcs []windowCalc) *float64 {
	var ungatedEnergy float64
	var ungatedCount int
	for _, c := range calcs {
		if c.window.LoudnessLUFS != nil && *c.window.LoudnessLUFS > -70 {
			ungatedEnergy += c.energy
			ungatedCount++
		}
	}
	if ungatedCount == 0 {
		return nil
	}
	ungated := -0.691 + 10*math.Log10(ungatedEnergy/float64(ungatedCount))
	relativeGate := ungated - 10
	var gatedEnergy float64
	var gatedCount int
	for _, c := range calcs {
		if c.window.LoudnessLUFS != nil && *c.window.LoudnessLUFS > -70 && *c.window.LoudnessLUFS >= relativeGate {
			gatedEnergy += c.energy
			gatedCount++
		}
	}
	if gatedCount == 0 {
		return nil
	}
	return floatPtr(-0.691 + 10*math.Log10(gatedEnergy/float64(gatedCount)))
}

func loudnessRange400(calcs []windowCalc, integrated *float64) (lra, p10, p95 *float64) {
	if integrated == nil {
		return nil, nil, nil
	}
	gate := *integrated - 20
	var values []float64
	for _, c := range calcs {
		if c.window.LoudnessLUFS != nil && *c.window.LoudnessLUFS > -70 && *c.window.LoudnessLUFS >= gate {
			values = append(values, *c.window.LoudnessLUFS)
		}
	}
	if len(values) < 2 {
		return nil, nil, nil
	}
	sort.Float64s(values)
	lo := percentile(values, 10)
	hi := percentile(values, 95)
	return floatPtr(hi - lo), floatPtr(lo), floatPtr(hi)
}
