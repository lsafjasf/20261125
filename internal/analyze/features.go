package analyze

import (
	"math"
)

func statsFor(samples []float64) ChannelStats {
	stats := ChannelStats{}
	if len(samples) == 0 {
		return stats
	}
	var sumSquares, peak float64
	silentBlocks, blocks := 0, 0
	const silentBlock = 2400
	for start := 0; start < len(samples); start += silentBlock {
		end := start + silentBlock
		if end > len(samples) {
			end = len(samples)
		}
		var blockEnergy float64
		for _, sample := range samples[start:end] {
			sumSquares += sample * sample
			blockEnergy += sample * sample
			peak = math.Max(peak, math.Abs(sample))
		}
		if math.Sqrt(blockEnergy/float64(end-start)) < 0.001 {
			silentBlocks++
		}
		blocks++
	}
	rms := math.Sqrt(sumSquares / float64(len(samples)))
	stats.RMSDB = db(rms, 1e-6)
	stats.PeakDB = db(peak, 1e-6)
	if rms > 0 {
		stats.CrestDB = stats.PeakDB - stats.RMSDB
	}
	if blocks > 0 {
		stats.Silent = float64(silentBlocks) / float64(blocks)
	}
	return stats
}

func fillBands(stats *ChannelStats, samples []float64, sampleRate int) {
	low, voice, high, total := bandEnergies(samples, sampleRate)
	if total <= 1e-20 {
		return
	}
	stats.LowRatio = low / total
	stats.VoiceRatio = voice / total
	stats.HighRatio = high / total
}

func bandEnergies(samples []float64, sampleRate int) (low, voice, high, total float64) {
	if sampleRate <= 0 {
		return 0, 0, 0, 0
	}
	const block = 8192
	for start := 0; start < len(samples); start += block {
		end := start + block
		if end > len(samples) {
			end = len(samples)
		}
		if end-start < 256 {
			break
		}
		blockLow, blockVoice, blockHigh, blockTotal := bandEnergiesBlock(samples[start:end], sampleRate)
		low += blockLow
		voice += blockVoice
		high += blockHigh
		total += blockTotal
	}
	return low, voice, high, total
}

func bandEnergiesBlock(samples []float64, sampleRate int) (low, voice, high, total float64) {
	size := 1
	for size < len(samples) {
		size <<= 1
	}
	if size < 256 {
		size = 256
	}
	values := make([]complex128, size)
	var mean float64
	for _, sample := range samples {
		mean += sample
	}
	if len(samples) > 0 {
		mean /= float64(len(samples))
	}
	for i := 0; i < size && i < len(samples); i++ {
		window := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(size-1))
		values[i] = complex((samples[i]-mean)*window, 0)
	}
	fft(values)
	nyquist := float64(sampleRate) / 2
	binHz := float64(sampleRate) / float64(size)
	for i := 1; i < size/2; i++ {
		frequency := float64(i) * binHz
		if frequency < 20 || frequency > nyquist {
			continue
		}
		power := real(values[i])*real(values[i]) + imag(values[i])*imag(values[i])
		total += power
		switch {
		case frequency >= 20 && frequency < 120:
			low += power
		case frequency >= 300 && frequency < 3400:
			voice += power
		case frequency >= 8000:
			high += power
		}
	}
	return low, voice, high, total
}

func fft(values []complex128) {
	n := len(values)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			values[i], values[j] = values[j], values[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		step := complex(math.Cos(angle), math.Sin(angle))
		for i := 0; i < n; i += length {
			phase := complex(1, 0)
			for j := 0; j < length/2; j++ {
				even := values[i+j]
				odd := values[i+j+length/2] * phase
				values[i+j] = even + odd
				values[i+j+length/2] = even - odd
				phase *= step
			}
		}
	}
}

func db(value, floor float64) float64 {
	if value < floor {
		return -120
	}
	return 20 * math.Log10(value)
}

func ratioDB(value, reference float64) float64 {
	if reference <= 0 || value <= 0 {
		return -120
	}
	return 20 * math.Log10(value/reference)
}

func rms(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, sample := range samples {
		sum += sample * sample
	}
	return math.Sqrt(sum / float64(len(samples)))
}
