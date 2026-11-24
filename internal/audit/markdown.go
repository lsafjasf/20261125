package audit

import (
	"fmt"
	"strings"
)

// RenderMarkdown renders a human-readable Chinese audit report.
func RenderMarkdown(report Report) string {
	var b strings.Builder
	b.WriteString("# MOV 编辑列表与音频起点审计报告\n\n")
	b.WriteString("- 输入方式：仅以 `os.O_RDONLY` 打开；解析过程不写回、不移动、不修补输入文件。\n")
	b.WriteString("- 换算原则：整数盒字段保持原值；跨 timescale 先用有理数保存精确结果，再给出浮点秒和 floor/nearest/ceil 取整。\n")
	b.WriteString("- 适用盒：`moov/mvhd`、`moov/trak/tkhd`、`moov/trak/edts/elst`、`moov/trak/mdia/mdhd`。\n\n")

	renderFormula(&b)
	renderSummary(&b, report)
	renderCoverage(&b, report)

	for i := range report.Inputs {
		renderFile(&b, &report.Inputs[i], i)
	}
	return b.String()
}

func renderFormula(b *strings.Builder) {
	b.WriteString("## 换算公式\n\n")
	b.WriteString("设 `M = mvhd.timescale`，`D = mdhd.timescale`，第 `i` 个 edit 的 `segment_duration=S_i`、`media_time=T_i`、速率定点值为 `r_i=R_i/65536`。\n\n")
	b.WriteString("- 合成起点：`C_i = Σ(j<i) S_j`，单位为 `M`；合成区间为 `[C_i, C_i+S_i)`。\n")
	b.WriteString("- 普通/变速 edit 的精确媒体时长：`L_i = S_i × D/M × r_i`，单位为 `D`；正向播放媒体区间为 `[T_i, T_i+L_i)`。\n")
	b.WriteString("- 时间映射：`t_comp(M) = C_i + (t_media(D)-T_i) × M/(D×r_i)`；因此媒体零点映射到 `C_i - T_i×M/(D×r_i)`，该值可为负。\n")
	b.WriteString("- `T_i=-1`：标准 empty edit，只占用合成区间，不存在媒体区间；开头的 empty edit 会把后一个非空 edit 推迟其 `S/M` 秒。\n")
	b.WriteString("- `r_i=0`：ISO BMFF 定义为 dwell，整个合成区间呈现 `T_i` 这一个媒体时间点，媒体消耗长度为 0；Apple QTFF 则规定 MOV 的 media rate 不得为 0 或负数，报告同时给出兼容性告警。\n")
	b.WriteString("- `T_i<-1`：不是标准 empty edit；FFmpeg 严格模式会拒绝。本审计保留有符号计算并单独告警，避免把异常负起点吞掉。\n")
	b.WriteString("- 起点语义：empty edit 改变下一个非空 edit 的合成 anchor `C_i`；`media_rate` 不改变当前 anchor `(T_i,C_i)`，但改变媒体区间右端点、映射斜率以及 media 0 对应的合成位置。零速率时媒体区间退化为单个时间点。\n\n")
}

func renderSummary(b *strings.Builder, report Report) {
	s := report.Summary
	b.WriteString("## 汇总\n\n")
	b.WriteString("| 文件 | trak | edit | empty | zero-rate dwell | 负起点轨 | 跨 timescale 轨 | warning | error |\n")
	b.WriteString("|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	b.WriteString(fmt.Sprintf("| %d | %d | %d | %d | %d | %d | %d | %d | %d |\n\n",
		s.Files, s.Tracks, s.Edits, s.EmptyEdits, s.DwellEdits, s.NegativeStarts,
		s.CrossTimescales, s.Warnings, s.Errors))
}

func renderCoverage(b *strings.Builder, report Report) {
	b.WriteString("## 独立样例覆盖矩阵\n\n")
	b.WriteString("| 输入 | empty edit | 负合成原点/负字段 | zero-rate dwell | 跨 timescale | 非 1 速率 |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, file := range report.Inputs {
		var empty, negative, dwell, cross, rate bool
		for _, track := range file.Tracks {
			if file.Movie != nil && track.MDHD != nil && file.Movie.Timescale != track.MDHD.Timescale {
				cross = true
			}
			if track.AudioStart != nil && (track.AudioStart.NegativeTimelineOrigin || track.AudioStart.SourceMediaTimeUnits < 0) {
				negative = true
			}
			for _, edit := range track.Edits {
				empty = empty || edit.Empty
				dwell = dwell || edit.Dwell
				negative = negative || edit.NegativeMediaTime
				if edit.RateRational != "1" && edit.RateRational != "0" {
					rate = true
				}
			}
		}
		b.WriteString("| `" + file.Path + "` | " + yesNo(empty) + " | " + yesNo(negative) + " | " + yesNo(dwell) + " | " + yesNo(cross) + " | " + yesNo(rate) + " |\n")
	}
	b.WriteString("\n")
}

func yesNo(v bool) string {
	if v {
		return "是"
	}
	return "—"
}

func br(lines ...string) string {
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	return strings.Join(nonEmpty, "<br>")
}
