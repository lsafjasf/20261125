package analyze

import (
	"fmt"
	"math"
	"sort"

	"surrounddiag/internal/wav"
)

type analyzer struct {
	file      *wav.File
	config    Config
	channels  [][]float64
	windows   []Window
	Stats     []ChannelStats
	Pairs     []PairResult
	wideField WideFieldCheck
}

func Analyze(file *wav.File, config Config) (*Report, error) {
	run, report, err := prepare(file, config)
	if err != nil {
		return nil, err
	}
	report.Pairs, report.UncorrelatedPairs = run.summarizePairs()
	run.Pairs = report.Pairs
	report.Findings = run.findingsWithoutReference()
	report.WideField = run.wideField
	return report, nil
}

func prepare(file *wav.File, config Config) (*analyzer, *Report, error) {
	if file == nil || file.Channels < 1 {
		return nil, nil, fmt.Errorf("empty WAV")
	}
	config = normalizeConfig(config)
	run := &analyzer{file: file, config: config}
	run.channels = make([][]float64, file.Channels)
	for i := range run.channels {
		run.channels[i] = file.Channel(i)
	}
	run.windows = run.scanWindows()
	report := run.baseReport()
	run.Stats = report.Stats
	return run, report, nil
}

func normalizeConfig(config Config) Config {
	if config.WindowSeconds <= 0 {
		config.WindowSeconds = 1
	}
	if config.HopSeconds <= 0 {
		config.HopSeconds = config.WindowSeconds
	}
	if config.MaxDelayMS <= 0 {
		config.MaxDelayMS = 20
	}
	if config.AnalysisStart < 0 {
		config.AnalysisStart = 0
	}
	return config
}

func (run *analyzer) scanWindows() []Window {
	file := run.file
	startFrame, endFrame := 0, file.Frames
	if run.config.AnalysisStart > 0 {
		startFrame = int(run.config.AnalysisStart.Seconds() * float64(file.SampleRate))
	}
	if run.config.AnalysisEnd > 0 {
		endFrame = int(run.config.AnalysisEnd.Seconds() * float64(file.SampleRate))
	}
	if endFrame > file.Frames {
		endFrame = file.Frames
	}
	if startFrame < 0 {
		startFrame = 0
	}
	if endFrame <= startFrame {
		startFrame, endFrame = 0, file.Frames
	}
	windowFrames := int(math.Round(run.config.WindowSeconds * float64(file.SampleRate)))
	hopFrames := int(math.Round(run.config.HopSeconds * float64(file.SampleRate)))
	if windowFrames < 256 {
		windowFrames = 256
	}
	if hopFrames < 1 {
		hopFrames = 1
	}
	if windowFrames > endFrame-startFrame {
		windowFrames = endFrame - startFrame
	}
	maxLag := int(run.config.MaxDelayMS * float64(file.SampleRate) / 1000)
	var windows []Window
	for start := startFrame; start+windowFrames <= endFrame; start += hopFrames {
		end := start + windowFrames
		window := run.inspectWindow(len(windows), start, end, maxLag)
		windows = append(windows, window)
		if end == endFrame {
			break
		}
	}
	return windows
}

func (run *analyzer) inspectWindow(index, start, end, maxLag int) Window {
	window := Window{
		Index: index,
		Start: float64(start) / float64(run.file.SampleRate),
		End:   float64(end) / float64(run.file.SampleRate),
	}
	window.Channels = make([]WindowChannel, len(run.channels))
	for i, channel := range run.channels {
		stats := statsFor(channel[start:end])
		fillBands(&stats, channel[start:end], run.file.SampleRate)
		window.Channels[i] = WindowChannel{
			Index: i, Name: run.file.Layout[i], RMSDB: stats.RMSDB,
			PeakDB: stats.PeakDB, LowRatio: stats.LowRatio,
			VoiceRatio: stats.VoiceRatio, HighRatio: stats.HighRatio,
		}
	}
	for i := 0; i < len(run.channels); i++ {
		for j := i + 1; j < len(run.channels); j++ {
			pair := WindowPair{A: i, B: j}
			if window.Channels[i].RMSDB > -70 && window.Channels[j].RMSDB > -70 {
				match := Match(run.channels[i], run.channels[j], start, end, maxLag, 8)
				pair.LagSamples = match.LagSamples
				pair.Correlation = match.Correlation
				pair.EnvelopeCorr = match.EnvelopeCorr
				pair.Valid = true
			}
			window.Pairs = append(window.Pairs, pair)
		}
	}
	return window
}

func (run *analyzer) baseReport() *Report {
	report := &Report{
		File:         run.file.Path,
		SampleRate:   run.file.SampleRate,
		Channels:     run.file.Channels,
		Bits:         run.file.Bits,
		Duration:     run.file.Duration.Seconds(),
		Frames:       run.file.Frames,
		Layout:       append([]string(nil), run.file.Layout...),
		LayoutSource: run.file.LayoutSource,
		ChannelMask:  fmt.Sprintf("0x%x", run.file.ChannelMask),
		AnalysisNote: "仅声明顺序是容器元数据；工具不会把未经验证的统计特征当作真实声道标签。",
	}
	for i, channel := range run.channels {
		stats := statsFor(channel)
		fillBands(&stats, channel, run.file.SampleRate)
		stats.Index, stats.Name = i, run.file.Layout[i]
		report.Stats = append(report.Stats, stats)
	}
	return report
}

func (run *analyzer) summarizePairs() ([]PairResult, int) {
	type aggregate struct {
		lags, warnings []int
		corrs, envs    []float64
		active         int
	}
	byPair := make(map[[2]int]*aggregate)
	for _, window := range run.windows {
		for _, pair := range window.Pairs {
			if !pair.Valid || math.Abs(pair.Correlation) < 0.55 && pair.EnvelopeCorr < 0.75 {
				continue
			}
			key := [2]int{pair.A, pair.B}
			agg := byPair[key]
			if agg == nil {
				agg = &aggregate{}
				byPair[key] = agg
			}
			agg.active++
			agg.lags = append(agg.lags, pair.LagSamples)
			agg.corrs = append(agg.corrs, pair.Correlation)
			agg.envs = append(agg.envs, pair.EnvelopeCorr)
			candidate := DelayPair{Correlation: pair.Correlation, EnvelopeCorr: pair.EnvelopeCorr}
			if mirrorEvidence(candidate) {
				agg.warnings = append(agg.warnings, window.Index)
			}
		}
	}
	result := make([]PairResult, 0, len(byPair))
	uncorrelated := 0
	for key, agg := range byPair {
		corr := medianFloat(agg.corrs)
		env := medianFloat(agg.envs)
		if math.Abs(corr) < 0.55 && env < 0.75 {
			uncorrelated++
			continue
		}
		medianLag := medianInt(agg.lags)
		result = append(result, PairResult{
			A: key[0], B: key[1],
			AChannel: run.file.Layout[key[0]], BChannel: run.file.Layout[key[1]],
			LagSamples:  medianLag,
			LagMS:       float64(medianLag) * 1000 / float64(run.file.SampleRate),
			Correlation: corr, EnvelopeCorr: env,
			Inverted:     len(agg.warnings)*2 >= agg.active && len(agg.warnings) > 0,
			MedianLag:    medianLag,
			VoteWarnings: len(agg.warnings), ActiveWindows: agg.active,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		si := math.Abs(result[i].Correlation) * math.Max(0.2, result[i].EnvelopeCorr)
		sj := math.Abs(result[j].Correlation) * math.Max(0.2, result[j].EnvelopeCorr)
		return si > sj
	})
	return result, uncorrelated
}
