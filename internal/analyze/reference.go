package analyze

import (
	"fmt"
	"math"
	"sort"

	"surrounddiag/internal/wav"
)

func AnalyzeReference(file, reference *wav.File, config Config) (*Report, error) {
	if reference == nil {
		return nil, fmt.Errorf("missing reference WAV")
	}
	if file.SampleRate != reference.SampleRate {
		return nil, fmt.Errorf("reference sample rate %d differs from export %d; sample-level comparison refused",
			reference.SampleRate, file.SampleRate)
	}
	if file.Channels != reference.Channels {
		return nil, fmt.Errorf("reference has %d channels, export has %d", reference.Channels, file.Channels)
	}
	run, report, err := prepare(file, config)
	if err != nil {
		return nil, err
	}
	report.Pairs, report.UncorrelatedPairs = run.summarizePairs()
	run.Pairs = report.Pairs
	report.Findings = nil
	run.buildWideFieldGuard()
	report.WideField = run.wideField
	report.Reference = reference.Path
	report.ReferenceStats = referenceStats(reference)
	report.ChannelMap = inferMapping(run, reference)
	report.Findings = append(report.Findings, run.compareWithReference(reference, report.ChannelMap)...)
	report.Findings = dedupeFindings(report.Findings)
	report.AnalysisNote = "有参考母版：声道映射与极性通过同速率、逐样点对齐确认；无镜像证据的低相关声场不判反相。"
	return report, nil
}

func referenceStats(reference *wav.File) []ChannelStats {
	out := make([]ChannelStats, reference.Channels)
	for i := range out {
		channel := reference.Channel(i)
		stats := statsFor(channel)
		fillBands(&stats, channel, reference.SampleRate)
		stats.Index, stats.Name = i, reference.Layout[i]
		out[i] = stats
	}
	return out
}

type mappingCandidate struct {
	output, reference int
	corr, env, score  float64
	lag               int
}

func inferMapping(run *analyzer, reference *wav.File) []ChannelMapping {
	if len(run.windows) == 0 {
		return nil
	}
	selected := selectMappingWindows(run.windows, 5)
	var candidates []mappingCandidate
	maxLag := int(run.config.MaxDelayMS * float64(run.file.SampleRate) / 1000)
	for outIndex := 0; outIndex < run.file.Channels; outIndex++ {
		outputSamples := run.channels[outIndex]
		for refIndex := 0; refIndex < reference.Channels; refIndex++ {
			refSamples := reference.Channel(refIndex)
			var corrs, envs []float64
			var lags []int
			for _, window := range selected {
				start := int(math.Round(window.Start * float64(run.file.SampleRate)))
				end := int(math.Round(window.End * float64(run.file.SampleRate)))
				if end > len(refSamples) {
					end = len(refSamples)
				}
				match := Match(outputSamples, refSamples, start, end, maxLag, 4)
				if math.Abs(match.Correlation) >= 0.35 || match.EnvelopeCorr >= 0.45 {
					corrs = append(corrs, match.Correlation)
					envs = append(envs, match.EnvelopeCorr)
					lags = append(lags, match.LagSamples)
				}
			}
			if len(corrs) == 0 {
				continue
			}
			corr := medianFloat(corrs)
			env := medianFloat(envs)
			score := math.Abs(corr) * (0.55 + 0.45*math.Max(0, env))
			if score < 0.42 {
				continue
			}
			candidates = append(candidates, mappingCandidate{
				output: outIndex, reference: refIndex, corr: corr, env: env,
				score: score, lag: medianInt(lags),
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	assignedOut := make(map[int]mappingCandidate)
	assignedRef := make(map[int]bool)
	for _, candidate := range candidates {
		if _, exists := assignedOut[candidate.output]; exists || assignedRef[candidate.reference] {
			continue
		}
		if math.Abs(candidate.corr) < 0.58 {
			continue
		}
		assignedOut[candidate.output] = candidate
		assignedRef[candidate.reference] = true
	}
	// Prefer the identity mapping when no stronger unique match was found and
	// the same-position signal is still coherent.
	for outIndex := 0; outIndex < run.file.Channels; outIndex++ {
		if _, exists := assignedOut[outIndex]; exists || assignedRef[outIndex] {
			continue
		}
		for _, candidate := range candidates {
			if candidate.output == outIndex && candidate.reference == outIndex && math.Abs(candidate.corr) >= 0.48 {
				assignedOut[outIndex] = candidate
				assignedRef[outIndex] = true
				break
			}
		}
	}
	result := make([]ChannelMapping, run.file.Channels)
	for output := 0; output < run.file.Channels; output++ {
		candidate, ok := assignedOut[output]
		if !ok {
			result[output] = ChannelMapping{
				OutputIndex: output, OutputChannel: run.file.Layout[output],
				ReferenceIndex: -1, ReferenceChannel: "unmatched", Status: "unmatched",
			}
			continue
		}
		status := "matched"
		if candidate.corr <= -0.70 && math.Abs(candidate.corr) >= 0.82 && candidate.env >= 0.65 {
			status = "inverted"
		} else if candidate.reference != output {
			reverse, hasReverse := assignedOut[candidate.reference]
			if hasReverse && reverse.reference == output {
				status = "swapped"
			} else {
				status = "misrouted"
			}
		}
		result[output] = ChannelMapping{
			OutputIndex: output, OutputChannel: run.file.Layout[output],
			ReferenceIndex:   candidate.reference,
			ReferenceChannel: reference.Layout[candidate.reference],
			LagSamples:       candidate.lag, Correlation: candidate.corr,
			EnvelopeCorr: candidate.env, Confidence: math.Min(0.99, candidate.score),
			Status: status,
		}
	}
	return result
}

func selectMappingWindows(windows []Window, limit int) []Window {
	if len(windows) <= limit {
		return windows
	}
	out := make([]Window, 0, limit)
	for i := 0; i < limit; i++ {
		index := int(math.Round(float64(i) * float64(len(windows)-1) / float64(limit-1)))
		out = append(out, windows[index])
	}
	return out
}
