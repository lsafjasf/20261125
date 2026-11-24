package mixloud

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func WriteMarkdown(path string, report *Report) error {
	return os.WriteFile(path, []byte(RenderMarkdown(report)), 0o644)
}

func RenderMarkdown(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 混音短时响度核对报告\n\n")
	fmt.Fprintf(&b, "- 输入：`%s`\n", r.Input)
	if len(r.Schemes) > 0 {
		m := r.Schemes[0].Metrics
		fmt.Fprintf(&b, "- 时长：%s（%.3f s）；采样率：%d Hz；声道：%d\n", timecode(m.DurationSec), m.DurationSec, m.SampleRate, m.Channels)
		fmt.Fprintf(&b, "- 窗口：%d ms；步进：%d ms；窗数：%d\n", m.WindowMS, m.HopMS, m.WindowCount)
	}
	for _, note := range r.Notes {
		fmt.Fprintf(&b, "- 口径：%s\n", note)
	}
	b.WriteString("\n## 三方案对比\n\n")
	b.WriteString("| 方案 | 整合 LUFS | LRA400 | LRA3s | 最大400ms LUFS | 最大峰值 dBFS | 满刻度样本 | 对白占比/总时长 | 对白占比/有声 | 静音 | 广告突入 | 反相 | 爆音 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, sr := range r.Schemes {
		m := sr.Metrics
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %d | %.1f%% | %.1f%% | %d | %d | %d | %d |\n",
			m.Label, mdLU(m.IntegratedLUFS), mdLU(m.LRA400MS), mdLU(m.LRAEBU3S), mdLU(m.MaxWindowLUFS),
			mdDB(m.MaxPeakDBFS), m.ClippedSamples, 100*m.DialogueRatioTotal, 100*m.DialogueRatioActive,
			len(sr.SilenceSegments), len(sr.AdIntrusions), len(sr.PhaseInversions), len(sr.BurstEvents))
	}
	renderStemTable(&b, r.Stems)

	b.WriteString("\n## 交付判断\n\n")
	verdicts := r.Verdicts
	if verdicts == nil {
		verdicts = conclusions(r)
	}
	for _, line := range verdicts {
		fmt.Fprintf(&b, "- %s\n", line)
	}

	for _, sr := range r.Schemes {
		renderScheme(&b, sr)
	}
	return b.String()
}

func renderStemTable(b *strings.Builder, stems []SchemeReport) {
	if len(stems) == 0 {
		return
	}
	b.WriteString("\n## 对白/音乐/效果声分轨\n\n")
	b.WriteString("| 分轨 | 整合 LUFS | LRA400 | 最大400ms LUFS | 最大峰值 dBFS | 对白/活动占比 | 窗数 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
	for _, sr := range stems {
		m := sr.Metrics
		ratio := 100 * m.DialogueRatioTotal
		if m.Scheme != "stem_dialogue" {
			ratio = 100 * m.ActiveDurationSec / m.DurationSec
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %.1f%% | %d |\n",
			m.Label, mdLU(m.IntegratedLUFS), mdLU(m.LRA400MS), mdLU(m.MaxWindowLUFS), mdDB(m.MaxPeakDBFS), ratio, m.WindowCount)
	}
}

func conclusions(r *Report) []string {
	var out []string
	for _, sr := range r.Schemes {
		m := sr.Metrics
		switch m.Scheme {
		case "bypass":
			if len(sr.PhaseInversions) > 0 {
				out = append(out, "不处理存在双声道反相段，单声道兼容性风险必须先修复。")
			}
			if m.ClippedSamples > 0 || (m.MaxPeakDBFS != nil && *m.MaxPeakDBFS >= -0.1) {
				out = append(out, fmt.Sprintf("不处理的最大峰值为 %s，满刻度样本 %d；总时长平均值不能替代这些局部爆音检查。", mdDB(m.MaxPeakDBFS), m.ClippedSamples))
			}
			if len(sr.AdIntrusions) > 0 {
				out = append(out, fmt.Sprintf("不处理检测到 %d 段广告/突发响度侵入，需要人工确认。", len(sr.AdIntrusions)))
			}
		case "peak_limit":
			if m.MaxPeakDBFS != nil {
				out = append(out, fmt.Sprintf("峰值限制后最大峰值 %s，满刻度样本 %d；请同时用真峰值表确认目标编码的 dBTP。", mdDB(m.MaxPeakDBFS), m.ClippedSamples))
			}
		case "dialogue_priority":
			out = append(out, fmt.Sprintf("对白优先方案的对白时长占比为 %.1f%%（有声段内 %.1f%%）；分类口径：%s。", 100*m.DialogueRatioTotal, 100*m.DialogueRatioActive, m.DialogueClassification))
		}
	}
	return out
}

func renderScheme(b *strings.Builder, sr SchemeReport) {
	m := sr.Metrics
	fmt.Fprintf(b, "\n## %s（`%s`）明细\n\n", m.Label, m.Scheme)
	fmt.Fprintf(b, "- 整合响度：%s LUFS；LRA(400ms)：%s；P10/P95：%s/%s LUFS\n", mdLU(m.IntegratedLUFS), mdLU(m.LRA400MS), mdLU(m.LRA10), mdLU(m.LRA95))
	fmt.Fprintf(b, "- 最响 400 ms 窗：%s LUFS；P95 窗响度：%s LUFS；中位窗响度：%s LUFS\n", mdLU(m.MaxWindowLUFS), mdLU(m.P95WindowLUFS), mdLU(m.MedianWindowLUFS))
	fmt.Fprintf(b, "- 最大采样峰值：%s dBFS；≥0.999 FS 样本：%d；≥0.99 FS 样本：%d\n", mdDB(m.MaxPeakDBFS), m.ClippedSamples, m.NearClipSamples)
	fmt.Fprintf(b, "- 对白时长：%.3f s；全曲占比：%.2f%%；有声段占比：%.2f%%\n", m.DialogueDurationSec, 100*m.DialogueRatioTotal, 100*m.DialogueRatioActive)

	renderEventTable(b, "静音段", sr.SilenceSegments, false)
	renderEventTable(b, "广告/响度突入", sr.AdIntrusions, true)
	renderPhaseTable(b, sr.PhaseInversions)
	renderEventTable(b, "局部爆音/峰值事件", sr.BurstEvents, true)
	renderTopWindows(b, sr.Windows)
}

func renderEventTable(b *strings.Builder, title string, events []Event, loud bool) {
	fmt.Fprintf(b, "\n### %s（%d）\n\n", title, len(events))
	if len(events) == 0 {
		b.WriteString("未检出。\n")
		return
	}
	if loud {
		b.WriteString("| 时间 | 时长 s | 响度 LUFS | 增量 LU | 峰值 dBFS | 峰均比 dB | 说明 |\n|---|---:|---:|---:|---:|---:|---|\n")
		for _, e := range events {
			fmt.Fprintf(b, "| %s–%s | %.3f | %s | %s | %s | %s | %s |\n",
				timecode(e.StartSec), timecode(e.EndSec), e.DurationSec,
				mdLU(e.LoudnessLUFS), mdLU(e.DeltaLU), mdDB(e.PeakDBFS), mdLU(e.CrestFactorDB), e.Message)
		}
		return
	}
	b.WriteString("| 时间 | 时长 s | 峰值 dBFS | 说明 |\n|---|---:|---:|---|\n")
	for _, e := range events {
		fmt.Fprintf(b, "| %s–%s | %.3f | %s | %s |\n", timecode(e.StartSec), timecode(e.EndSec), e.DurationSec, mdDB(e.PeakDBFS), e.Message)
	}
}

func renderPhaseTable(b *strings.Builder, events []Event) {
	fmt.Fprintf(b, "\n### 双声道反相段（%d）\n\n", len(events))
	if len(events) == 0 {
		b.WriteString("未检出。\n")
		return
	}
	b.WriteString("| 时间 | 时长 s | 最低相关系数 | 单声道兼容 dB | 说明 |\n|---|---:|---:|---:|---|\n")
	for _, e := range events {
		corr := ""
		if e.Correlation != nil {
			corr = fmt.Sprintf("%.3f", *e.Correlation)
		}
		fmt.Fprintf(b, "| %s–%s | %.3f | %s | %s | %s |\n", timecode(e.StartSec), timecode(e.EndSec), e.DurationSec, corr, mdDB(e.MonoCompatDB), e.Message)
	}
}

func renderTopWindows(b *strings.Builder, windows []Window) {
	if len(windows) == 0 {
		return
	}
	top := append([]Window(nil), windows...)
	sort.Slice(top, func(i, j int) bool {
		return ptrValue(top[i].PeakDBFS, -200) > ptrValue(top[j].PeakDBFS, -200)
	})
	if len(top) > 8 {
		top = top[:8]
	}
	b.WriteString("\n### 峰值最高的 8 个 400 ms 窗\n\n")
	b.WriteString("| 时间 | 响度 LUFS | RMS dBFS | 峰值 dBFS | 相关 | 对白分 | 削波样本 |\n|---|---:|---:|---:|---:|---:|---:|\n")
	for _, w := range top {
		corr := ""
		if w.Correlation != nil {
			corr = fmt.Sprintf("%.2f", *w.Correlation)
		}
		fmt.Fprintf(b, "| %s–%s | %s | %s | %s | %s | %.2f | %d |\n",
			timecode(w.StartSec), timecode(w.EndSec), mdLU(w.LoudnessLUFS), mdDB(w.RMSDBFS), mdDB(w.PeakDBFS), corr, w.DialogueScore, w.ClippedSamples)
	}
}

func ptrValue(p *float64, fallback float64) float64 {
	if p == nil || !finite(*p) {
		return fallback
	}
	return *p
}

func mdLU(p *float64) string {
	if p == nil || !finite(*p) {
		return "−∞"
	}
	return fmt.Sprintf("%.1f", *p)
}

func mdDB(p *float64) string {
	if p == nil || !finite(*p) {
		return "−∞"
	}
	return fmt.Sprintf("%.2f", *p)
}

func timecode(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	minutes := int(sec / 60)
	seconds := sec - float64(minutes)*60
	return fmt.Sprintf("%02d:%06.3f", minutes, seconds)
}
