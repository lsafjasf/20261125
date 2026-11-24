package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"surrounddiag/internal/analyze"
)

func WriteJSON(path string, report *analyze.Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return write(path, append(data, '\n'))
}

func WriteMarkdown(path string, report *analyze.Report) error {
	return write(path, []byte(Markdown(report)))
}

func Markdown(report *analyze.Report) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# 5.1 导出取证报告\n\n")
	fmt.Fprintf(&out, "- 文件：`%s`\n", report.File)
	if report.Reference != "" {
		fmt.Fprintf(&out, "- 参考母版：`%s`\n", report.Reference)
	} else {
		fmt.Fprintf(&out, "- 参考母版：未提供（声道交换与绝对极性只能列为候选）\n")
	}
	fmt.Fprintf(&out, "- 格式：%d Hz / %d bit / %d 声道，时长 %.3f s\n", report.SampleRate, report.Bits, report.Channels, report.Duration)
	fmt.Fprintf(&out, "- 声明顺序：%s（%s，mask %s）\n", strings.Join(report.Layout, ", "), report.LayoutSource, report.ChannelMask)
	fmt.Fprintf(&out, "- 宽声场保护：%s（FL/FR r=%+.3f，镜像证据对=%d）\n\n",
		report.WideField.Verdict, report.WideField.NegativeCorr, report.WideField.MirrorPairs)
	fmt.Fprintf(&out, "> %s\n\n", report.AnalysisNote)

	out.WriteString("## 结论\n\n")
	if len(report.Findings) == 0 {
		out.WriteString("未发现达到证据门槛的相位、映射、中心或低频异常。没有把低相关宽声场报成反相。\n\n")
	} else {
		findings := append([]analyze.Finding(nil), report.Findings...)
		sort.SliceStable(findings, func(i, j int) bool {
			if findings[i].Start != findings[j].Start {
				return findings[i].Start < findings[j].Start
			}
			return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
		})
		for _, finding := range findings {
			fmt.Fprintf(&out, "### [%s] %s · %s\n\n", severityZH(finding.Severity), timeRange(finding.Start, finding.End), strings.Join(finding.Channels, " / "))
			fmt.Fprintf(&out, "- 现象：%s\n", finding.Message)
			fmt.Fprintf(&out, "- 可能根因：%s\n", finding.RootCause)
			fmt.Fprintf(&out, "- 置信度：%.2f。%s\n\n", finding.Confidence, finding.Evidence)
		}
	}

	out.WriteString("## 逐声道分布\n\n")
	out.WriteString("| # | 声道 | RMS dBFS | 峰值 dBFS | 低频 20–120 Hz | 语音 300–3400 Hz | 高频 ≥8 kHz | 静音比例 |\n")
	out.WriteString("|---:|---|---:|---:|---:|---:|---:|---:|\n")
	for _, stats := range report.Stats {
		fmt.Fprintf(&out, "| %d | %s | %.1f | %.1f | %.1f%% | %.1f%% | %.1f%% | %.1f%% |\n",
			stats.Index+1, stats.Name, stats.RMSDB, stats.PeakDB,
			stats.LowRatio*100, stats.VoiceRatio*100, stats.HighRatio*100, stats.Silent*100)
	}
	out.WriteString("\n")
	if len(report.ChannelMap) > 0 {
		out.WriteString("## 参考映射\n\n")
		out.WriteString("| 导出声道 | 参考声道 | 状态 | 参考相对导出 samples | r | 包络 r |\n")
		out.WriteString("|---|---|---|---:|---:|---:|\n")
		for _, mapping := range report.ChannelMap {
			if mapping.ReferenceIndex < 0 {
				fmt.Fprintf(&out, "| %s | — | %s | — | — | — |\n", mapping.OutputChannel, mapping.Status)
				continue
			}
			fmt.Fprintf(&out, "| %s | %s | %s | %+d | %.3f | %.3f |\n",
				mapping.OutputChannel, mapping.ReferenceChannel, mapping.Status,
				mapping.LagSamples, mapping.Correlation, mapping.EnvelopeCorr)
		}
		out.WriteString("\n")
	}
	if len(report.Pairs) > 0 {
		out.WriteString("## 强相关声道对与样点延迟\n\n")
		out.WriteString("正延迟表示 B 晚于 A。镜像标记只在强负相关且镜像包络成立时出现。\n\n")
		out.WriteString("| A | B | 延迟 samples | 延迟 ms | r | 包络 r | 镜像证据 |\n")
		out.WriteString("|---|---|---:|---:|---:|---:|---|\n")
		for _, pair := range report.Pairs {
			mirror := "否"
			if pair.Inverted {
				mirror = "是"
			}
			fmt.Fprintf(&out, "| %s | %s | %+d | %+.3f | %.3f | %.3f | %s |\n",
				pair.AChannel, pair.BChannel, pair.MedianLag, pair.LagMS,
				pair.Correlation, pair.EnvelopeCorr, mirror)
		}
		out.WriteString("\n")
	}
	out.WriteString("## 复核命令\n\n")
	fmt.Fprintf(&out, "```sh\n./surrounddiag audition --file %s --outdir previews --play ffplay\n```\n\n", shellPath(report.File))
	out.WriteString("试听文件是逐声道单声道 24-bit PCM；左右宽声场仍应保留原始扩散感，不应因本报告的低相关项被反向处理。\n")
	return out.String()
}

func severityRank(severity string) int {
	switch severity {
	case "high":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func severityZH(severity string) string {
	switch severity {
	case "high":
		return "高"
	case "warning":
		return "中"
	default:
		return "信息"
	}
}

func timeRange(start, end float64) string {
	return fmt.Sprintf("%s–%s", timestamp(start), timestamp(end))
}

func timestamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	minutes := int(seconds) / 60
	remaining := seconds - float64(minutes*60)
	return fmt.Sprintf("%02d:%06.3f", minutes, remaining)
}

func shellPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func write(path string, data []byte) error {
	if path == "" || path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
