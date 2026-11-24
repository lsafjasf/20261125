package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"rec2020-to-rec709-shoulder/internal/pipeline"
)

type Report struct {
	Title               string                  `json:"title"`
	CreatedAt           time.Time               `json:"created_at"`
	Input               string                  `json:"input"`
	FrameCount          int                     `json:"frame_count"`
	Width               int                     `json:"width"`
	Height              int                     `json:"height"`
	InputTransfer       string                  `json:"input_transfer"`
	SignalRange         string                  `json:"signal_range"`
	OutputTransfer      string                  `json:"output_transfer"`
	FixedKnee           float64                 `json:"fixed_knee"`
	HuePreserveShoulder bool                    `json:"hue_preserve_shoulder"`
	Assumptions         []string                `json:"assumptions"`
	AdaptiveRules       []string                `json:"adaptive_rules"`
	Summaries           []VariantSummary        `json:"summaries"`
	Decisions           []pipeline.Decision     `json:"decisions"`
	FrameMetrics        []pipeline.FrameMetrics `json:"frame_metrics"`
}

type VariantSummary struct {
	Variant                   string  `json:"variant"`
	Frames                    int     `json:"frames"`
	HueMeanAbsDegrees         float64 `json:"hue_mean_abs_degrees"`
	HueP95AbsDegrees          float64 `json:"hue_p95_abs_degrees"`
	HueWorstMaxDegrees        float64 `json:"hue_worst_max_degrees"`
	HueOver10Pct              float64 `json:"hue_over_10deg_pct"`
	DesaturatedPct            float64 `json:"desaturated_pct"`
	MeanDeltaE76              float64 `json:"mean_delta_e76"`
	OutputMeanLuma            float64 `json:"output_mean_luma"`
	OutputMeanLabL            float64 `json:"output_mean_lab_l"`
	OutputP95Luma             float64 `json:"output_p95_luma"`
	OutputClipPct             float64 `json:"output_clip_pct"`
	GlobalTextureRetention    float64 `json:"global_texture_retention"`
	HighlightTextureRetention float64 `json:"highlight_texture_retention"`
	SevereInteriorRetention   float64 `json:"severe_interior_texture_retention"`
	ClippedTextureRetention   float64 `json:"clipped_texture_retention"`
}

func Summarize(metrics []pipeline.FrameMetrics) []VariantSummary {
	byVariant := map[string][]pipeline.FrameMetrics{}
	for _, m := range metrics {
		byVariant[m.Variant] = append(byVariant[m.Variant], m)
	}
	order := []string{pipeline.VariantHard, pipeline.VariantFixed, pipeline.VariantAdaptive}
	var out []VariantSummary
	for _, name := range order {
		frames := byVariant[name]
		if len(frames) == 0 {
			continue
		}
		s := VariantSummary{Variant: name, Frames: len(frames)}
		var worstHue float64
		for _, m := range frames {
			s.HueMeanAbsDegrees += m.HueMeanAbsDegrees
			s.HueP95AbsDegrees += m.HueP95AbsDegrees
			s.HueOver10Pct += m.HueOver10Pct
			s.DesaturatedPct += m.DesaturatedPct
			s.MeanDeltaE76 += m.MeanDeltaE76
			s.OutputMeanLuma += m.OutputMeanLuma
			s.OutputMeanLabL += m.OutputMeanLabL
			s.OutputP95Luma += m.OutputP95Luma
			s.OutputClipPct += m.OutputClipPct
			s.GlobalTextureRetention += m.GlobalTextureRetention
			s.HighlightTextureRetention += m.HighlightTextureRetention
			s.SevereInteriorRetention += m.SevereInteriorRetention
			s.ClippedTextureRetention += m.ClippedTextureRetention
			worstHue = max(worstHue, m.HueMaxAbsDegrees)
		}
		n := float64(len(frames))
		s.HueMeanAbsDegrees /= n
		s.HueP95AbsDegrees /= n
		s.HueWorstMaxDegrees = worstHue
		s.HueOver10Pct /= n
		s.DesaturatedPct /= n
		s.MeanDeltaE76 /= n
		s.OutputMeanLuma /= n
		s.OutputMeanLabL /= n
		s.OutputP95Luma /= n
		s.OutputClipPct /= n
		s.GlobalTextureRetention /= n
		s.HighlightTextureRetention /= n
		s.SevereInteriorRetention /= n
		s.ClippedTextureRetention /= n
		out = append(out, roundSummary(s))
	}
	return out
}

func WriteFiles(outDir string, r Report) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	jsonPath := filepath.Join(outDir, "report.json")
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return err
	}
	if err := writeCSV(filepath.Join(outDir, "frame_metrics.csv"), r.FrameMetrics); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "report.md"), []byte(Markdown(r)), 0o644)
}

func writeCSV(path string, metrics []pipeline.FrameMetrics) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w := csv.NewWriter(file)
	header := []string{"frame", "variant", "hue_mean_abs", "hue_p95", "hue_max", "hue_over10_pct", "desat_pct", "delta_e76", "mean_luma", "clip_pct", "global_texture_ret", "highlight_texture_ret", "severe_interior_ret", "clipped_texture_ret"}
	if err := w.Write(header); err != nil {
		return err
	}
	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].Frame == metrics[j].Frame {
			return metrics[i].Variant < metrics[j].Variant
		}
		return metrics[i].Frame < metrics[j].Frame
	})
	for _, m := range metrics {
		row := []string{
			fmt.Sprint(m.Frame), m.Variant,
			fmt.Sprintf("%.4f", m.HueMeanAbsDegrees), fmt.Sprintf("%.4f", m.HueP95AbsDegrees), fmt.Sprintf("%.4f", m.HueMaxAbsDegrees),
			fmt.Sprintf("%.3f", m.HueOver10Pct), fmt.Sprintf("%.3f", m.DesaturatedPct), fmt.Sprintf("%.3f", m.MeanDeltaE76),
			fmt.Sprintf("%.5f", m.OutputMeanLuma), fmt.Sprintf("%.4f", m.OutputClipPct),
			fmt.Sprintf("%.4f", m.GlobalTextureRetention), fmt.Sprintf("%.4f", m.HighlightTextureRetention),
			fmt.Sprintf("%.4f", m.SevereInteriorRetention), fmt.Sprintf("%.4f", m.ClippedTextureRetention),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func Markdown(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.Title)
	fmt.Fprintf(&b, "- 输入：`%s`；%d 帧，%dx%d\n", r.Input, r.FrameCount, r.Width, r.Height)
	fmt.Fprintf(&b, "- 输入传输曲线：`%s`；电平：`%s`；输出：Rec.709 + `%s`\n", r.InputTransfer, r.SignalRange, r.OutputTransfer)
	fmt.Fprintf(&b, "- 固定肩拐点：线性 RGB `%.2f`；肩函数在拐点 C1 连续并渐近到 1\n", r.FixedKnee)
	if r.HuePreserveShoulder {
		b.WriteString("- 肩曲线模式：max-RGB 比例压缩，保持 RGB 通道比例/色相；可用 `-hue-preserve=false` 对照逐通道软肩\n\n")
	} else {
		b.WriteString("- 肩曲线模式：逐通道压缩，可观察通道比例变化导致的色相漂移；默认建议 `-hue-preserve=true`\n\n")
	}

	b.WriteString("## 处理假设\n")
	for _, a := range r.Assumptions {
		fmt.Fprintf(&b, "- %s\n", a)
	}
	b.WriteString("\n## 自适应触发与防抖\n")
	for _, rule := range r.AdaptiveRules {
		fmt.Fprintf(&b, "- %s\n", rule)
	}

	b.WriteString("\n## 三版汇总\n\n")
	b.WriteString("| 版本 | 平均色相偏移° | P95色相° | 最差色相° | >10°占比% | 去饱和% | 平均L* | 硬峰值% | 全局纹理 | 高光纹理 | 严重高光内部纹理 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, s := range r.Summaries {
		fmt.Fprintf(&b, "| `%s` | %.2f | %.2f | %.1f | %.2f | %.2f | %.1f | %.2f | %.3f | %.3f | %.3f |\n",
			s.Variant, s.HueMeanAbsDegrees, s.HueP95AbsDegrees, s.HueWorstMaxDegrees, s.HueOver10Pct,
			s.DesaturatedPct, s.OutputMeanLabL, s.OutputClipPct, s.GlobalTextureRetention,
			s.HighlightTextureRetention, s.SevereInteriorRetention)
	}

	b.WriteString("\n## 逐帧自适应决策\n\n")
	b.WriteString("| 帧 | 场景 | knee | pressure | 超域% | P99.5 | 高光面积% | 纹理比 | cut | 触发摘要 |\n")
	b.WriteString("|---:|---|---:|---:|---:|---:|---:|---:|---:|---|\n")
	for _, d := range r.Decisions {
		summary := strings.Join([]string{}, "")
		if d.Triggers.OverRange {
			summary += "超域;"
		}
		if d.Triggers.Headroom {
			summary += "峰值;"
		}
		if d.Triggers.HighlightArea {
			summary += "面积;"
		}
		if d.Triggers.HighlightTexture {
			summary += "纹理;"
		}
		if summary == "" {
			summary = "未达显式阈值"
		}
		fmt.Fprintf(&b, "| %d | %s | %.3f | %.2f | %.2f | %.3f | %.1f | %.2f | %.2f | %s |\n",
			d.Frame, profileLabel(d.Profile), d.Knee, d.Pressure, d.Triggers.OverRangePct, d.Triggers.P995Max,
			d.Triggers.HighlightAreaPct, d.Triggers.DetailRatio, d.CutScore, summary)
	}

	b.WriteString("\n## 输出与复现\n")
	b.WriteString("- 三版 PNG 序列：`hard_clip/`、`fixed_shoulder/`、`adaptive_shoulder/`\n")
	b.WriteString("- 对比图：`contact_compare.png`、`hue_shift_map.png`、`texture_map.png`、`clip_map.png`\n")
	b.WriteString("- 完整机器可读结果：`report.json`；逐帧指标：`frame_metrics.csv`\n")
	b.WriteString("- 可选 MP4：`*_rec709.mp4` 与 `three_way_compare_rec709.mp4`（若已用 ffmpeg 编码）\n")
	b.WriteString("- MP4 编码示例：`ffmpeg -framerate 30 -i adaptive_shoulder/frame_%04d.png -c:v libx264 -pix_fmt yuv420p -color_primaries bt709 -color_trc bt709 -colorspace bt709 adaptive.mp4`\n")
	return b.String()
}

func profileLabel(profile string) string {
	switch profile {
	case pipeline.ProfileTexture:
		return "纹理保护"
	case pipeline.ProfileSpecular:
		return "镜面保护"
	default:
		return "均衡"
	}
}

func roundSummary(s VariantSummary) VariantSummary {
	s.HueMeanAbsDegrees = round(s.HueMeanAbsDegrees, 3)
	s.HueP95AbsDegrees = round(s.HueP95AbsDegrees, 3)
	s.HueWorstMaxDegrees = round(s.HueWorstMaxDegrees, 3)
	s.HueOver10Pct = round(s.HueOver10Pct, 3)
	s.DesaturatedPct = round(s.DesaturatedPct, 3)
	s.MeanDeltaE76 = round(s.MeanDeltaE76, 3)
	s.OutputMeanLuma = round(s.OutputMeanLuma, 5)
	s.OutputMeanLabL = round(s.OutputMeanLabL, 2)
	s.OutputP95Luma = round(s.OutputP95Luma, 5)
	s.OutputClipPct = round(s.OutputClipPct, 4)
	s.GlobalTextureRetention = round(s.GlobalTextureRetention, 4)
	s.HighlightTextureRetention = round(s.HighlightTextureRetention, 4)
	s.SevereInteriorRetention = round(s.SevereInteriorRetention, 4)
	s.ClippedTextureRetention = round(s.ClippedTextureRetention, 4)
	return s
}

func round(v float64, digits int) float64 {
	p := float64(1)
	for i := 0; i < digits; i++ {
		p *= 10
	}
	return float64(int64(v*p+0.5)) / p
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
