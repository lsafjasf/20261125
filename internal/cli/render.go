package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"avsyncdiag/internal/sync"
)

func renderText(report *sync.Report) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "结论：%s\n", report.Summary)
	fmt.Fprintf(&builder, "分类：%s（置信度 %s）\n",
		kindLabelCN(report.Classification), report.Confidence)

	if report.FirstRootCauseFrame != nil {
		ref := report.FirstRootCauseFrame
		fmt.Fprintf(&builder, "根因首帧：frame=%d time=%s stage=%s series=%s kind=%s\n",
			ref.Frame, frameTime(ref.Frame, report.Options), ref.Stage, ref.Series,
			kindLabelCN(ref.Kind))
	}
	if report.FirstPlaybackAnomalyFrame != nil {
		ref := report.FirstPlaybackAnomalyFrame
		fmt.Fprintf(&builder, "播放可见首帧：frame=%d time=%s series=%s\n",
			ref.Frame, frameTime(ref.Frame, report.Options), ref.Series)
	}
	if report.EstimatedDriftNSPerFrame != 0 {
		fmt.Fprintf(&builder, "估计漂移：%.1f ns/frame（%.3f ms/s）\n",
			report.EstimatedDriftNSPerFrame,
			report.EstimatedDriftNSPerFrame*report.Options.FPS/float64(time.Millisecond))
	}
	if report.EstimatedOnsetFrame != nil {
		fmt.Fprintf(&builder, "外推漂移起点：frame=%d time=%s\n",
			*report.EstimatedOnsetFrame, frameTime(*report.EstimatedOnsetFrame, report.Options))
	}

	builder.WriteString("\n阶段轨迹：\n")
	w := tabwriter.NewWriter(&builder, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "stage\tsamples\tframes\tfirst_pos_ms\tlast_pos_ms\tjumps")
	for _, summary := range report.StageSummaries {
		fmt.Fprintf(w, "%s\t%d\t%d-%d\t%s\t%s\t%d\n",
			summary.Stage,
			summary.Samples,
			summary.FirstFrame,
			summary.LastFrame,
			nsPtrMS(summary.FirstPositionNS),
			nsPtrMS(summary.LastPositionNS),
			summary.PositionJumps)
	}
	_ = w.Flush()

	builder.WriteString("\n偏移序列：\n")
	w = tabwriter.NewWriter(&builder, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "series\tkind\tconf\tfirst\tinitial_ms\tfinal_ms\tslope_us/frame\tr2\tjump_frame")
	for _, series := range report.Series {
		jumpFrame := "-"
		if series.JumpFrame != nil {
			jumpFrame = fmt.Sprintf("%d", *series.JumpFrame)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%.3f\t%.3f\t%.3f\t%.3f\t%s\n",
			series.Name,
			kindLabelCN(series.Kind),
			series.Confidence,
			series.FirstFrame,
			float64(series.InitialMedianNS)/1e6,
			float64(series.FinalMedianNS)/1e6,
			series.SlopeNSPerFrame/1000,
			series.R2,
			jumpFrame)
	}
	_ = w.Flush()

	if len(report.Anomalies) > 0 {
		builder.WriteString("\n异常事件（最多 10 条）：\n")
		limit := 10
		if len(report.Anomalies) < limit {
			limit = len(report.Anomalies)
		}
		for _, anomaly := range report.Anomalies[:limit] {
			fmt.Fprintf(&builder, "- frame=%d time=%s stage=%s kind=%s jump=%.3fms missing=%d %s\n",
				anomaly.Frame,
				frameTime(anomaly.Frame, report.Options),
				anomaly.Stage,
				kindLabelCN(anomaly.Kind),
				float64(anomaly.JumpNS)/1e6,
				anomaly.MissingFrames,
				anomaly.Series)
		}
	}

	if len(report.EvidenceRows) > 0 {
		builder.WriteString("\n首异常前后媒体位置（ms；lip=audio-video，正为口型偏慢）：\n")
		w = tabwriter.NewWriter(&builder, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "frame\ttime\tcapture\tpts\tedit\trender\tplayback\tplay_lip")
		for _, row := range report.EvidenceRows {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				row.Frame,
				frameTime(row.Frame, report.Options),
				positionCell(row, sync.StageCapture),
				positionCell(row, sync.StagePTS),
				positionCell(row, sync.StageEdit),
				positionCell(row, sync.StageRender),
				positionCell(row, sync.StagePlayback),
				lipCell(row, sync.StagePlayback))
		}
		_ = w.Flush()
	}

	return builder.String()
}

func frameTime(frame int64, options sync.Options) string {
	return (time.Duration(frame) * options.FrameDuration).Round(time.Millisecond).String()
}

func nsPtrMS(value *int64) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%.3f", float64(*value)/1e6)
}

func positionCell(row sync.PositionRow, stage sync.Stage) string {
	value, ok := row.Positions[stage]
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%.3f", float64(value)/1e6)
}

func lipCell(row sync.PositionRow, stage sync.Stage) string {
	value, ok := row.LipOffset[stage]
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%+.3f", float64(value)/1e6)
}

func kindLabelCN(kind string) string {
	switch kind {
	case sync.KindFixedOffset:
		return "固定偏移"
	case sync.KindLinearDrift:
		return "线性漂移"
	case sync.KindFrameSkip:
		return "局部跳帧"
	case sync.KindLocalJump:
		return "局部阶跃"
	case sync.KindInSync:
		return "同步正常"
	case sync.KindIrregular:
		return "不规则"
	case sync.KindInsufficient:
		return "证据不足"
	default:
		return kind
	}
}
