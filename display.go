package mp4diag

import "fmt"

func displayWithReference(report *Report, refReport *Report) DisplayImpact {
	total := len(refReport.Samples)
	impact := DisplayImpact{
		Method:     "reference_hashes_plus_stss_gop_model",
		Confidence: "high_for_linear_no_b_frame_fixture",
	}

	if report.Reference == nil {
		return displayWithoutReference(report)
	}

	if changed := report.Reference.ChangedSamples; len(changed) > 0 {
		first := minIntSlice(changed)
		if first >= 1 && first <= len(refReport.Samples) &&
			refReport.Samples[first-1].ReferenceStatus == "non_reference" {
			impact.FaultSamples = []int{first}
			impact.AffectedSamples = []int{first}
			impact.AffectedDisplayFrames = displayOrdersForSamples(refReport.Samples, impact.AffectedSamples)
			impact.CleanSampleRanges = complementSpans(total, impact.AffectedSamples)
			affectedDisplaySet := intSet(impact.AffectedDisplayFrames)
			var cleanDisplay []int
			for frame := 1; frame <= total; frame++ {
				if !affectedDisplaySet[frame] {
					cleanDisplay = append(cleanDisplay, frame)
				}
			}
			impact.CleanDisplayFrameRanges = consecutiveSpans(cleanDisplay)
			impact.Confidence = "high_for_nal_ref_idc_non_reference"
			impact.Note = "损坏 sample 的 NAL nal_ref_idc=0；按非参考帧只影响自身处理，最终仍建议帧哈希复核"
			return impact
		}

		nextSync := nextSyncAfter(refReport.Samples, first, total)
		lastAffected := nextSync - 1
		if nextSync > total {
			lastAffected = total
			impact.NextSyncSample = 0
		} else {
			impact.NextSyncSample = nextSync
		}

		impact.FaultSamples = []int{first}
		impact.AffectedSamples = inclusiveRange(first, lastAffected)
		impact.AffectedDisplayFrames = displayOrdersForSamples(refReport.Samples, impact.AffectedSamples)
		impact.CleanSampleRanges = mergeSpans([]Span{spanIf(1, first-1), spanIf(nextSync, total)})
		affectedDisplaySet := intSet(impact.AffectedDisplayFrames)
		var cleanDisplay []int
		for frame := 1; frame <= total; frame++ {
			if !affectedDisplaySet[frame] {
				cleanDisplay = append(cleanDisplay, frame)
			}
		}
		impact.CleanDisplayFrameRanges = consecutiveSpans(cleanDisplay)
		impact.Note = "参考哈希确认压缩样本不同；P 帧参考传播区间由 stss 同步样本界定，最终像素范围可用 framemd5/解码器复核"
		return impact
	}

	if missing := report.Reference.MissingSamples; len(missing) > 0 {
		firstMissing := minIntSlice(missing)
		impact.FaultSamples = []int{firstMissing}
		impact.AffectedSamples = inclusiveRange(firstMissing, total)
		impact.AffectedDisplayFrames = impact.AffectedSamples
		impact.CleanSampleRanges = []Span{{First: 1, Last: firstMissing - 1}}
		if firstMissing == 1 {
			impact.CleanSampleRanges = nil
		}
		impact.CleanDisplayFrameRanges = impact.CleanSampleRanges
		impact.Confidence = "high"
		impact.Note = "参考区间在候选文件中缺失；这是容器/数据完整性问题，不把缺失字节称为像素翻转"
		return impact
	}

	impact.CleanSampleRanges = []Span{{First: 1, Last: total}}
	impact.CleanDisplayFrameRanges = []Span{{First: 1, Last: total}}
	impact.Confidence = "high_with_reference"
	impact.Note = "所有参考 sample 的压缩负载哈希一致；若仅有样本表差异，修复元数据后不应留下像素损坏"
	return impact
}

func appendDisplayFindings(report *Report) {
	display := report.Display
	if len(display.AffectedDisplayFrames) > 0 {
		pixel := PixelNotAssessed
		code := "display_range_unavailable"
		if report.Reference != nil && len(report.Reference.ChangedSamples) > 0 {
			pixel = PixelSuspected
			code = "predicted_flower_frame_range"
		}
		report.Findings = append(report.Findings, Finding{
			Layer: LayerDisplay, Severity: SeverityWarning, Code: code,
			Message: fmt.Sprintf("受影响显示帧 %v；干净可显示区间 %s", display.AffectedDisplayFrames, formatSpans(display.CleanDisplayFrameRanges)),
			Samples: display.AffectedSamples, PixelDamage: pixel,
		})
		return
	}
	if report.Reference != nil && report.Reference.PixelDamage == PixelRuledOut && hasMetadataFault(report) {
		report.Findings = append(report.Findings, Finding{
			Layer: LayerDisplay, Severity: SeverityInfo, Code: "metadata_only_pixels_intact",
			Message:     "参考 sample 负载全部一致；显示异常只可能来自元数据/映射，不应报告为像素损坏",
			PixelDamage: PixelRuledOut,
		})
	}
}

func formatSpans(spans []Span) string {
	if len(spans) == 0 {
		return "无"
	}
	out := ""
	for i, span := range spans {
		if i > 0 {
			out += ","
		}
		if span.First == span.Last {
			out += fmt.Sprintf("%d", span.First)
		} else {
			out += fmt.Sprintf("%d-%d", span.First, span.Last)
		}
	}
	return out
}

func nextSyncAfter(samples []SampleInfo, after, total int) int {
	for number := after + 1; number <= total; number++ {
		if samples[number-1].Sync {
			return number
		}
	}
	return total + 1
}

func inclusiveRange(first, last int) []int {
	if first > last {
		return nil
	}
	out := make([]int, 0, last-first+1)
	for n := first; n <= last; n++ {
		out = append(out, n)
	}
	return out
}

func spanIf(first, last int) Span {
	if first > last {
		return Span{}
	}
	return Span{First: first, Last: last}
}

func mergeSpans(spans []Span) []Span {
	var out []Span
	for _, span := range spans {
		if span.First == 0 || span.Last == 0 || span.First > span.Last {
			continue
		}
		if len(out) == 0 || span.First > out[len(out)-1].Last+1 {
			out = append(out, span)
			continue
		}
		if span.Last > out[len(out)-1].Last {
			out[len(out)-1].Last = span.Last
		}
	}
	return out
}

func displayOrdersForSamples(samples []SampleInfo, numbers []int) []int {
	set := intSet(numbers)
	var orders []int
	for _, sample := range samples {
		if set[sample.Number] {
			orders = append(orders, sample.DisplayOrder)
		}
	}
	return orders
}

func intSet(values []int) map[int]bool {
	set := make(map[int]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func minIntSlice(values []int) int {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}
	return min
}
