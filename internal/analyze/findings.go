package analyze

import (
	"fmt"
	"math"
)

type span struct {
	start, end float64
}

func (run *analyzer) findingsWithoutReference() []Finding {
	var findings []Finding
	findings = append(findings, run.polarityFindings()...)
	findings = append(findings, run.delayFindings()...)
	findings = append(findings, run.roleFindings()...)
	findings = append(findings, run.surroundFindings()...)
	findings = append(findings, run.centerFindings()...)
	findings = append(findings, run.lfeFindings()...)
	run.buildWideFieldGuard()
	return dedupeFindings(findings)
}

func (run *analyzer) polarityFindings() []Finding {
	var findings []Finding
	for _, pair := range run.Pairs {
		if pair.VoteWarnings == 0 {
			continue
		}
		flags := make([]bool, len(run.windows))
		for _, window := range run.windows {
			for _, wp := range window.Pairs {
				if wp.A == pair.A && wp.B == pair.B && wp.Valid {
					candidate := DelayPair{Correlation: wp.Correlation, EnvelopeCorr: wp.EnvelopeCorr}
					flags[window.Index] = mirrorEvidence(candidate)
				}
			}
		}
		for _, current := range spansFrom(flags, run.windows) {
			root := "复制、分轨或插件链中一侧极性被翻转；无参考母版时不能判定是哪一侧。"
			if pair.AChannel == "FC" || pair.BChannel == "FC" {
				root = "中心声道相对左右总线的极性相反；折叠下混会对白相消，表现为对白变虚、声像后退。"
			}
			findings = append(findings, Finding{
				Severity: "high", Code: "polarity_inversion_candidate",
				Channels: []string{pair.AChannel, pair.BChannel},
				Start:    current.start, End: current.end,
				Message: fmt.Sprintf("%s 与 %s 呈强镜像波形（r=%.2f，包络相关=%.2f），且非普通去相关声场。",
					pair.AChannel, pair.BChannel, pair.Correlation, pair.EnvelopeCorr),
				RootCause: root, Confidence: math.Min(0.98, math.Abs(pair.Correlation)*pair.EnvelopeCorr),
				Evidence: "需同时满足 |r|>=0.85、r<=-0.75、包络相关>=0.70；低相关宽声场不触发。",
			})
		}
	}
	return findings
}

func (run *analyzer) delayFindings() []Finding {
	var findings []Finding
	for _, pair := range run.Pairs {
		if pair.Inverted || pair.MedianLag == 0 || math.Abs(pair.Correlation) < 0.90 || pair.EnvelopeCorr < 0.82 {
			continue
		}
		flags := make([]bool, len(run.windows))
		for _, window := range run.windows {
			for _, wp := range window.Pairs {
				if wp.A != pair.A || wp.B != pair.B || !wp.Valid {
					continue
				}
				if wp.LagSamples == pair.MedianLag && math.Abs(wp.Correlation) >= 0.85 {
					flags[window.Index] = true
				}
			}
		}
		for _, current := range spansFrom(flags, run.windows) {
			findings = append(findings, Finding{
				Severity: "info", Code: "consistent_sample_delay",
				Channels: []string{pair.AChannel, pair.BChannel},
				Start:    current.start, End: current.end,
				Message: fmt.Sprintf("%s 相对 %s 稳定延迟 %+d samples（%+.3f ms）。",
					pair.BChannel, pair.AChannel, pair.MedianLag, pair.LagMS),
				RootCause:  "可能来自复制轨、总线延迟补偿或插件 PDC 不一致；也可能是有意的时间差。",
				Confidence: math.Min(0.95, math.Abs(pair.Correlation)*pair.EnvelopeCorr),
				Evidence:   "样本级整数延迟，由原始采样率互相关求得；正号表示 B 晚于 A。",
			})
		}
	}
	return findings
}

func (run *analyzer) roleFindings() []Finding {
	if len(run.file.Layout) != run.file.Channels {
		return nil
	}
	var declaredLFE = -1
	for i, name := range run.file.Layout {
		if name == "LFE" {
			declaredLFE = i
		}
	}
	if declaredLFE < 0 {
		return nil
	}
	active := make(map[int]bool)
	for i, stats := range run.Stats {
		if stats.RMSDB > -48 {
			active[i] = true
		}
	}
	if !active[declaredLFE] {
		return nil
	}
	lfeStats := run.Stats[declaredLFE]
	lfeLike := isLFELike(lfeStats)
	if lfeLike {
		return nil
	}
	for i, stats := range run.Stats {
		if i == declaredLFE || !active[i] || !isLFELike(stats) {
			continue
		}
		return []Finding{{
			Severity: "high", Code: "channel_role_swap_lfe_candidate",
			Channels: []string{"LFE", stats.Name}, Start: 0, End: run.file.Duration.Seconds(),
			Message: fmt.Sprintf("声明为 LFE 的声道低频占比仅 %.0f%%，而 %s 的低频占比达 %.0f%%。",
				lfeStats.LowRatio*100, stats.Name, stats.LowRatio*100),
			RootCause:  "6 声道导出顺序/声道映射疑似错误，低频与另一路内容互换或 LFE 未正确路由。",
			Confidence: 0.72, Evidence: "低频占比与语音/高频占比来自整段 FFT 能量分布，仅作映射线索。",
		}}
	}
	return nil
}

func isLFELike(stats ChannelStats) bool {
	return stats.LowRatio >= 0.60 && stats.VoiceRatio <= 0.15 && stats.HighRatio <= 0.08
}

func (run *analyzer) surroundFindings() []Finding {
	var findings []Finding
	front := run.frontReferenceDB()
	if front < -70 {
		return nil
	}
	for i, name := range run.file.Layout {
		if name != "BL" && name != "BR" && name != "SL" && name != "SR" {
			continue
		}
		var silentFlags []bool
		silentFlags = make([]bool, len(run.windows))
		for _, window := range run.windows {
			stats := window.Channels[i]
			activeFront := run.windowFrontDB(window) > -45
			if activeFront && stats.RMSDB < -60 {
				silentFlags[window.Index] = true
			}
		}
		for _, current := range spansFrom(silentFlags, run.windows) {
			findings = append(findings, Finding{
				Severity: "high", Code: "surround_channel_silent",
				Channels: []string{name}, Start: current.start, End: current.end,
				Message:    fmt.Sprintf("%s 在前后声道有节目电平时无有效信号。", name),
				RootCause:  "环绕分轨漏分配、导出通道被丢弃或映射到未使用声道。",
				Confidence: 0.82, Evidence: "窗口 RMS 低于 -60 dBFS，且前场仍有节目。",
			})
		}
	}
	return findings
}

func (run *analyzer) centerFindings() []Finding {
	center := -1
	for i, name := range run.file.Layout {
		if name == "FC" {
			center = i
		}
	}
	if center < 0 {
		return nil
	}
	front := run.frontReferenceDB()
	stats := run.Stats[center]
	if stats.RMSDB < front-24 || stats.RMSDB < -55 {
		return []Finding{{
			Severity: "warning", Code: "center_missing_candidate",
			Channels: []string{"FC"}, Start: 0, End: run.file.Duration.Seconds(),
			Message:    fmt.Sprintf("FC 比左右前场低 %.1f dB，未形成可靠中心声像。", front-stats.RMSDB),
			RootCause:  "中心声道未参与导出、被折叠进 L/R，或通道映射把 FC 放到了别的声道。",
			Confidence: 0.62, Evidence: "中心电平证据，不单独证明反相；需结合对白试听与参考母版。",
		}}
	}
	if stats.LowRatio > 0.70 && stats.VoiceRatio < 0.12 {
		return []Finding{{
			Severity: "warning", Code: "center_low_frequency_dominant",
			Channels: []string{"FC"}, Start: 0, End: run.file.Duration.Seconds(),
			Message:    fmt.Sprintf("FC 的低频占比 %.0f%%，语音频段仅 %.0f%%。", stats.LowRatio*100, stats.VoiceRatio*100),
			RootCause:  "FC 与 LFE 可能互换，或低频总线被错误送入中心声道。",
			Confidence: 0.68, Evidence: "中心声道内容不符合典型对白/中置的能量分布。",
		}}
	}
	return nil
}

func (run *analyzer) lfeFindings() []Finding {
	for i, name := range run.file.Layout {
		if name != "LFE" {
			continue
		}
		stats := run.Stats[i]
		if stats.RMSDB < -55 {
			return nil
		}
		if stats.LowRatio < 0.40 && stats.VoiceRatio > 0.18 {
			return []Finding{{
				Severity: "warning", Code: "lfe_content_anomaly",
				Channels: []string{"LFE"}, Start: 0, End: run.file.Duration.Seconds(),
				Message: fmt.Sprintf("LFE 仅有 %.0f%% 能量低于 120 Hz，却含 %.0f%% 语音频段能量。",
					stats.LowRatio*100, stats.VoiceRatio*100),
				RootCause:  "LFE 可能被送入全频内容，或声道顺序把普通分轨放进了 LFE。",
				Confidence: 0.70, Evidence: "LFE 应为受限于低频的辅助声道，但该判断也受素材本身影响。",
			}}
		}
	}
	return nil
}

func (run *analyzer) frontReferenceDB() float64 {
	var values []float64
	for _, name := range []string{"FL", "FR"} {
		if index := run.file.ChannelIndex(name); index >= 0 {
			values = append(values, run.Stats[index].RMSDB)
		}
	}
	if len(values) == 0 {
		for i := 0; i < len(run.Stats) && i < 2; i++ {
			values = append(values, run.Stats[i].RMSDB)
		}
	}
	if len(values) == 0 {
		return -120
	}
	maximum := values[0]
	for _, value := range values[1:] {
		maximum = math.Max(maximum, value)
	}
	return maximum
}

func (run *analyzer) windowFrontDB(window Window) float64 {
	var values []float64
	for _, name := range []string{"FL", "FR"} {
		if index := run.file.ChannelIndex(name); index >= 0 {
			values = append(values, window.Channels[index].RMSDB)
		}
	}
	if len(values) == 0 && len(window.Channels) >= 2 {
		values = []float64{window.Channels[0].RMSDB, window.Channels[1].RMSDB}
	}
	maximum := -120.0
	for _, value := range values {
		maximum = math.Max(maximum, value)
	}
	return maximum
}

func (run *analyzer) buildWideFieldGuard() {
	var left, right = run.file.ChannelIndex("FL"), run.file.ChannelIndex("FR")
	guard := WideFieldCheck{Verdict: "未抽取到前场 L/R；未执行宽声场反相防护。"}
	if left >= 0 && right >= 0 {
		guard.CheckedPairs = []string{"FL/FR"}
		found := false
		for _, pair := range run.Pairs {
			if pair.A == left && pair.B == right {
				guard.NegativeCorr = pair.Correlation
				found = true
				if pair.VoteWarnings > 0 {
					guard.MirrorPairs++
				}
			}
		}
		if !found && run.file.Frames > 0 {
			end := run.file.Frames
			if limit := run.file.SampleRate * 5; end > limit {
				end = limit
			}
			maxLag := int(run.config.MaxDelayMS * float64(run.file.SampleRate) / 1000)
			match := Match(run.channels[left], run.channels[right], 0, end, maxLag, 8)
			guard.NegativeCorr = match.Correlation
			if mirrorEvidence(match) {
				guard.MirrorPairs = 1
			}
		}
		if guard.MirrorPairs == 0 {
			guard.Verdict = "FL/FR 无镜像证据；负相关或低相关不被判为反相。"
		} else {
			guard.Verdict = "FL/FR 存在镜像证据，需进一步核对其中一路的极性。"
		}
	}
	run.wideField = guard
}

func spansFrom(flags []bool, windows []Window) []span {
	var out []span
	for index, flagged := range flags {
		if !flagged || index >= len(windows) {
			continue
		}
		current := span{start: windows[index].Start, end: windows[index].End}
		if len(out) > 0 && out[len(out)-1].end >= current.start-1e-6 {
			out[len(out)-1].end = current.end
		} else {
			out = append(out, current)
		}
	}
	return out
}

func dedupeFindings(findings []Finding) []Finding {
	seen := make(map[string]bool, len(findings))
	out := findings[:0]
	for _, finding := range findings {
		key := fmt.Sprintf("%s|%v|%.3f|%.3f", finding.Code, finding.Channels, finding.Start, finding.End)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, finding)
	}
	return out
}
