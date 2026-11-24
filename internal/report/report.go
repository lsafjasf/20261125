// Package report writes the human- and machine-readable audit outputs.
package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"

	"logshoulder/internal/display"
	"logshoulder/internal/pipeline"
)

// Conversions runs all exposure samples.
func Conversions(shoulder display.Shoulder) ([]pipeline.Conversion, error) {
	cases := pipeline.Cases()
	out := make([]pipeline.Conversion, 0, len(cases))
	for _, c := range cases {
		conv, err := pipeline.Run(c, shoulder)
		if err != nil {
			return nil, err
		}
		out = append(out, conv)
	}
	return out, nil
}

// WriteMarkdown writes the visual audit narrative.
func WriteMarkdown(w io.Writer, convs []pipeline.Conversion, shoulder display.Shoulder) error {
	if _, err := fmt.Fprintln(w, "# Log 高光截断审计"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n## 结论"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n高光截断发生在 **显示转换入口的 legal-white 归一化**，不是相机 Log 编码端。旧路径先把 `code / (940/1023)` 钳到 1，再套 sRGB；有效 Log 码在进入反 Log 之前就已被压成同一个显示白。暗部抬起同源于把 Log 码当作显示线性值，而不是先做反 Log。"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n修复路径保留 `INVERSE_LOG -> scene-linear -> shoulder -> sRGB` 的顺序：低于肩部拐点的中间调逐值恒等；拐点以上用有理肩部压入可用 SDR 余量，仍保持严格单调。源端已达到 code 1.0 的样本会单独标成不可恢复，不会被“恢复”成虚假层次。"); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "\n## 曲线反证"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n| 曲线 | 18% 灰码 | legal white 码 | legal white 场景 | code 1.0 场景 |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, c := range convs {
		info := c.Info
		if _, err := fmt.Fprintf(w, "| %s | %.5f | %.5f | %.3fx | %.3fx |\n",
			info.ShortName, info.MidGrayCode, info.LegalWhiteCode, info.LegalWhiteScene, info.CeilingScene); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "\n肩部参数：knee `%.2f`，output ceiling `%.2f`，标定点 `%.1fx -> %.3f`，有理尺度 `%.4f`。输出上限不参与硬钳，曲线渐近但低于该上限。\n",
		shoulder.Knee, shoulder.Ceiling, shoulder.CalibrationInput, shoulder.CalibrationOutput, shoulder.Scale()); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "\n## 三组曝光对照"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n| 样本 | EV | 源端截断 | 旧路径早钳 | 源高光层数 | 旧层数 | 修复层数 | 旧末端间隔 | 修复末端间隔 | 旧窗顶 | 修复窗顶 |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, c := range convs {
		f := c.Diagnose()
		m := c.Measure()
		if _, err := fmt.Fprintf(w, "| %s | %+.0f | %d | %d/%d | %d | %d | %d | %.6f | %.6f | %.4f | %.4f |\n",
			c.Case.Name, c.Case.EV, f.SourceClips, f.PrematureClips, len(c.Case.Highlights),
			m.SourceHighlightLevels, m.LegacyHighlightLevels, m.FixedHighlightLevels,
			m.LegacyLastGap, m.FixedLastGap, m.LegacyWindowMaxCode, m.FixedWindowMaxCode); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\n## 中间调与暗部约束"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n| 样本 | 旧暗部抬升 | 修复暗部误差 | 旧中间调误差 | 修复中间调误差 | 修复高光严格单调 |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, c := range convs {
		m := c.Measure()
		if _, err := fmt.Fprintf(w, "| %s | %+.4f | %.9f | %+.4f | %.9f | %t |\n",
			c.Case.Name, m.LegacyShadowLift, m.FixedShadowError, m.LegacyMidtoneError,
			m.FixedMidtoneError, m.FixedStrictlyOrdered); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\n## 逐样本判断"); err != nil {
		return err
	}
	for _, c := range convs {
		f := c.Diagnose()
		if _, err := fmt.Fprintf(w, "\n- **%s**：`%s`。%s。首个被早钳的源码 `%.5f`，最后一个仍有值的源码 `%.5f`；曲线在 legal white 以上仍解出 **%d** 个不同场景层次。",
			c.Case.Name, f.Stage, c.Explain(), f.FirstClippedCode, f.LastSourceCode, f.RecoverableSceneLevels); err != nil {
			return err
		}
		if f.Explanation != "" {
			if _, err := fmt.Fprintf(w, "证据：%s。", f.Explanation); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	return nil
}

// WriteMetricsCSV writes one row per exposure sample.
func WriteMetricsCSV(w io.Writer, convs []pipeline.Conversion) error {
	cw := csv.NewWriter(w)
	header := []string{
		"case", "curve", "ev", "legal_white_code", "legal_white_scene",
		"encoder_ceiling_scene", "premature_clips", "source_clips",
		"source_highlight_levels", "legacy_highlight_levels", "fixed_highlight_levels",
		"legacy_clipped_fraction", "legacy_highlight_span", "fixed_highlight_span",
		"legacy_last_gap", "fixed_last_gap", "legacy_window_max", "fixed_window_max",
		"legacy_shadow_lift", "fixed_shadow_error", "legacy_midtone_error",
		"fixed_midtone_error", "fixed_strictly_ordered",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, c := range convs {
		f := c.Diagnose()
		m := c.Measure()
		row := []string{
			c.Case.Name, c.Info.ShortName, format(c.Case.EV), format(c.Info.LegalWhiteCode),
			format(c.Info.LegalWhiteScene), format(c.Info.CeilingScene),
			strconv.Itoa(f.PrematureClips), strconv.Itoa(f.SourceClips),
			strconv.Itoa(m.SourceHighlightLevels), strconv.Itoa(m.LegacyHighlightLevels),
			strconv.Itoa(m.FixedHighlightLevels), format(m.LegacyClippedFraction),
			format(m.LegacyHighlightSpan), format(m.FixedHighlightSpan),
			format(m.LegacyLastGap), format(m.FixedLastGap),
			format(m.LegacyWindowMaxCode), format(m.FixedWindowMaxCode),
			format(m.LegacyShadowLift), format(m.FixedShadowError),
			format(m.LegacyMidtoneError), format(m.FixedMidtoneError),
			strconv.FormatBool(m.FixedStrictlyOrdered),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteTraceCSV records every stage for every patch.
func WriteTraceCSV(w io.Writer, convs []pipeline.Conversion) error {
	cw := csv.NewWriter(w)
	header := []string{
		"case", "curve", "patch", "role", "scene_linear", "encoded_code",
		"clamped_code", "source_clipped", "decoded_scene", "legacy_normalized",
		"legacy_display_code", "fixed_display_linear", "fixed_display_code",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, c := range convs {
		for _, row := range c.Rows {
			rec := []string{
				c.Case.Name, c.Info.ShortName, row.Label, string(row.Role),
				format(row.Scene), format(row.RawCode), format(row.Code),
				strconv.FormatBool(row.SourceClipped), format(row.DecodedScene),
				format(row.LegacyNormalized), format(row.LegacyDisplayCode),
				format(row.FixedDisplayLinear), format(row.FixedDisplayCode),
			}
			if err := cw.Write(rec); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

func format(v float64) string {
	if math.IsNaN(v) {
		return "NaN"
	}
	return strconv.FormatFloat(v, 'f', 9, 64)
}
