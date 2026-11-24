package mixloud

import "math"

func stemDialogueScore(stem *Audio, start, end int) float64 {
	if stem == nil || start < 0 || end > stem.Frames() || end <= start {
		return 0
	}
	var sumSquares float64
	var count int
	for ch := 0; ch < stem.NumChannels(); ch++ {
		for i := start; i < end; i++ {
			x := stem.Channels[ch][i]
			sumSquares += x * x
			count++
		}
	}
	if count == 0 {
		return 0
	}
	rms := math.Sqrt(sumSquares / float64(count))
	db := safeDB(rms)
	if math.IsInf(db, -1) {
		return 0
	}
	return clamp01((db + 55) / 10)
}

func heuristicDialogueScore(a *Audio, start, end int, centerShare *float64) (score, modulationScore, centerScore float64) {
	if a == nil || end <= start {
		return 0, 0, 0
	}
	voice := dialogueAnalysisChannel(a, start, end)
	var sumSquares float64
	for _, x := range voice {
		sumSquares += x * x
	}
	rmsDB := safeDB(math.Sqrt(sumSquares / float64(len(voice))))
	if math.IsInf(rmsDB, -1) || rmsDB < -55 {
		return 0, 0, 1
	}

	levelScore := clamp01((rmsDB + 52) / 24)
	centerScore = 1.0
	if centerShare != nil {
		centerScore = clamp01((*centerShare - 0.45) / 0.35)
	}
	modulationScore = speechModulation(voice, a.SampleRate)
	bandScore := voiceBandScore(voice, a.SampleRate)
	zcrScore := zeroCrossingScore(voice)

	score = clamp01(0.10*levelScore + 0.30*centerScore + 0.30*modulationScore + 0.20*bandScore + 0.10*zcrScore)
	return score, modulationScore, centerScore
}

func dialogueAnalysisChannel(a *Audio, start, end int) []float64 {
	n := end - start
	out := make([]float64, n)
	if a.NumChannels() >= 2 {
		for i := 0; i < n; i++ {
			out[i] = (a.Channels[0][start+i] + a.Channels[1][start+i]) / 2
		}
		return out
	}
	copy(out, a.Channels[0][start:end])
	return out
}

func speechModulation(x []float64, sampleRate int) float64 {
	block := sampleRate / 50
	if block < 20 || len(x) < 2*block {
		return 0
	}
	var levels []float64
	for start := 0; start+block <= len(x); start += block {
		var sum float64
		for _, v := range x[start : start+block] {
			sum += v * v
		}
		levels = append(levels, math.Sqrt(sum/float64(block)))
	}
	if len(levels) < 4 {
		return 0
	}
	meanLevel := mean(levels)
	if meanLevel < 1e-5 {
		return 0
	}
	var variance float64
	for _, v := range levels {
		variance += (v - meanLevel) * (v - meanLevel)
	}
	cv := math.Sqrt(variance/float64(len(levels))) / meanLevel
	return clamp01((cv - 0.15) / 0.45)
}

func voiceBandScore(x []float64, sampleRate int) float64 {
	high := highPass(sampleRate, 220, 0.707)
	low := lowPass(sampleRate, 4200, 0.707)
	var bandEnergy, fullEnergy float64
	for _, v := range x {
		fullEnergy += v * v
		y := low.process(high.process(v))
		bandEnergy += y * y
	}
	if fullEnergy <= 1e-12 {
		return 0
	}
	return clamp01((bandEnergy/fullEnergy - 0.15) / 0.45)
}

func lowPass(fs int, freq, q float64) biquad {
	w0 := 2 * math.Pi * freq / float64(fs)
	cw, sw := math.Cos(w0), math.Sin(w0)
	alpha := sw / (2 * q)
	a0 := 1 + alpha
	return biquad{
		b0: (1 - cw) / 2 / a0,
		b1: (1 - cw) / a0,
		b2: (1 - cw) / 2 / a0,
		a1: -2 * cw / a0,
		a2: (1 - alpha) / a0,
	}
}

func zeroCrossingScore(x []float64) float64 {
	if len(x) < 2 {
		return 0
	}
	var crossings int
	for i := 1; i < len(x); i++ {
		if (x[i-1] >= 0 && x[i] < 0) || (x[i-1] < 0 && x[i] >= 0) {
			crossings++
		}
	}
	rate := float64(crossings) / float64(len(x)-1)
	switch {
	case rate < 0.004:
		return clamp01(rate / 0.004)
	case rate <= 0.18:
		return 1
	case rate < 0.40:
		return 1 - (rate-0.18)/0.22
	default:
		return 0
	}
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
