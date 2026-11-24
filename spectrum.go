package sdrquant

import (
	"fmt"
	"math"
)

func spectrumDiagnostics(values []float64, width, height, requestedSize int) (*FloatImage, []float64, SpectrumMetrics, error) {
	if len(values) != width*height || width <= 0 || height <= 0 {
		return nil, nil, SpectrumMetrics{}, fmt.Errorf("invalid spectrum input %dx%d", width, height)
	}
	if requestedSize <= 0 {
		requestedSize = 512
	}
	if requestedSize < 64 {
		requestedSize = 64
	}
	if requestedSize > 2048 {
		requestedSize = 2048
	}
	size := nextPowerOfTwo(requestedSize)
	cropWidth := min(width, size)
	cropHeight := min(height, size)
	startX := (width - cropWidth) / 2
	startY := (height - cropHeight) / 2

	mean := 0.0
	for y := 0; y < cropHeight; y++ {
		for x := 0; x < cropWidth; x++ {
			mean += values[(startY+y)*width+startX+x]
		}
	}
	mean /= float64(cropWidth * cropHeight)

	grid := make([]complex128, size*size)
	for y := 0; y < cropHeight; y++ {
		wy := hann(y, cropHeight)
		for x := 0; x < cropWidth; x++ {
			v := values[(startY+y)*width+startX+x] - mean
			grid[y*size+x] = complex(v*hann(x, cropWidth)*wy, 0)
		}
	}
	fft2(grid, size)

	power := make([]float64, size*size)
	shiftedDB := make([]float64, size*size)
	maxDB := math.Inf(-1)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			src := shiftedIndex(x, y, size)
			p := real(grid[src])*real(grid[src]) + imag(grid[src])*imag(grid[src])
			power[y*size+x] = p
			db := 10 * math.Log10(p+1e-15)
			shiftedDB[y*size+x] = db
			if db > maxDB {
				maxDB = db
			}
		}
	}

	normalized := make([]float64, len(power))
	floor := maxDB - 60
	for i, db := range shiftedDB {
		normalized[i] = math.Max(0, math.Min(1, (db-floor)/60))
	}

	metrics := SpectrumMetrics{Size: size}
	total, low, mid, high := 0.0, 0.0, 0.0, 0.0
	weightedRadius := 0.0
	peakPower := math.Inf(-1)
	for y := 0; y < size; y++ {
		fy := float64(y-size/2) / float64(size)
		for x := 0; x < size; x++ {
			fx := float64(x-size/2) / float64(size)
			radius := math.Hypot(fx, fy)
			if radius == 0 {
				continue
			}
			p := power[y*size+x]
			total += p
			weightedRadius += p * radius
			switch {
			case radius < 0.10:
				low += p
			case radius < 0.25:
				mid += p
			default:
				high += p
			}
			if radius >= 0.001 && radius <= 0.5 && p > peakPower {
				peakPower = p
				metrics.PeakRadius = radius
			}
		}
	}
	if total > 0 {
		metrics.LowFrequencyRatio = low / total
		metrics.MidFrequencyRatio = mid / total
		metrics.HighFrequencyRatio = high / total
		metrics.Centroid = weightedRadius / total
	}
	if !math.IsInf(peakPower, -1) {
		metrics.PeakPowerDB = 10 * math.Log10(peakPower+1e-15)
	}
	return heatMap(normalized, size, size), shiftedDB, metrics, nil
}

func hann(index, length int) float64 {
	if length <= 1 {
		return 1
	}
	return 0.5 - 0.5*math.Cos(2*math.Pi*float64(index)/float64(length-1))
}
