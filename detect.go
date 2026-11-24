package mixloud

import (
	"fmt"
	"math"
	"sort"
)

func DetectSpecial(a *Audio, calcs []windowCalc, opts Options) (silence, ads, phase, bursts []Event) {
	silence = detectSilence(a, opts)
	ads = detectAdIntrusions(a, calcs, opts)
	phase = detectPhaseInversions(calcs)
	bursts = detectBursts(a, calcs)
	return
}

func detectSilence(a *Audio, opts Options) []Event {
	frameSize := a.SampleRate / 100
	if frameSize <= 0 {
		frameSize = 1
	}
	var events []Event
	var runStart = -1
	var runPeak float64
	flush := func(endFrame int) {
		if runStart < 0 {
			return
		}
		start := float64(runStart) / float64(a.SampleRate)
		end := float64(endFrame) / float64(a.SampleRate)
		if end-start >= opts.MinSilenceSec {
			events = append(events, Event{
				Kind:        "silence",
				Severity:    "info",
				StartSec:    start,
				EndSec:      end,
				DurationSec: end - start,
				PeakDBFS:    floatPtr(safeDB(runPeak)),
				Message:     fmt.Sprintf("低于 %.0f dBFS 的静音/近静音段", opts.SilenceDBFS),
			})
		}
		runStart = -1
		runPeak = 0
	}

	for start := 0; start < a.Frames(); start += frameSize {
		end := minInt(a.Frames(), start+frameSize)
		var energy float64
		var count int
		var peak float64
		for ch := 0; ch < a.NumChannels(); ch++ {
			for i := start; i < end; i++ {
				x := a.Channels[ch][i]
				energy += x * x
				ax := math.Abs(x)
				if ax > peak {
					peak = ax
				}
				count++
			}
		}
		rmsDB := safeDB(math.Sqrt(energy / float64(maxInt(1, count))))
		silent := math.IsInf(rmsDB, -1) || rmsDB < opts.SilenceDBFS
		if silent {
			if runStart < 0 {
				runStart = start
			}
			if peak > runPeak {
				runPeak = peak
			}
		} else {
			flush(start)
		}
	}
	flush(a.Frames())
	return events
}

func detectAdIntrusions(a *Audio, calcs []windowCalc, opts Options) []Event {
	type candidate struct {
		index    int
		baseline float64
		delta    float64
	}
	var candidates []candidate
	for i, c := range calcs {
		if c.window.LoudnessLUFS == nil {
			continue
		}
		baseline := previousBaseline(calcs, i)
		if !finite(baseline) {
			continue
		}
		delta := *c.window.LoudnessLUFS - baseline
		if delta >= opts.AdJumpLU && *c.window.LoudnessLUFS >= -35 {
			candidates = append(candidates, candidate{index: i, baseline: baseline, delta: delta})
		}
	}

	var events []Event
	for run := range contiguousRuns(len(candidates), func(i, j int) bool {
		return candidates[j].index == candidates[i].index+1
	}) {
		first, last := candidates[run[0]], candidates[run[1]]
		w0 := calcs[first.index].window
		w1 := calcs[last.index].window
		duration := w1.EndSec - w0.StartSec
		if duration < opts.AdMinSec {
			continue
		}
		maxLoud := math.Inf(-1)
		maxDelta := math.Inf(-1)
		baseline := first.baseline
		for k := first.index; k <= last.index; k++ {
			if l := calcs[k].window.LoudnessLUFS; l != nil && *l > maxLoud {
				maxLoud = *l
			}
			if l := calcs[k].window.LoudnessLUFS; l != nil {
				if d := *l - baseline; d > maxDelta {
					maxDelta = d
				}
			}
		}
		peak, crest := intervalPeakCrest(a, w0.StartSec, w1.EndSec)
		if crest != nil && *crest >= 12 {
			continue
		}
		severity := "warning"
		message := "短时响度相对前 2 秒基线突增，疑似广告/片头突入"
		if crest != nil && *crest <= 8 {
			severity = "error"
			message = "持续高响度且压缩感强，高度疑似广告突入"
		}
		events = append(events, Event{
			Kind:          "ad_intrusion",
			Severity:      severity,
			StartSec:      w0.StartSec,
			EndSec:        w1.EndSec,
			DurationSec:   duration,
			PeakDBFS:      peak,
			LoudnessLUFS:  floatPtr(maxLoud),
			BaselineLUFS:  floatPtr(baseline),
			DeltaLU:       floatPtr(maxDelta),
			CrestFactorDB: crest,
			Message:       message,
		})
	}
	return events
}

func previousBaseline(calcs []windowCalc, current int) float64 {
	start := maxInt(0, current-20)
	var values []float64
	for i := start; i < current; i++ {
		if l := calcs[i].window.LoudnessLUFS; l != nil && finite(*l) {
			values = append(values, *l)
		}
	}
	if len(values) < 5 {
		return math.NaN()
	}
	sort.Float64s(values)
	return percentile(values, 50)
}

func detectPhaseInversions(calcs []windowCalc) []Event {
	var events []Event
	for run := range boolWindowRuns(calcs, func(w Window) bool { return w.PhaseInverted }) {
		first, last := calcs[run[0]].window, calcs[run[1]].window
		var minCorr, minMono float64 = 1, 0
		for i := run[0]; i <= run[1]; i++ {
			w := calcs[i].window
			if w.Correlation != nil && *w.Correlation < minCorr {
				minCorr = *w.Correlation
			}
			if w.MonoCompatibilityDB != nil && *w.MonoCompatibilityDB < minMono {
				minMono = *w.MonoCompatibilityDB
			}
		}
		events = append(events, Event{
			Kind:         "phase_inversion",
			Severity:     "error",
			StartSec:     first.StartSec,
			EndSec:       last.EndSec,
			DurationSec:  last.EndSec - first.StartSec,
			Correlation:  floatPtr(minCorr),
			MonoCompatDB: floatPtr(minMono),
			Message:      "左右声道强负相关，疑似一声道反相；单声道下混可能抵消",
		})
	}
	return events
}

func detectBursts(a *Audio, calcs []windowCalc) []Event {
	var events []Event
	for run := range boolWindowRuns(calcs, func(w Window) bool {
		return (w.PeakDBFS != nil && *w.PeakDBFS > -0.5) || w.ClippedSamples > 0
	}) {
		first, last := calcs[run[0]].window, calcs[run[1]].window
		peakDB, crest := intervalPeakCrest(a, first.StartSec, last.EndSec)
		clipped := int64(0)
		for i := run[0]; i <= run[1]; i++ {
			clipped += calcs[i].window.ClippedSamples
		}
		severity := "warning"
		message := "400 ms 窗峰值超过 -0.5 dBFS，存在局部爆音风险"
		if clipped > 0 || (peakDB != nil && *peakDB >= -0.1) {
			severity = "error"
			message = "检测到满刻度/削波样本或接近 0 dBFS 的局部峰值"
		}
		events = append(events, Event{
			Kind:          "local_burst",
			Severity:      severity,
			StartSec:      first.StartSec,
			EndSec:        last.EndSec,
			DurationSec:   last.EndSec - first.StartSec,
			PeakDBFS:      peakDB,
			CrestFactorDB: crest,
			Message:       message,
		})
	}
	return events
}

func intervalPeakCrest(a *Audio, startSec, endSec float64) (*float64, *float64) {
	start := maxInt(0, int(math.Floor(startSec*float64(a.SampleRate))))
	end := minInt(a.Frames(), int(math.Ceil(endSec*float64(a.SampleRate))))
	if end <= start {
		return nil, nil
	}
	var peak, energy float64
	var count int
	for ch := 0; ch < a.NumChannels(); ch++ {
		for i := start; i < end; i++ {
			x := a.Channels[ch][i]
			ax := math.Abs(x)
			if ax > peak {
				peak = ax
			}
			energy += x * x
			count++
		}
	}
	peakDB := safeDB(peak)
	rmsDB := safeDB(math.Sqrt(energy / float64(maxInt(1, count))))
	if !finite(peakDB) || !finite(rmsDB) {
		return floatPtr(peakDB), nil
	}
	return floatPtr(peakDB), floatPtr(peakDB - rmsDB)
}

func boolWindowRuns(calcs []windowCalc, pred func(Window) bool) <-chan [2]int {
	ch := make(chan [2]int)
	go func() {
		defer close(ch)
		start := -1
		for i, c := range calcs {
			if pred(c.window) {
				if start < 0 {
					start = i
				}
				continue
			}
			if start >= 0 {
				ch <- [2]int{start, i - 1}
				start = -1
			}
		}
		if start >= 0 {
			ch <- [2]int{start, len(calcs) - 1}
		}
	}()
	return ch
}

func contiguousRuns(n int, adjacent func(i, j int) bool) <-chan [2]int {
	ch := make(chan [2]int)
	go func() {
		defer close(ch)
		if n == 0 {
			return
		}
		start := 0
		for i := 1; i < n; i++ {
			if !adjacent(i-1, i) {
				ch <- [2]int{start, i - 1}
				start = i
			}
		}
		ch <- [2]int{start, n - 1}
	}()
	return ch
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
