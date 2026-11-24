package main

import (
	"fmt"
	"io"
	"strings"

	"multisourceaudit/auditor"
)

func writeTextReport(w io.Writer, report auditor.Report) {
	fmt.Fprintf(w, "媒体: %s\n", report.MediaID)

	for _, verdict := range report.Verdicts {
		fmt.Fprintf(w, "  字段 %s -> %s", verdict.Field, statusLabel(verdict.Status))
		switch verdict.Status {
		case auditor.StatusAutomatic:
			fmt.Fprintf(w, " [%s] 值=%q 来源=%s\n",
				verdict.Confidence, verdict.Value, joinSources(verdict.ValueSources))
		case auditor.StatusManual:
			fmt.Fprintf(w, " 原因=%s（不输出任何结论值）\n", verdict.Reason)
		case auditor.StatusUndetermined:
			fmt.Fprintf(w, " 原因=%s（禁止回退文件系统）\n", verdict.Reason)
		}

		fmt.Fprintf(w, "    来源证据 (%d):\n", len(verdict.Provenance))
		for _, value := range verdict.Provenance {
			flags := []string{}
			if !value.Present {
				flags = append(flags, "解析失败")
			}
			if value.Present && value.Admitted {
				flags = append(flags, "可采信")
			}
			if value.Present && !value.Admitted {
				flags = append(flags, "不可采信")
			}
			fmt.Fprintf(w, "      - %-10s 原始=%-12q 规范=%-8q [%s] %s\n",
				value.Source, value.RawValue, value.Canonical,
				strings.Join(flags, ","), value.Detail)
		}

		if len(verdict.Conflicts) == 0 {
			fmt.Fprintln(w, "    冲突路径: 无")
			continue
		}
		fmt.Fprintf(w, "    冲突路径 (%d):\n", len(verdict.Conflicts))
		for _, conflict := range verdict.Conflicts {
			tags := []string{}
			if conflict.InvolvesFilesystem {
				tags = append(tags, "含文件系统")
			}
			if conflict.IntraSource {
				tags = append(tags, "同源内冲突")
			}
			fmt.Fprintf(w, "      - %s(%s) != %s(%s) [%s]\n",
				conflict.SourceA, conflict.ValueA,
				conflict.SourceB, conflict.ValueB,
				strings.Join(tags, ","))
		}
	}
	fmt.Fprintln(w)
}

func statusLabel(status string) string {
	switch status {
	case auditor.StatusAutomatic:
		return "自动结论"
	case auditor.StatusManual:
		return "人工判定"
	case auditor.StatusUndetermined:
		return "无法确定"
	default:
		return status
	}
}

func joinSources(sources []auditor.Source) string {
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		parts = append(parts, string(source))
	}
	return strings.Join(parts, ",")
}
