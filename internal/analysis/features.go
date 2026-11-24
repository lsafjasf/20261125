package analysis

import (
	"math"

	"forensicaudio/internal/audio"
)

type frameFeature struct {
	rmsDB        float64
	centroid     float64
	flatness     float64
	tonal        float64
	bands        [BandCount]float64
	phaseJump    float64
	interChannel bool
}

type frameSet struct {
	features   []frameFeature
	config     Config
	sampleRate int
}

func extractFrames(w *audio.WAV, cfg Config) frameSet {
	n := w.FrameCount
	channels := make([][]float64, w.Channels)
	for ch := range channels {
		channels[ch] = w.Channel(ch)
	}
	frameCount := 1
	if n > cfg.FrameSize {
		frameCount = 1 + (n-cfg.FrameSize)/cfg.Hop
	}
	window := hannWindow(cfg.FrameSize)
	windowEnergy := 0.0
	for _, v := range window {
		windowEnergy += v * v
	}
	if windowEnergy == 0 {
		windowEnergy = 1
	}
	eps := 1e-12
	features := make([]frameFeature, frameCount)
	prevSpec := make([][]complex128, w.Channels)
	prevDelta := make([][]float64, w.Channels)
	for i := 0; i < frameCount; i++ {
		start := i * cfg.Hop
		if start+cfg.FrameSize > n {
			start = maxInt(0, n-cfg.FrameSize)
		}
		var sumSquares float64
		var sampleCount int
		specs := make([][]complex128, w.Channels)
		var power []float64
		for ch := 0; ch < w.Channels; ch++ {
			buf := make([]complex128, cfg.FrameSize)
			for j := 0; j < cfg.FrameSize; j++ {
				idx := start + j
				var sample float64
				if idx < n {
					sample = channels[ch][idx]
				}
				buf[j] = complex(sample*window[j], 0)
				sumSquares += sample * sample
				sampleCount++
			}
			fftInPlace(buf)
			if power == nil {
				power = make([]float64, cfg.FrameSize/2+1)
			}
			for k := range power {
				power[k] += real(buf[k])*real(buf[k]) + imag(buf[k])*imag(buf[k])
			}
			specs[ch] = buf
		}
		denom := float64(sampleCount) * windowEnergy
		if denom <= 0 {
			denom = 1
		}
		for k := range power {
			power[k] /= denom
		}
		rms := math.Sqrt(sumSquares / float64(maxInt(1, sampleCount)))
		features[i].rmsDB = amplitudeDB(rms)
		features[i].centroid, features[i].flatness = spectralShape(power, w.SampleRate, cfg.FrameSize)
		features[i].tonal = clamp(1-features[i].flatness, 0, 1)
		features[i].bands = bandEnergies(power, w.SampleRate, cfg.FrameSize)
		monoJump, interJump, nextDelta := phaseJump(prevSpec, prevDelta, specs, cfg.FrameSize, eps)
		features[i].phaseJump = monoJump
		if interJump > monoJump {
			features[i].phaseJump = interJump
			features[i].interChannel = true
		}
		prevSpec = specs
		prevDelta = nextDelta
	}
	return frameSet{features: features, config: cfg, sampleRate: w.SampleRate}
}

func spectralShape(power []float64, sampleRate, fftSize int) (centroid, flatness float64) {
	var total, weighted float64
	var logSum, linearSum float64
	count := 0
	for k := 1; k < len(power)-1; k++ {
		p := power[k]
		if p <= 1e-14 {
			p = 1e-14
		}
		freq := float64(k) * float64(sampleRate) / float64(fftSize)
		total += p
		weighted += freq * p
		logSum += math.Log(p)
		linearSum += p
		count++
	}
	if total > 0 {
		centroid = weighted / total
	}
	if count > 0 && linearSum > 0 {
		flatness = math.Exp(logSum/float64(count)) / (linearSum / float64(count))
	}
	return clamp(centroid, 0, float64(sampleRate)/2), clamp(flatness, 0, 1)
}

func bandEnergies(power []float64, sampleRate, fftSize int) [BandCount]float64 {
	var out [BandCount]float64
	nyquist := float64(sampleRate) / 2
	loHz := math.Max(40, nyquist*0.002)
	hiHz := nyquist * 0.96
	if hiHz <= loHz {
		hiHz = nyquist
	}
	logLo := math.Log(loHz)
	logHi := math.Log(hiHz)
	for band := 0; band < BandCount; band++ {
		f0 := math.Exp(logLo + (logHi-logLo)*float64(band)/BandCount)
		f1 := math.Exp(logLo + (logHi-logLo)*float64(band+1)/BandCount)
		k0 := int(math.Floor(f0 * float64(fftSize) / float64(sampleRate)))
		k1 := int(math.Ceil(f1 * float64(fftSize) / float64(sampleRate)))
		if k0 < 1 {
			k0 = 1
		}
		if k1 <= k0 {
			k1 = k0 + 1
		}
		if k1 > len(power) {
			k1 = len(power)
		}
		sum := 0.0
		for k := k0; k < k1; k++ {
			sum += power[k]
		}
		mean := sum / float64(maxInt(1, k1-k0))
		out[band] = 10 * math.Log10(mean+1e-14)
	}
	return out
}

func phaseJump(prev [][]complex128, prevDelta [][]float64, cur [][]complex128, fftSize int, eps float64) (mono, inter float64, nextDelta [][]float64) {
	if len(prev) == 0 || len(prev) != len(cur) {
		return 0, 0, nil
	}
	for ch := range prev {
		if len(prev[ch]) == 0 || len(prev[ch]) != len(cur[ch]) {
			return 0, 0, nil
		}
	}
	nextDelta = make([][]float64, len(cur))
	totalMono := 0.0
	totalWeight := 0.0
	for ch := range cur {
		jump, weight, delta := resultPhaseResidual(cur[ch], prev[ch], prevDeltaAt(prevDelta, ch), fftSize, eps)
		nextDelta[ch] = delta
		if weight > 0 {
			totalMono += jump * weight
			totalWeight += weight
		}
	}
	if totalWeight > 0 {
		mono = totalMono / totalWeight
	}
	if len(cur) < 2 {
		return mono, 0, nextDelta
	}
	var interWeighted, interWeights float64
	for ch := 1; ch < len(cur); ch++ {
		sumCos, sumSin, sumW := 0.0, 0.0, 0.0
		n := minInt(len(cur[0]), len(cur[ch]))
		for k := 1; k < n-1; k++ {
			m0c := math.Hypot(real(cur[0][k]), imag(cur[0][k]))
			mcc := math.Hypot(real(cur[ch][k]), imag(cur[ch][k]))
			m0p := math.Hypot(real(prev[0][k]), imag(prev[0][k]))
			mcp := math.Hypot(real(prev[ch][k]), imag(prev[ch][k]))
			thr := eps * float64(fftSize)
			if m0c < thr || mcc < thr || m0p < thr || mcp < thr {
				continue
			}
			curDiff := math.Atan2(imag(cur[ch][k]), real(cur[ch][k])) -
				math.Atan2(imag(cur[0][k]), real(cur[0][k]))
			prevDiff := math.Atan2(imag(prev[ch][k]), real(prev[ch][k])) -
				math.Atan2(imag(prev[0][k]), real(prev[0][k]))
			residual := wrapAngle(curDiff - prevDiff)
			weight := math.Sqrt(m0c * mcc * m0p * mcp)
			sumCos += weight * math.Cos(residual)
			sumSin += weight * math.Sin(residual)
			sumW += weight
		}
		if sumW > 0 {
			interWeighted += 1 - math.Hypot(sumCos, sumSin)/sumW
			interWeights++
		}
	}
	if interWeights > 0 {
		inter = interWeighted / interWeights
	}
	return mono, inter, nextDelta
}

func resultPhaseResidual(cur, prev []complex128, prevDelta []float64, fftSize int, eps float64) (jump, weight float64, delta []float64) {
	n := minInt(len(cur), len(prev))
	sumCos, sumSin, sumW := 0.0, 0.0, 0.0
	delta = make([]float64, n)
	for i := range delta {
		delta[i] = math.NaN()
	}
	threshold := math.Max(eps*float64(fftSize), 0.05*maxMagnitude(cur))
	for k := 1; k < n-1; k++ {
		mc := math.Hypot(real(cur[k]), imag(cur[k]))
		mp := math.Hypot(real(prev[k]), imag(prev[k]))
		if mc < threshold || mp < threshold {
			continue
		}
		cross := cur[k] * complex(real(prev[k]), -imag(prev[k]))
		delta[k] = math.Atan2(imag(cross), real(cross))
		if k >= len(prevDelta) || math.IsNaN(prevDelta[k]) {
			continue
		}
		residual := wrapAngle(delta[k] - prevDelta[k])
		w := math.Sqrt(mc * mp)
		sumCos += w * math.Cos(residual)
		sumSin += w * math.Sin(residual)
		sumW += w
	}
	if sumW <= 0 {
		return 0, 0, delta
	}
	return 1 - math.Hypot(sumCos, sumSin)/sumW, sumW, delta
}

func prevDeltaAt(all [][]float64, ch int) []float64 {
	if ch < len(all) {
		return all[ch]
	}
	return nil
}

func maxMagnitude(spec []complex128) float64 {
	maxV := 0.0
	for k := 1; k < len(spec)-1; k++ {
		m := math.Hypot(real(spec[k]), imag(spec[k]))
		if m > maxV {
			maxV = m
		}
	}
	return maxV
}

func amplitudeDB(v float64) float64 {
	if v <= 1e-9 {
		return -180
	}
	return 20 * math.Log10(v)
}

func wrapAngle(v float64) float64 {
	for v > math.Pi {
		v -= 2 * math.Pi
	}
	for v < -math.Pi {
		v += 2 * math.Pi
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
