package main

import (
	"math"
	"testing"
)

func TestCenterSitingDecoderRemovesHalfSampleOffset(t *testing.T) {
	width := 320
	height := 8
	edges := []float64{31.5, 79.5, 127.5, 175.5, 223.5, 271.5}
	frame := NewSyntheticChromaFrame(width, height, ChromaCenter, edges)
	ramp := NewSyntheticChromaFrame(width, height, ChromaCenter, nil)

	wrong := EvaluateDecoder("wrong", frame, ramp, ChromaLeft, edges, width, height)
	correct := EvaluateDecoder("correct", frame, ramp, ChromaCenter, edges, width, height)

	if wrong.EdgeMAE < 0.9 || wrong.EdgeMAE > 1.1 {
		t.Fatalf("wrong left-aligned edge error = %.3f, want about one luma sample", wrong.EdgeMAE)
	}
	if correct.EdgeMAE > 0.12 {
		t.Fatalf("site-aware edge error = %.3f, want <= 0.12", correct.EdgeMAE)
	}
	if correct.EdgeMAE >= wrong.EdgeMAE/4 {
		t.Fatalf("corrected edge error %.3f did not substantially improve over %.3f", correct.EdgeMAE, wrong.EdgeMAE)
	}
}

func TestBlurOnlyDoesNotRemoveHalfSampleOffset(t *testing.T) {
	width := 320
	height := 8
	edges := []float64{31.5, 79.5, 127.5, 175.5, 223.5, 271.5}
	frame := NewSyntheticChromaFrame(width, height, ChromaCenter, edges)
	ramp := NewSyntheticChromaFrame(width, height, ChromaCenter, nil)
	blurred := EvaluateDecoder("blurred", BlurChromaPlane(frame), BlurChromaPlane(ramp), ChromaLeft, edges, width, height)

	if blurred.EdgeMAE < 0.9 || blurred.EdgeMAE > 1.2 {
		t.Fatalf("blur-only edge error = %.3f, want retained half-sample offset", blurred.EdgeMAE)
	}
	if blurred.MeanTransition <= 1 {
		t.Fatalf("blur transition width = %.2f, expect a wider transition", blurred.MeanTransition)
	}
}

func TestChromaResamplingKeepsEdgePhase(t *testing.T) {
	width := 64
	height := 4
	edges := []float64{31.5}
	center := NewSyntheticChromaFrame(width, height, ChromaCenter, edges)
	resampled, err := ConvertSiting(center, ChromaLeft)
	if err != nil {
		t.Fatal(err)
	}

	wrongEdge, _, ok := findEdge(center, 0, ChromaLeft, edges[0])
	if !ok {
		t.Fatal("wrong decoder edge not found")
	}
	resampledEdge, _, ok := findEdge(resampled, 0, ChromaLeft, edges[0])
	if !ok {
		t.Fatal("resampled decoder edge not found")
	}

	if math.Abs(wrongEdge-edges[0]) < 0.4 {
		t.Fatalf("wrong edge = %.2f, want half-sample offset", wrongEdge)
	}
	if math.Abs(resampledEdge-edges[0]) >= math.Abs(wrongEdge-edges[0]) {
		t.Fatalf("resampled edge %.2f did not improve over wrong edge %.2f", resampledEdge, wrongEdge)
	}
	if math.Abs(resampledEdge-edges[0]) > 0.9 {
		t.Fatalf("resampled edge = %.2f, reference = %.2f", resampledEdge, edges[0])
	}
}

func TestLeftSitingEncodeAndDecodePreservesChromaAtEvenPixel(t *testing.T) {
	frame := NewSyntheticChromaFrame(32, 2, ChromaLeft, nil)
	value := sampleChroma(frame.Cb, frame.Width, 0, 8, ChromaLeft)
	reference := triangleChroma(8)

	if math.Abs(value-reference) > 0.5 {
		t.Fatalf("left-sited value = %.2f, reference = %.2f", value, reference)
	}
}
