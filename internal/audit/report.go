package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

func WriteJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(report)
}

func WriteMarkdown(w io.Writer, report Report) error {
	fmt.Fprintln(w, "# 范围与黑电平审计报告")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- 生成时间：`%s`\n", report.GeneratedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(w, "- 工具版本：`%s`\n", report.ToolVersion)
	fmt.Fprintf(w, "- 判定原则：%s\n", report.Policy)
	fmt.Fprintln(w)
	writeBatch(w, report.Batch)
	writeSummaryTable(w, report.Clips)
	for _, clip := range report.Clips {
		writeClip(w, clip)
	}
	return nil
}

func writeBatch(w io.Writer, batch BatchSummary) {
	fmt.Fprintln(w, "## 批次结论")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- 样片：%d，成功分析：%d，未决：%d，标签不符：%d\n", batch.Clips, batch.Analyzed, batch.Indeterminate, batch.LabelMismatches)
	fmt.Fprintf(w, "- 主导判决：`%s`\n", emptyAs(batch.DominantDecision, "n/a"))
	fmt.Fprintf(w, "- 结论：%s\n", batch.BatchConclusion)
	if len(batch.Decisions) > 0 {
		fmt.Fprintf(w, "- 判决分布：%s\n", mapText(batch.Decisions))
	}
	if len(batch.Declared) > 0 {
		fmt.Fprintf(w, "- 声明范围：%s\n", mapText(batch.Declared))
	}
	if len(batch.CorrectedLabels) > 0 {
		fmt.Fprintf(w, "- 修正标签分布：%s\n", mapText(batch.CorrectedLabels))
	}
	fmt.Fprintln(w)
}

func writeSummaryTable(w io.Writer, clips []ClipResult) {
	fmt.Fprintln(w, "## 总览")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Clip | 声明 | 观察 p0.1–p99.9 | min/max | 16/235 轨道 | 判决 | 修正标签 | 黑电平 |")
	fmt.Fprintln(w, "|---|---:|---:|---:|---:|---|---|---|")
	for _, clip := range clips {
		if clip.Error != "" {
			fmt.Fprintf(w, "| `%s` | - | - | - | - | error | %s | - |\n", clip.ID, clip.Error)
			continue
		}
		assessment := clip.Assessment
		stats := clip.Stats
		fmt.Fprintf(w, "| `%s` | %s | %d–%d | %d/%d | %.3f%% / %.3f%% | %s (%s) | %s | %s |\n",
			clip.ID,
			assessment.Declared,
			stats.Percent.P001, stats.Percent.P999,
			stats.Min, stats.Max,
			stats.Fractions.EQ16*100, stats.Fractions.EQ235*100,
			assessment.Decision, assessment.Confidence,
			assessment.CorrectedLabel,
			blackSummary(assessment),
		)
	}
	fmt.Fprintln(w)
}

func blackSummary(a Assessment) string {
	for _, h := range a.Hypotheses {
		if (a.Decision == "limited" && h.Range.Name == "limited") ||
			(a.Decision == "full" && h.Range.Name == "full") {
			return fmt.Sprintf("%s, %+d code", h.BlackLevel.Reading, h.BlackLevel.PedestalCodes)
		}
	}
	if len(a.Hypotheses) < 2 {
		return "n/a"
	}
	l := a.Hypotheses[0].BlackLevel
	f := a.Hypotheses[1].BlackLevel
	return fmt.Sprintf("limited: %s/%+d, full: %s/%+d", l.Reading, l.PedestalCodes, f.Reading, f.PedestalCodes)
}

func writeClip(w io.Writer, clip ClipResult) {
	fmt.Fprintf(w, "## %s\n", clip.ID)
	fmt.Fprintln(w)
	if clip.Error != "" {
		fmt.Fprintf(w, "- **错误**：%s\n\n", clip.Error)
		return
	}
	fmt.Fprintf(w, "- **输入**：`%s`，格式 `%s`，%d×%d，%d 帧，采样 %d 个 luma 值\n",
		clip.Input.Path, clip.Input.Format, clip.Input.Width, clip.Input.Height, clip.Input.Frames, clip.Stats.Samples)
	fmt.Fprintf(w, "- **声明标签**：`%s`；**修正标签**：`%s`；**标签状态**：`%s`\n",
		clip.Assessment.Declared, clip.Assessment.CorrectedLabel, clip.Assessment.LabelStatus)
	fmt.Fprintf(w, "- **范围判决**：`%s`（置信度 `%s`）\n", clip.Assessment.Decision, clip.Assessment.Confidence)
	fmt.Fprintf(w, "- **结论**：%s\n\n", clip.Assessment.Summary)

	fmt.Fprintln(w, "**证据**")
	fmt.Fprintln(w)
	for _, line := range clip.Assessment.Evidence {
		fmt.Fprintf(w, "- %s\n", line)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "**候选解释**")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| 假设 | 分数 | 合法范围 | 黑电平读数 | 修正标签 |")
	fmt.Fprintln(w, "|---|---:|---|---|---|")
	for _, h := range clip.Assessment.Hypotheses {
		fmt.Fprintf(w, "| `%s` | %d | %s (%d–%d) | %s | %s |\n",
			h.Key, h.Score, h.Range.Name, h.Range.Black, h.Range.White, h.BlackLevel.Compact(), h.CorrectedLabel)
	}
	fmt.Fprintln(w)

	for i, h := range clip.Assessment.Hypotheses {
		fmt.Fprintf(w, "%d. **%s 的证据（score=%d）**\n", i+1, h.CorrectedLabel, h.Score)
		for _, line := range h.Evidence {
			fmt.Fprintf(w, "   - %s\n", line)
		}
	}
	fmt.Fprintln(w)

	for i, h := range clip.Assessment.Hypotheses {
		fmt.Fprintf(w, "%d. **%s**：%s。黑电平：`%s`；风格读数：`%s`。\n",
			i+1, h.CorrectedLabel, h.Summary, h.BlackLevel.Compact(), styleText(h.BlackLevel.StyleReading))
		if len(h.BlackLevel.Alternatives) > 0 {
			fmt.Fprintf(w, "   保留解释：%s。\n", strings.Join(h.BlackLevel.Alternatives, "；"))
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**使用限制**")
	fmt.Fprintln(w)
	for _, caveat := range clip.Assessment.Caveats {
		fmt.Fprintf(w, "- %s\n", caveat)
	}
	fmt.Fprintln(w)
}

func styleText(style string) string {
	switch style {
	case "low-key-or-stylized-dark-compatible":
		return "低照度/风格化暗部兼容"
	case "ordinary-shadow-placement":
		return "普通暗部分布；不能据此证明创作意图"
	default:
		return style
	}
}

func mapText(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("`%s`=%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

func emptyAs(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
