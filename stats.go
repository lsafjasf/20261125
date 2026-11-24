package mixloud

import (
	"math"
	"sort"
)

func summarize(a *Audio, calcs []windowCalc, silence []Event, scheme, label string, opts Options, hasStem bool) Metrics {
	integrated := integratedLoudness(calcs)
	lra, p10, p95 := loudnessRange400(calcs, integrated)
	lra3s := loudnessRangeLong(a, 3, 1, integrated)

	var loudness []float64
	var maxLoudness float64 = math.Inf(-1)
	var maxPeak float64
	var clipped, nearClip int64
	for ch := 0; ch < a.NumChannels(); ch++ {
		for _, x := range a.Channels[ch] {
			ax := math.Abs(x)
			if ax > maxPeak {
				maxPeak = ax
			}
			if ax >= 0.999 {
				clipped++
			}
			if ax >= 0.99 {
				nearClip++
			}
		}
	}
	for _, c := range calcs {
		if c.window.LoudnessLUFS != nil && finite(*c.window.LoudnessLUFS) {
			loudness = append(loudness, *c.window.LoudnessLUFS)
			if *c.window.LoudnessLUFS > maxLoudness {
				maxLoudness = *c.window.LoudnessLUFS
			}
		}
	}
	sort.Float64s(loudness)

	var dialogueIntervals [][2]float64
	for _, c := range calcs {
		if c.window.Dialogue {
			dialogueIntervals = append(dialogueIntervals, [2]float64{c.window.StartSec, c.window.EndSec})
		}
	}
	dialogueDuration := unionDuration(dialogueIntervals)
	var silenceDuration float64
	for _, ev := range silence {
		silenceDuration += ev.DurationSec
	}
	activeDuration := math.Max(0, a.Duration()-silenceDuration)

	classification := "mix_heuristic:center+modulation+voice_band"
	if hasStem {
		classification = "dialogue_stem_vad"
	}

	m := Metrics{
		Scheme:                 scheme,
		Label:                  label,
		SampleRate:             a.SampleRate,
		Channels:               a.NumChannels(),
		DurationSec:            a.Duration(),
		WindowMS:               opts.WindowMS,
		HopMS:                  opts.HopMS,
		WindowCount:            len(calcs),
		IntegratedLUFS:         integrated,
		LRA400MS:               lra,
		LRA10:                  p10,
		LRA95:                  p95,
		LRAEBU3S:               lra3s,
		MaxWindowLUFS:          floatPtr(maxLoudness),
		MaxPeakDBFS:            floatPtr(safeDB(maxPeak)),
		ClippedSamples:         clipped,
		NearClipSamples:        nearClip,
		DialogueDurationSec:    dialogueDuration,
		DialogueRatioTotal:     dialogueDuration / a.Duration(),
		ActiveDurationSec:      activeDuration,
		DialogueClassification: classification,
	}
	if len(loudness) > 0 {
		m.P95WindowLUFS = floatPtr(percentile(loudness, 95))
		m.MedianWindowLUFS = floatPtr(percentile(loudness, 50))
	}
	if activeDuration > 0 {
		m.DialogueRatioActive = dialogueDuration / activeDuration
	}
	return m
}

func loudnessRangeLong(a *Audio, blockSec, hopSec float64, integrated *float64) *float64 {
	if integrated == nil {
		return nil
	}
	filtered := applyKWeight(a)
	weights := channelWeights(a.NumChannels())
	block := int(math.Round(float64(a.SampleRate) * blockSec))
	hop := int(math.Round(float64(a.SampleRate) * hopSec))
	if block > a.Frames() || hop <= 0 {
		return nil
	}

	var values []float64
	gate := *integrated - 20
	for start := 0; start+block <= a.Frames(); start += hop {
		var energy float64
		for ch := 0; ch < a.NumChannels(); ch++ {
			var sum float64
			for i := start; i < start+block; i++ {
				v := filtered[ch][i]
				sum += v * v
			}
			energy += weights[ch] * sum / float64(block)
		}
		if energy <= 1e-12 {
			continue
		}
		loudness := -0.691 + 10*math.Log10(energy)
		if loudness > -70 && loudness >= gate {
			values = append(values, loudness)
		}
	}
	if len(values) < 2 {
		return nil
	}
	sort.Float64s(values)
	return floatPtr(percentile(values, 95) - percentile(values, 10))
}

func unionDuration(intervals [][2]float64) float64 {
	if len(intervals) == 0 {
		return 0
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i][0] < intervals[j][0] })
	total := 0.0
	curStart, curEnd := intervals[0][0], intervals[0][1]
	for _, iv := range intervals[1:] {
		if iv[0] <= curEnd {
			if iv[1] > curEnd {
				curEnd = iv[1]
			}
			continue
		}
		total += curEnd - curStart
		curStart, curEnd = iv[0], iv[1]
	}
	total += curEnd - curStart
	return total
}
