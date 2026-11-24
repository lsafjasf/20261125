package main

import "math"

const (
	ink        = "#0f172a"
	muted      = "#64748b"
	grid       = "#e2e8f0"
	panelFill  = "#fbfdff"
	cardStroke = "#cbd5e1"
)

var seriesColors = []string{"#2563eb", "#059669", "#7c3aed", "#d97706", "#0891b2", "#be185d"}

type plotBox struct {
	X, Y, W, H float64
	XMin, XMax float64
	YMin, YMax float64
}

func (p plotBox) MapX(v float64) float64 {
	return p.X + (v-p.XMin)/(p.XMax-p.XMin)*p.W
}

func (p plotBox) MapY(v float64) float64 {
	return p.Y + p.H - (v-p.YMin)/(p.YMax-p.YMin)*p.H
}

func niceBounds(maxValue float64, tickCount int) (float64, []float64) {
	if maxValue <= 0 {
		maxValue = 1
	}
	rawStep := maxValue / float64(tickCount)
	magnitude := math.Pow(10, math.Floor(math.Log10(rawStep)))
	normalized := rawStep / magnitude
	var nice float64
	switch {
	case normalized <= 1:
		nice = 1
	case normalized <= 2:
		nice = 2
	case normalized <= 5:
		nice = 5
	default:
		nice = 10
	}
	step := nice * magnitude
	top := math.Ceil(maxValue/step) * step
	ticks := make([]float64, 0, tickCount+1)
	for v := 0.0; v <= top+step*0.001; v += step {
		ticks = append(ticks, v)
	}
	return top, ticks
}

func grayCurve(c Config, count int) []Sample {
	codes := GrayCodes(count)
	out := make([]Sample, len(codes))
	for i, code := range codes {
		out[i] = c.DecodeGray(code)
	}
	return out
}

func checkpoint(c Config, code float64) Sample {
	return c.DecodeGray(code)
}
