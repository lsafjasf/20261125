package analysis

import (
	"math"
	"sort"
)

const (
	BandCount = 16
	// Context-only evidence is deliberately capped below the lowest possible
	// decision threshold. A noise-floor drift can be informative, but it must
	// never turn an interval red on its own.
	maxContextOnlyContribution = 0.28
)

type Config struct {
	FrameSize     int `json:"frameSize"`
	Hop           int `json:"hop"`
	ContextFrames int `json:"contextFrames"`
	GuardFrames   int `json:"guardFrames"`
	LocalFrames   int `json:"localFrames"`
}

func DefaultConfig(sampleRate int) Config {
	frameSize := 2048
	if sampleRate > 0 && sampleRate < 32000 {
		frameSize = 1024
	}
	hop := frameSize / 2
	contextFrames := int(math.Round(2.0 * float64(sampleRate) / float64(hop)))
	if contextFrames < 8 {
		contextFrames = 8
	}
	guardFrames := int(math.Round(0.20 * float64(sampleRate) / float64(hop)))
	if guardFrames < 2 {
		guardFrames = 2
	}
	return Config{
		FrameSize:     frameSize,
		Hop:           hop,
		ContextFrames: contextFrames,
		GuardFrames:   guardFrames,
		LocalFrames:   3,
	}
}

type ContextEvidence struct {
	LeftStart    float64 `json:"leftStart"`
	LeftEnd      float64 `json:"leftEnd"`
	RightStart   float64 `json:"rightStart"`
	RightEnd     float64 `json:"rightEnd"`
	LocalDB      float64 `json:"localDb"`
	LeftDB       float64 `json:"leftDb"`
	RightDB      float64 `json:"rightDb"`
	PredictedDB  float64 `json:"predictedDb"`
	ResidualDB   float64 `json:"residualDb"`
	SpreadDB     float64 `json:"spreadDb"`
	NoiseFloorDB float64 `json:"noiseFloorDb"`
	Score        float64 `json:"score"`
}

type PhaseEvidence struct {
	Jump         float64 `json:"jump"`
	Baseline     float64 `json:"baseline"`
	Tonal        float64 `json:"tonal"`
	Score        float64 `json:"score"`
	InterChannel bool    `json:"interChannel"`
}

type RepeatEvidence struct {
	Score      float64 `json:"score"`
	MatchStart float64 `json:"matchStart"`
	MatchEnd   float64 `json:"matchEnd"`
	SegmentID  string  `json:"segmentId"`
}

type Interval struct {
	Index         int             `json:"index"`
	Start         float64         `json:"start"`
	End           float64         `json:"end"`
	RMSDB         float64         `json:"rmsDb"`
	Context       ContextEvidence `json:"context"`
	Phase         PhaseEvidence   `json:"phase"`
	Repeat        RepeatEvidence  `json:"repeat"`
	Confidence    float64         `json:"confidence"`
	Flagged       bool            `json:"flagged"`
	LegacyFlagged bool            `json:"legacyFlagged"`
	Reasons       []string        `json:"reasons,omitempty"`
}

type RepeatRegion struct {
	ID         string  `json:"id"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	MatchStart float64 `json:"matchStart"`
	MatchEnd   float64 `json:"matchEnd"`
	Frames     int     `json:"frames"`
	Similarity float64 `json:"similarity"`
}

type Stats struct {
	Intervals       int            `json:"intervals"`
	Flagged         int            `json:"flagged"`
	FlaggedRatio    float64        `json:"flaggedRatio"`
	FlaggedSeconds  float64        `json:"flaggedSeconds"`
	RedRanges       int            `json:"redRanges"`
	Evidence        map[string]int `json:"evidence"`
	GridStart       float64        `json:"gridStart"`
	GridEnd         float64        `json:"gridEnd"`
	IntervalSeconds float64        `json:"intervalSeconds"`
}

type Alignment struct {
	Aligned         bool    `json:"aligned"`
	IntervalCount   int     `json:"intervalCount"`
	GridStart       float64 `json:"gridStart"`
	GridEnd         float64 `json:"gridEnd"`
	IntervalSeconds float64 `json:"intervalSeconds"`
}

type ThresholdInfo struct {
	Sensitivity      float64 `json:"sensitivity"`
	ConfidenceCutoff float64 `json:"confidenceCutoff"`
}

type Range struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return 0.5 * (cp[n/2-1] + cp[n/2])
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	pos := clamp(p, 0, 1) * float64(len(cp)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return cp[lo]
	}
	frac := pos - float64(lo)
	return cp[lo]*(1-frac) + cp[hi]*frac
}

func mad(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := median(values)
	dev := make([]float64, len(values))
	for i, v := range values {
		dev[i] = math.Abs(v - m)
	}
	return median(dev)
}

func round(v float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(v*pow) / pow
}
