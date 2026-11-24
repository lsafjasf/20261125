package sync

import (
	"fmt"
	"time"
)

func conclude(report *Report) {
	if report.FirstAbnormalFrame != nil {
		first := report.FirstAbnormalFrame
		report.Classification = first.Kind
		report.Confidence = "medium"
		if report.EstimatedSkippedFrames == 0 {
			report.EstimatedSkippedFrames = firstMissingFrames(report, first)
		}
		for i := range report.Series {
			series := &report.Series[i]
			if series.Name == first.Series && series.JumpFrame != nil &&
				*series.JumpFrame == first.Frame {
				report.Confidence = series.Confidence
				report.EstimatedDriftNSPerFrame = series.PostJumpSlopeNSPerFrame
				break
			}
		}
	} else {
		selected := selectPrimarySeries(report)
		if selected == nil {
			report.Classification = KindInsufficient
			report.Confidence = "low"
			report.Summary = "轨迹样本不足，无法判断同步形态。"
			return
		}

		report.Classification = selected.Kind
		report.Confidence = selected.Confidence
		report.EstimatedDriftNSPerFrame = selected.SlopeNSPerFrame
		report.EstimatedOnsetFrame = selected.EstimatedOnsetFrame

		frame := firstSeriesThresholdFrame(selected)
		if frame == nil {
			firstFrame := selected.FirstFrame
			frame = &firstFrame
		}
		ref := &FrameRef{
			Frame:  *frame,
			Stage:  selected.Stage,
			Series: selected.Name,
			Kind:   selected.Kind,
		}
		report.FirstAbnormalFrame = ref
		report.FirstRootCauseFrame = earliestSeriesCause(report)
		if selected.Stage == StagePlayback {
			report.FirstPlaybackAnomalyFrame = ref
		}
	}

	if lip := findSeries(report, "lip_sync_playback"); lip != nil {
		report.ObservedFinalOffsetNS = lip.LastValueNS
		if report.EstimatedDriftNSPerFrame == 0 &&
			report.Classification != KindFrameSkip &&
			report.Classification != KindLocalJump {
			report.EstimatedDriftNSPerFrame = lip.SlopeNSPerFrame
		}
		if report.FirstPlaybackAnomalyFrame == nil &&
			(lip.Kind == KindFixedOffset || lip.Kind == KindLinearDrift) {
			frame := firstSeriesThresholdFrame(lip)
			if frame != nil {
				report.FirstPlaybackAnomalyFrame = &FrameRef{
					Frame:  *frame,
					Stage:  StagePlayback,
					Series: lip.Name,
					Kind:   lip.Kind,
				}
			}
		}
	} else if endToEnd := findSeries(report, "end_to_end_capture_to_playback"); endToEnd != nil {
		report.ObservedFinalOffsetNS = endToEnd.LastValueNS
		if report.EstimatedDriftNSPerFrame == 0 {
			report.EstimatedDriftNSPerFrame = endToEnd.SlopeNSPerFrame
		}
	}

	report.Summary = formatSummary(report)
}

func firstMissingFrames(report *Report, ref *FrameRef) int64 {
	for _, anomaly := range report.Anomalies {
		if anomaly.Frame == ref.Frame && anomaly.Series == ref.Series {
			return anomaly.MissingFrames
		}
	}
	return 0
}

func findSeries(report *Report, name string) *SeriesReport {
	for i := range report.Series {
		if report.Series[i].Name == name {
			return &report.Series[i]
		}
	}
	return nil
}

func selectPrimarySeries(report *Report) *SeriesReport {
	preferred := []string{
		"lip_sync_playback",
		"end_to_end_capture_to_playback",
		"lip_sync_render",
		"render_to_playback",
		"edit_to_render",
		"pts_to_edit",
		"capture_to_pts",
	}
	for _, name := range preferred {
		series := findSeries(report, name)
		if series != nil && isAbnormalKind(series.Kind) {
			return series
		}
	}
	for i := range report.Series {
		if isAbnormalKind(report.Series[i].Kind) {
			return &report.Series[i]
		}
	}
	playbackLip := findSeries(report, "lip_sync_playback")
	if playbackLip != nil {
		return playbackLip
	}
	if len(report.Series) > 0 {
		return &report.Series[0]
	}
	return nil
}

func earliestSeriesCause(report *Report) *FrameRef {
	var result *FrameRef
	for i := range report.Series {
		series := &report.Series[i]
		if !isAbnormalKind(series.Kind) {
			continue
		}
		frame := firstSeriesThresholdFrame(series)
		if frame == nil {
			value := series.FirstFrame
			frame = &value
		}
		if result == nil || *frame < result.Frame ||
			(*frame == result.Frame && stageRank(series.Stage) < stageRank(result.Stage)) {
			result = &FrameRef{
				Frame:  *frame,
				Stage:  series.Stage,
				Series: series.Name,
				Kind:   series.Kind,
			}
		}
	}
	return result
}

func firstSeriesThresholdFrame(series *SeriesReport) *int64 {
	var result *int64
	if series.FirstAbsoluteOffsetFrame != nil {
		value := *series.FirstAbsoluteOffsetFrame
		result = &value
	}
	if series.FirstChangeFrame != nil &&
		(result == nil || *series.FirstChangeFrame < *result) {
		value := *series.FirstChangeFrame
		result = &value
	}
	return result
}

func isAbnormalKind(kind string) bool {
	switch kind {
	case KindFixedOffset, KindLinearDrift, KindLocalJump, KindFrameSkip, KindIrregular:
		return true
	default:
		return false
	}
}

func kindLabel(kind string) string {
	switch kind {
	case KindFixedOffset:
		return "固定偏移"
	case KindLinearDrift:
		return "线性漂移"
	case KindLocalJump:
		return "局部阶跃/跳帧"
	case KindFrameSkip:
		return "局部跳帧"
	case KindInSync:
		return "同步正常"
	case KindIrregular:
		return "不规则抖动"
	default:
		return "证据不足"
	}
}

func formatSummary(report *Report) string {
	if report.FirstAbnormalFrame == nil {
		return "未发现超过阈值的异常帧。"
	}
	first := report.FirstAbnormalFrame
	at := time.Duration(float64(first.Frame) * float64(report.Options.FrameDuration))
	text := fmt.Sprintf("首个异常帧 frame=%d @ %s，阶段=%s，形态=%s",
		first.Frame, at.Round(time.Millisecond), first.Stage, kindLabel(report.Classification))
	if report.EstimatedSkippedFrames > 0 {
		text += fmt.Sprintf("，约 %d 帧", report.EstimatedSkippedFrames)
	}
	if report.ObservedFinalOffsetNS != 0 {
		ms := float64(report.ObservedFinalOffsetNS) / float64(time.Millisecond)
		direction := "音频超前/口型偏慢"
		if report.ObservedFinalOffsetNS < 0 {
			direction = "音频滞后/口型偏快"
		}
		text += fmt.Sprintf("；最终唇音偏移 %.1f ms（%s）", ms, direction)
	}
	return text
}
