package analyze

import (
	"fmt"
	"math"

	"surrounddiag/internal/wav"
)

func (run *analyzer) compareWithReference(reference *wav.File, mappings []ChannelMapping) []Finding {
	if len(run.windows) == 0 || len(mappings) != run.file.Channels {
		return nil
	}
	maxLag := int(run.config.MaxDelayMS * float64(run.file.SampleRate) / 1000)
	refChannels := make([][]float64, reference.Channels)
	for i := range refChannels {
		refChannels[i] = reference.Channel(i)
	}
	var findings []Finding
	for outIndex, mapping := range mappings {
		expected := outIndex
		target := expected
		if mapping.ReferenceIndex >= 0 {
			target = mapping.ReferenceIndex
		}
		routeFlags := make([]bool, len(run.windows))
		invertFlags := make([]bool, len(run.windows))
		delayFlags := make([]bool, len(run.windows))
		missingFlags := make([]bool, len(run.windows))
		for _, window := range run.windows {
			start := int(math.Round(window.Start * float64(run.file.SampleRate)))
			end := int(math.Round(window.End * float64(run.file.SampleRate)))
			if end > len(refChannels[expected]) || end > len(refChannels[target]) {
				end = minInt(end, len(refChannels[expected]), len(refChannels[target]))
			}
			if end <= start {
				continue
			}
			outStats := window.Channels[outIndex]
			expectedStats := statsFor(refChannels[expected][start:end])
			expectedMatch := Match(run.channels[outIndex], refChannels[expected], start, end, maxLag, 8)
			if expectedStats.RMSDB > -48 && outStats.RMSDB < -60 {
				missingFlags[window.Index] = true
				continue
			}
			if outStats.RMSDB < -85 || expectedStats.RMSDB < -65 {
				continue
			}
			if target != expected {
				targetStats := statsFor(refChannels[target][start:end])
				if targetStats.RMSDB > -60 {
					mappedMatch := Match(run.channels[outIndex], refChannels[target], start, end, maxLag, 8)
					if positiveMatch(mappedMatch) && math.Abs(mappedMatch.Correlation) > math.Abs(expectedMatch.Correlation)+0.04 {
						routeFlags[window.Index] = true
						continue
					}
				}
			}
			if mirrorMatch(expectedMatch) {
				invertFlags[window.Index] = true
				continue
			}
			if positiveMatch(expectedMatch) && math.Abs(expectedMatch.Correlation) >= 0.88 &&
				expectedMatch.EnvelopeCorr >= 0.80 && expectedMatch.LagSamples != 0 {
				delayFlags[window.Index] = true
			}
		}
		findings = append(findings, findingFromFlags(run, routeFlags, Finding{
			Severity: "high", Code: "reference_channel_swap_or_misroute",
			Channels:   []string{run.file.Layout[outIndex], mapping.ReferenceChannel},
			RootCause:  "导出通道顺序/总线映射与参考母版不一致，造成对白或环绕被送到错误声道。",
			Confidence: clamp01(mapping.Confidence),
			Evidence: fmt.Sprintf("该路与参考 %s 唯一最佳匹配（r=%.2f）；同位置参考声道不再是最佳匹配。",
				mapping.ReferenceChannel, mapping.Correlation),
		}, "该时间段内导出声道与参考同位置声道不一致，而与另一参考声道一致。")...)
		findings = append(findings, findingFromFlags(run, invertFlags, Finding{
			Severity: "high", Code: "reference_polarity_inversion",
			Channels:   []string{run.file.Layout[outIndex]},
			RootCause:  "相对参考母版发生极性翻转；中心声道翻转会在下混中抵消对白，环绕翻转会削弱扩散声场。",
			Confidence: 0.96,
			Evidence:   "导出与参考同声道强负相关且镜像包络高度一致，不是仅凭低相关判断。",
		}, "该时间段样点波形与参考母版呈镜像。")...)
		findings = append(findings, findingFromFlags(run, delayFlags, Finding{
			Severity: "warning", Code: "reference_sample_delay",
			Channels:   []string{run.file.Layout[outIndex]},
			RootCause:  "导出链存在未补偿延迟、重复轨或总线 PDC 偏差。",
			Confidence: clamp01(mapping.Confidence),
			Evidence: fmt.Sprintf("参考相对导出 %+d samples（%+.3f ms）；负值表示导出比参考晚，正值表示导出比参考早。",
				mapping.LagSamples, float64(mapping.LagSamples)*1000/float64(run.file.SampleRate)),
		}, "该时间段存在样本级延迟。")...)
		findings = append(findings, findingFromFlags(run, missingFlags, Finding{
			Severity: "high", Code: "reference_channel_missing",
			Channels:   []string{run.file.Layout[outIndex]},
			RootCause:  "该分轨未导出、被静音、被送到未使用声道或导出时被降混删除。",
			Confidence: 0.90,
			Evidence:   "参考声道有节目电平，而导出对应声道低于 -60 dBFS。",
		}, "该时间段导出声道静音，但参考母版同声道有节目。")...)
	}
	findings = append(findings, run.referenceDistributionFindings(reference, mappings)...)
	return findings
}

func positiveMatch(match DelayPair) bool {
	return match.Correlation >= 0.55 && match.EnvelopeCorr >= 0.35
}

func mirrorMatch(match DelayPair) bool {
	return mirrorEvidence(match)
}

func findingFromFlags(run *analyzer, flags []bool, base Finding, message string) []Finding {
	var out []Finding
	for _, current := range spansFrom(flags, run.windows) {
		finding := base
		finding.Start, finding.End = current.start, current.end
		finding.Message = fmt.Sprintf("%s %.3f–%.3f s：%s", finding.Channels[0], current.start, current.end, message)
		out = append(out, finding)
	}
	return out
}

func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}
