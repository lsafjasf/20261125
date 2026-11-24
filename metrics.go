package main

import (
	"math"
)

type Metrics struct {
	Name            string  `json:"name"`
	EdgeMAE         float64 `json:"edge_mae_luma_samples"`
	ChromaMAE       float64 `json:"chroma_mae_levels"`
	MeanTransition  float64 `json:"mean_transition_luma_samples"`
	VerticalEdgeMAE float64 `json:"vertical_edge_mae_luma_samples"`
}

func EvaluateDecoder(name string, frame, ramp *YUV422, assumed ChromaSiting, edges []float64, width, height int) Metrics {
	edgeErrors := make([]float64, 0, len(edges)*height)
	verticalErrors := make([]float64, 0, len(edges))
	transitionWidths := make([]float64, 0, len(edges)*height)

	for row := 0; row < height; row++ {
		for _, edge := range edges {
			detected, widthPx, ok := findEdge(frame, row, assumed, edge)
			if !ok {
				continue
			}
			edgeErrors = append(edgeErrors, math.Abs(detected-edge))
			transitionWidths = append(transitionWidths, widthPx)
		}
	}

	for _, edge := range edges {
		detected, _, ok := findEdge(frame, height/2, assumed, edge)
		if ok {
			verticalErrors = append(verticalErrors, math.Abs(detected-edge))
		}
	}

	var maeSum float64
	var maeCount float64
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value := sampleChroma(ramp.Cb, width, y, float64(x), assumed)
			maeSum += math.Abs(value - triangleChroma(float64(x)))
			maeCount++
		}
	}

	return Metrics{
		Name:            name,
		EdgeMAE:         mean(edgeErrors),
		ChromaMAE:       maeSum / maeCount,
		MeanTransition:  mean(transitionWidths),
		VerticalEdgeMAE: mean(verticalErrors),
	}
}

func NewSyntheticChromaFrame(width, height int, siting ChromaSiting, edges []float64) *YUV422 {
	planeWidth := width / 2
	frame := &YUV422{
		Width:  width,
		Height: height,
		Siting: siting,
		Y:      make([]uint8, width*height),
		Cb:     make([]uint8, planeWidth*height),
		Cr:     make([]uint8, planeWidth*height),
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			frame.Y[y*width+x] = 128
		}
		for k := 0; k < planeWidth; k++ {
			sampleX := chromaX(siting, k)
			value := triangleChroma(sampleX)
			if len(edges) > 0 {
				value = 40
			}
			for _, edge := range edges {
				if sampleX >= edge {
					value = 210
				}
			}
			offset := y*planeWidth + k
			frame.Cb[offset] = clampByte(value)
			frame.Cr[offset] = clampByte(256 - value)
		}
	}
	return frame
}

func BlurChromaPlane(frame *YUV422) *YUV422 {
	planeWidth := frame.Width / 2
	blurred := &YUV422{
		Width:  frame.Width,
		Height: frame.Height,
		Siting: frame.Siting,
		Y:      append([]uint8(nil), frame.Y...),
		Cb:     make([]uint8, len(frame.Cb)),
		Cr:     make([]uint8, len(frame.Cr)),
	}

	for y := 0; y < frame.Height; y++ {
		for k := 0; k < planeWidth; k++ {
			offset := y*planeWidth + k
			left := y*planeWidth + clampInt(k-1, 0, planeWidth-1)
			right := y*planeWidth + clampInt(k+1, 0, planeWidth-1)
			blurred.Cb[offset] = clampByte((float64(frame.Cb[left]) + 2*float64(frame.Cb[offset]) + float64(frame.Cb[right])) / 4)
			blurred.Cr[offset] = clampByte((float64(frame.Cr[left]) + 2*float64(frame.Cr[offset]) + float64(frame.Cr[right])) / 4)
		}
	}
	return blurred
}

func findEdge(frame *YUV422, row int, assumed ChromaSiting, expected float64) (float64, float64, bool) {
	start := clampInt(int(math.Floor(expected))-8, 0, frame.Width-2)
	end := clampInt(int(math.Ceil(expected))+9, start+2, frame.Width)
	threshold := 125.0

	for x := start; x < end-1; x++ {
		left := sampleChroma(frame.Cb, frame.Width, row, float64(x), assumed)
		right := sampleChroma(frame.Cb, frame.Width, row, float64(x+1), assumed)
		if left <= threshold && right > threshold {
			edge := float64(x) + (threshold-left)/(right-left)
			return edge, crossingWidth(frame, row, assumed, x), true
		}
	}
	return 0, 0, false
}

func crossingWidth(frame *YUV422, row int, assumed ChromaSiting, crossing int) float64 {
	low := 62.5
	high := 187.5
	left := crossing
	right := crossing

	for left > 0 {
		value := sampleChroma(frame.Cb, frame.Width, row, float64(left), assumed)
		if value <= low {
			break
		}
		left--
	}
	for right < frame.Width-1 {
		value := sampleChroma(frame.Cb, frame.Width, row, float64(right), assumed)
		if value >= high {
			break
		}
		right++
	}
	return float64(right - left)
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}
