package analysis

import (
	"math"
	"sort"

	"forensicaudio/internal/audio"
)

type Metadata struct {
	Filename      string  `json:"filename"`
	SHA256        string  `json:"sha256"`
	Bytes         int     `json:"bytes"`
	SampleRate    int     `json:"sampleRate"`
	Channels      int     `json:"channels"`
	BitsPerSample int     `json:"bitsPerSample"`
	Duration      float64 `json:"duration"`
}

type Analysis struct {
	ID          string         `json:"id,omitempty"`
	Metadata    Metadata       `json:"metadata"`
	Config      Config         `json:"config"`
	Intervals   []Interval     `json:"intervals"`
	Repeats     []RepeatRegion `json:"repeats"`
	Sensitivity float64        `json:"sensitivity"`
	Threshold   ThresholdInfo  `json:"threshold"`
	Stats       Stats          `json:"stats"`
	Legacy      Stats          `json:"legacyStats"`
	Alignment   Alignment      `json:"alignment"`

	sortOrder []int
	cutoff    float64
}

func DecisionCutoff(sensitivity float64) float64 {
	return 0.85 - 0.45*clamp(sensitivity, 0, 1)
}

func Analyze(w *audio.WAV, filename string, sensitivity float64) (*Analysis, error) {
	cfg := DefaultConfig(w.SampleRate)
	frames := extractFrames(w, cfg)
	repeats := DetectRepeats(w)
	if repeats == nil {
		repeats = []RepeatRegion{}
	}
	intervals := make([]Interval, len(frames.features))
	rawPhase := make([]float64, len(frames.features))
	for i, f := range frames.features {
		rawPhase[i] = f.phaseJump
	}
	for i, f := range frames.features {
		startFrame := i * cfg.Hop
		endFrame := minInt(w.FrameCount, startFrame+cfg.FrameSize)
		start := float64(startFrame) / float64(w.SampleRate)
		end := float64(endFrame) / float64(w.SampleRate)
		ctx := contextEvidence(frames, i)
		phase := phaseEvidence(frames, rawPhase, i)
		rep := repeatEvidence(repeats, start, end)
		confidence := maxContextOnlyContribution*ctx.Score + 0.62*phase.Score + 0.72*rep.Score +
			0.35*math.Min(ctx.Score, math.Max(phase.Score, rep.Score))
		confidence = clamp(confidence, 0, 1)
		legacy := math.Abs(shortFloorDB(frames, i)-ctx.NoiseFloorDB) > 2.5
		intervals[i] = Interval{
			Index:         i,
			Start:         round(start, 5),
			End:           round(end, 5),
			RMSDB:         round(f.rmsDB, 2),
			Context:       ctx,
			Phase:         phase,
			Repeat:        rep,
			Confidence:    round(confidence, 4),
			LegacyFlagged: legacy,
			Reasons:       evidenceReasons(ctx, phase, rep),
		}
	}
	a := &Analysis{
		Metadata: Metadata{
			Filename:      filename,
			SHA256:        w.SHA256,
			Bytes:         len(w.Original),
			SampleRate:    w.SampleRate,
			Channels:      w.Channels,
			BitsPerSample: w.BitsPerSample,
			Duration:      round(w.Duration, 4),
		},
		Config:    cfg,
		Intervals: intervals,
		Repeats:   repeats,
	}
	a.Alignment = Alignment{
		Aligned:         true,
		IntervalCount:   len(intervals),
		GridStart:       0,
		GridEnd:         round(w.Duration, 4),
		IntervalSeconds: round(float64(cfg.Hop)/float64(w.SampleRate), 5),
	}
	a.buildSortOrder()
	a.applySensitivity(sensitivity)
	a.Stats = computeStats(a.Intervals, func(iv Interval) bool { return iv.Flagged }, a.Alignment)
	a.Legacy = computeStats(a.Intervals, func(iv Interval) bool { return iv.LegacyFlagged }, a.Alignment)
	a.Legacy.Evidence = map[string]int{"noiseFloor": a.Legacy.Flagged}
	return a, nil
}

func (a *Analysis) buildSortOrder() {
	a.sortOrder = make([]int, len(a.Intervals))
	for i := range a.sortOrder {
		a.sortOrder[i] = i
	}
	sort.SliceStable(a.sortOrder, func(i, j int) bool {
		left := a.Intervals[a.sortOrder[i]].Confidence
		right := a.Intervals[a.sortOrder[j]].Confidence
		if left == right {
			return a.sortOrder[i] < a.sortOrder[j]
		}
		return left < right
	})
}

func (a *Analysis) applySensitivity(sensitivity float64) {
	sensitivity = clamp(sensitivity, 0, 1)
	cutoff := DecisionCutoff(sensitivity)
	for i := range a.Intervals {
		a.Intervals[i].Flagged = a.Intervals[i].Confidence >= cutoff
	}
	a.Sensitivity = round(sensitivity, 3)
	a.cutoff = cutoff
	a.Threshold = ThresholdInfo{
		Sensitivity:      a.Sensitivity,
		ConfidenceCutoff: round(cutoff, 4),
	}
}

func contextEvidence(frames frameSet, i int) ContextEvidence {
	cfg := frames.config
	n := len(frames.features)
	leftStart := maxInt(0, i-cfg.ContextFrames)
	leftEnd := maxInt(0, i-cfg.GuardFrames)
	rightStart := minInt(n, i+cfg.GuardFrames+1)
	rightEnd := minInt(n, i+cfg.ContextFrames+1)
	hasLeft := leftEnd-leftStart >= 3
	hasRight := rightEnd-rightStart >= 3
	ctx := ContextEvidence{
		LeftStart:  seconds(leftStart, cfg, frames.sampleRate),
		LeftEnd:    seconds(leftEnd, cfg, frames.sampleRate),
		RightStart: seconds(rightStart, cfg, frames.sampleRate),
		RightEnd:   seconds(rightEnd, cfg, frames.sampleRate),
	}
	localStart := maxInt(0, i-cfg.LocalFrames/2)
	localEnd := minInt(n, i+cfg.LocalFrames/2+1)
	ctx.LocalDB = round(medianDB(frames.features, localStart, localEnd, -1), 2)
	if !hasLeft || !hasRight {
		if hasLeft {
			ctx.LeftDB = round(medianDB(frames.features, leftStart, leftEnd, -1), 2)
			ctx.PredictedDB = ctx.LeftDB
			ctx.NoiseFloorDB = ctx.LeftDB
		} else if hasRight {
			ctx.RightDB = round(medianDB(frames.features, rightStart, rightEnd, -1), 2)
			ctx.PredictedDB = ctx.RightDB
			ctx.NoiseFloorDB = ctx.RightDB
		}
		return ctx
	}
	ctx.LeftDB = round(medianDB(frames.features, leftStart, leftEnd, -1), 2)
	ctx.RightDB = round(medianDB(frames.features, rightStart, rightEnd, -1), 2)
	ctx.PredictedDB = round(0.5*(ctx.LeftDB+ctx.RightDB), 2)
	ctx.ResidualDB = round(math.Abs(ctx.LocalDB-ctx.PredictedDB), 2)
	ctx.NoiseFloorDB = round(percentileDB(frames.features, leftStart, rightEnd, 0.20), 2)
	spread := sideSpreadDB(frames.features, leftStart, leftEnd, rightStart, rightEnd, ctx.LeftDB, ctx.RightDB)
	ctx.SpreadDB = round(spread, 2)
	rmsScore := clamp((ctx.ResidualDB/math.Max(1.5, spread)-1.5)/3.0, 0, 1)
	bandScore := contextBandScore(frames.features, localStart, localEnd, leftStart, leftEnd, rightStart, rightEnd)
	ctx.Score = round(math.Max(rmsScore, bandScore), 4)
	return ctx
}

func contextBandScore(features []frameFeature, localStart, localEnd, leftStart, leftEnd, rightStart, rightEnd int) float64 {
	var sum float64
	for band := 0; band < BandCount; band++ {
		left := medianBand(features, leftStart, leftEnd, band)
		right := medianBand(features, rightStart, rightEnd, band)
		local := medianBand(features, localStart, localEnd, band)
		predicted := 0.5 * (left + right)
		spread := math.Max(1.5, sideBandSpread(features, leftStart, leftEnd, rightStart, rightEnd, band, left, right))
		sum += math.Abs(local-predicted) / spread
	}
	z := sum / BandCount
	return clamp((z-1.5)/3.0, 0, 1)
}

func sideSpreadDB(features []frameFeature, leftStart, leftEnd, rightStart, rightEnd int, leftDB, rightDB float64) float64 {
	var dev []float64
	for i := leftStart; i < leftEnd; i++ {
		dev = append(dev, math.Abs(features[i].rmsDB-leftDB))
	}
	for i := rightStart; i < rightEnd; i++ {
		dev = append(dev, math.Abs(features[i].rmsDB-rightDB))
	}
	if len(dev) == 0 {
		return 0
	}
	return median(dev)
}

func sideBandSpread(features []frameFeature, leftStart, leftEnd, rightStart, rightEnd, band int, left, right float64) float64 {
	var dev []float64
	for i := leftStart; i < leftEnd; i++ {
		dev = append(dev, math.Abs(features[i].bands[band]-left))
	}
	for i := rightStart; i < rightEnd; i++ {
		dev = append(dev, math.Abs(features[i].bands[band]-right))
	}
	if len(dev) == 0 {
		return 0
	}
	return median(dev)
}

func phaseEvidence(frames frameSet, raw []float64, i int) PhaseEvidence {
	cfg := frames.config
	n := len(raw)
	var base []float64
	for j := maxInt(0, i-cfg.ContextFrames); j < maxInt(0, i-cfg.GuardFrames); j++ {
		base = append(base, raw[j])
	}
	for j := minInt(n, i+cfg.GuardFrames+1); j < minInt(n, i+cfg.ContextFrames+1); j++ {
		base = append(base, raw[j])
	}
	baseline := median(base)
	spread := 1.4826 * mad(base)
	tonal := frames.features[i].tonal
	trust := clamp(tonal*2.5, 0.15, 1)
	denom := math.Max(0.12, 4*spread+0.08)
	score := clamp((raw[i]-baseline-0.04)/denom, 0, 1) * trust
	return PhaseEvidence{
		Jump:         round(raw[i], 4),
		Baseline:     round(baseline, 4),
		Tonal:        round(tonal, 4),
		Score:        round(score, 4),
		InterChannel: frames.features[i].interChannel,
	}
}

func repeatEvidence(regions []RepeatRegion, start, end float64) RepeatEvidence {
	for _, r := range regions {
		if overlapRatio(start, end, r.Start, r.End) > 0.15 ||
			overlapRatio(start, end, r.MatchStart, r.MatchEnd) > 0.15 {
			return RepeatEvidence{
				Score:      1,
				MatchStart: r.MatchStart,
				MatchEnd:   r.MatchEnd,
				SegmentID:  r.ID,
			}
		}
	}
	return RepeatEvidence{}
}

func evidenceReasons(ctx ContextEvidence, phase PhaseEvidence, repeat RepeatEvidence) []string {
	var reasons []string
	if repeat.Score >= 0.5 {
		reasons = append(reasons, "重复样本")
	}
	if phase.Score >= 0.30 {
		reasons = append(reasons, "相位跳变")
	}
	if ctx.Score >= 0.35 {
		reasons = append(reasons, "上下文突变")
	}
	return reasons
}

func computeStats(intervals []Interval, flagged func(Interval) bool, alignment Alignment) Stats {
	stats := Stats{
		Intervals:       len(intervals),
		Evidence:        map[string]int{"context": 0, "phase": 0, "repeat": 0},
		GridStart:       alignment.GridStart,
		GridEnd:         alignment.GridEnd,
		IntervalSeconds: alignment.IntervalSeconds,
	}
	var redStart, redEnd float64
	inRed := false
	for _, iv := range intervals {
		isFlagged := flagged(iv)
		if isFlagged {
			stats.Flagged++
			if !inRed {
				stats.RedRanges++
				redStart = iv.Start
				redEnd = iv.End
				inRed = true
			} else if iv.Start <= redEnd+1e-9 {
				if iv.End > redEnd {
					redEnd = iv.End
				}
			} else {
				stats.FlaggedSeconds += redEnd - redStart
				redStart = iv.Start
				redEnd = iv.End
			}
		}
		if iv.Context.Score >= 0.35 {
			stats.Evidence["context"]++
		}
		if iv.Phase.Score >= 0.30 {
			stats.Evidence["phase"]++
		}
		if iv.Repeat.Score >= 0.5 {
			stats.Evidence["repeat"]++
		}
	}
	if inRed {
		stats.FlaggedSeconds += redEnd - redStart
	}
	if stats.Intervals > 0 {
		stats.FlaggedRatio = round(float64(stats.Flagged)/float64(stats.Intervals), 4)
	}
	stats.FlaggedSeconds = round(stats.FlaggedSeconds, 4)
	return stats
}

func medianDB(features []frameFeature, start, end, band int) float64 {
	if band < 0 {
		values := make([]float64, 0, maxInt(0, end-start))
		for i := start; i < end; i++ {
			values = append(values, features[i].rmsDB)
		}
		return median(values)
	}
	return medianBand(features, start, end, band)
}

func medianBand(features []frameFeature, start, end, band int) float64 {
	values := make([]float64, 0, maxInt(0, end-start))
	for i := start; i < end; i++ {
		values = append(values, features[i].bands[band])
	}
	return median(values)
}

func percentileDB(features []frameFeature, start, end int, p float64) float64 {
	values := make([]float64, 0, maxInt(0, end-start))
	for i := start; i < end; i++ {
		values = append(values, features[i].rmsDB)
	}
	return percentile(values, p)
}

func shortFloorDB(frames frameSet, i int) float64 {
	cfg := frames.config
	start := maxInt(0, i-cfg.GuardFrames)
	end := minInt(len(frames.features), i+cfg.GuardFrames+1)
	return percentileDB(frames.features, start, end, 0.20)
}

func seconds(frame int, cfg Config, sampleRate int) float64 {
	return round(float64(frame*cfg.Hop)/float64(maxInt(1, sampleRate)), 4)
}
