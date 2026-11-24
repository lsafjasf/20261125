package cbarcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// WriteJSON emits deterministic, indented JSON with a trailing newline.
func WriteJSON(w io.Writer, report *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(report)
}

// Markdown renders the human-readable summary. The JSON report remains the
// complete source of truth, including all 18 candidates for every region.
func Markdown(report *Report, title string) string {
	var b strings.Builder
	if title == "" {
		title = "彩条/灰阶合规报告"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- 帧：`%dx%d`，存储位深 `%d-bit`\n", report.FrameWidth, report.FrameHeight, report.StorageDepth)
	if report.Source != "" {
		fmt.Fprintf(&b, "- 来源：`%s`\n", report.Source)
	}
	fmt.Fprintf(&b, "- 目标：`%s`；阈值：亮度 `%.2f`、色度 `%.2f` 个 8-bit 码值\n",
		report.Target, report.Thresholds.Luma, report.Thresholds.Chroma)
	fmt.Fprintf(&b, "- 候选最佳：`%s`，得分 `%.3f`；目标得分 `%.3f`\n",
		report.Summary.Best, report.Summary.BestScore, report.Summary.TargetScore)
	status := "PASS"
	if !report.Summary.Compliant {
		status = "FAIL"
	}
	fmt.Fprintf(&b, "- 结论：**%s**；亮度违规 `%d` 像素，色度违规 `%d` 像素\n\n",
		status, report.Summary.LumaViolationPixels, report.Summary.ChromaViolationPixels)

	if v := report.FirstViolation; v != nil {
		fmt.Fprintf(&b, "## 首个违规像素\n\n")
		fmt.Fprintf(&b, "`(x=%d, y=%d)`，区域 `%s`，原因 `%s`。\n\n",
			v.X, v.Y, v.Region, strings.Join(v.Reasons, ", "))
		fmt.Fprintf(&b, "| 项 | Y | Cb | Cr |\n|---|---:|---:|---:|\n")
		fmt.Fprintf(&b, "| 实测码值 | %.0f | %.0f | %.0f |\n", v.Observed.Y, v.Observed.Cb, v.Observed.Cr)
		fmt.Fprintf(&b, "| 目标码值 | %.0f | %.0f | %.0f |\n", v.Expected.Y, v.Expected.Cb, v.Expected.Cr)
		fmt.Fprintf(&b, "| 8-bit 等效误差 | %.2f | %.2f | %.2f |\n", v.ErrorCode8.Y, v.ErrorCode8.Cb, v.ErrorCode8.Cr)
		fmt.Fprintf(&b, "| 合法码域 | %.0f..%.0f | %.0f..%.0f | %.0f..%.0f |\n\n",
			v.Bounds.YMin, v.Bounds.YMax, v.Bounds.CbMin, v.Bounds.CbMax, v.Bounds.CrMin, v.Bounds.CrMax)
	} else {
		fmt.Fprintf(&b, "## 首个违规像素\n\n无。\n\n")
	}
	if v := report.FirstChromaViolation; v != nil {
		fmt.Fprintf(&b, "首个色度违规：`(x=%d, y=%d)`，区域 `%s`，原因 `%s`，实测 Cb/Cr `%.0f/%.0f`，目标 `%.0f/%.0f`。\n\n",
			v.X, v.Y, v.Region, strings.Join(v.Reasons, ", "),
			v.Observed.Cb, v.Observed.Cr, v.Expected.Cb, v.Expected.Cr)
	}

	fmt.Fprintf(&b, "## 逐区域比较\n\n")
	fmt.Fprintf(&b, "| 区域 | 像素 | 目标分 | 最佳配置 | 最佳分 | ΔY | ΔCb | ΔCr | 违规/越界 | 首坏像素 |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---|---:|---:|---:|---:|---:|---|\n")
	for _, r := range report.Regions {
		state := "ok"
		if !r.TargetEquivalentBest {
			state = "config?"
		}
		if r.FirstViolation != nil {
			state = fmt.Sprintf("(%d,%d)", r.FirstViolation.X, r.FirstViolation.Y)
		}
		fmt.Fprintf(&b, "| %s | %d | %.2f | `%s` | %.2f | %.2f | %.2f | %.2f | L%d/C%d | %s |\n",
			r.Name, r.Pixels, r.TargetScore, r.Best, r.BestScore,
			r.MeanError8.Y, r.MeanError8.Cb, r.MeanError8.Cr,
			r.LumaViolationPixels, r.ChromaViolationPixels, state)
	}

	fmt.Fprintf(&b, "\n## 全局候选（逐项比较 range / matrix / depth）\n\n")
	fmt.Fprintf(&b, "| 排名 | 配置 | 得分 | ΔY | ΔCb | ΔCr | Y越界 | C越界 | 等效最佳 |\n")
	fmt.Fprintf(&b, "|---:|---|---:|---:|---:|---:|---:|---:|---|\n")
	for _, c := range report.GlobalCandidates {
		fmt.Fprintf(&b, "| %d | `%s` | %.3f | %.3f | %.3f | %.3f | %d | %d | %s |\n",
			c.Rank, c.Config, c.Score, c.MeanLumaError8, c.MeanCbError8, c.MeanCrError8,
			c.IllegalLumaPixels, c.IllegalChromaPixels, yesNo(c.EquivalentToBest))
	}
	return b.String()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
