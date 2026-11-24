package lutdiff

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ArtifactStats carries measurements of the generated image artifacts.
type ArtifactStats struct {
	HeatScale    float64
	Gain         float64
	ImageMaxDist float64
	ImageMaxCode float64
}

// ReportData is everything the Markdown report needs.
type ReportData struct {
	GridSize int
	Samples  []SampleResult
	Cube     *CubeAnalysis
	Art      ArtifactStats
}

func f3(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
func f2(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func rgbString(c RGB) string {
	return fmt.Sprintf("(%.3f, %.3f, %.3f)", c[0], c[1], c[2])
}

func codeRGB(c RGB) string {
	return fmt.Sprintf("(%3d,%3d,%3d)",
		int(mathRound(c[0]*255)), int(mathRound(c[1]*255)), int(mathRound(c[2]*255)))
}

func mathRound(v float64) int {
	if v < 0 {
		return int(v - 0.5)
	}
	return int(v + 0.5)
}

func groupsString(g []string) string { return strings.Join(g, "/") }

// WriteSamplesCSV writes the fixed-sample measurements as CSV.
func WriteSamplesCSV(dir string, rows []SampleResult) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "samples.csv")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{
		"id", "name", "groups", "r", "g", "b",
		"AB_r", "AB_g", "AB_b", "BA_r", "BA_g", "BA_b",
		"euclid_dist", "deltaE76", "max_code_delta",
		"avg_ref_code_error", "avg_ref_deltaE76",
	})
	for _, s := range rows {
		rec := []string{
			s.ID, s.Name, groupsString(s.Groups),
			f3(s.Color[0]), f3(s.Color[1]), f3(s.Color[2]),
			f3(s.AB[0]), f3(s.AB[1]), f3(s.AB[2]),
			f3(s.BA[0]), f3(s.BA[1]), f3(s.BA[2]),
			f3(s.Dist), f2(s.DeltaE), f1(s.CodeDelta),
			f1(s.AvgRefCodeErr), f2(s.AvgRefDeltaE),
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// WriteReport renders REPORT.md into dir.
func WriteReport(dir string, d *ReportData) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	writeHeader(&b, d)
	writeSampleTable(&b, d)
	writeRegionSection(&b, d)
	writeAverageSection(&b, d)
	writeImageSection(&b, d)
	writeReproSection(&b, d)

	path := filepath.Join(dir, "REPORT.md")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeHeader(b *strings.Builder, d *ReportData) {
	fmt.Fprintf(b, "# 两个 LUT 串联顺序差异报告（A→B 对比 B→A）\n\n")
	fmt.Fprintf(b, "所有结果均由纯 Go 标准库程序生成：%d³ LUT、三线性插值、%d³ RGB 立方体密集扫描。\n\n",
		d.GridSize, d.Cube.Grid)
	fmt.Fprintf(b, "- LUT A：逐通道伽马曲线（gamma=%.2f），非线性对比度变换。\n", GammaA)
	fmt.Fprintf(b, "- LUT B：Rec.709 亮度加权的饱和度矩阵（增益=%.2f）+ 色域裁剪。\n", SaturationB)
	fmt.Fprintf(b, "- 管线一：`A→B`，即 `B(A(x))`；管线二：`B→A`，即 `A(B(x))`。\n\n")

	neu := d.Cube.Regions[RegionNeutralBit]
	sat := d.Cube.Regions[RegionSaturatedBit]
	bnd := d.Cube.Regions[RegionBoundaryBit]
	fmt.Fprintf(b, "**核心结论**\n\n")
	fmt.Fprintf(b, "- 内部中性灰轴 R=G=B 上两条管线严格一致，固定中性样本差为 0；仅在近黑端第一个 LUT 格内，节点裁剪加三线性插值会产生最多 %.1f 码值的轻微偏差。近中性统计桶（极差≤0.04）最大 %.1f 码值、平均 ΔE %.2f，与高饱和区相差一个数量级。\n",
		d.Cube.GrayAxisMaxCode, neu.MaxCodeDelta, neu.MeanDeltaE)
	fmt.Fprintf(b, "- 高饱和区域不可交换：最大通道差 %.1f 个 8-bit 码值、最大 ΔE*ab %.2f，均值 ΔE %.2f。\n",
		sat.MaxCodeDelta, sat.MaxDeltaE, sat.MeanDeltaE)
	fmt.Fprintf(b, "- 接近边界值的区域同样不可交换：最大通道差 %.1f 码值；裁剪与插值会放大顺序效应。\n",
		bnd.MaxCodeDelta)
	fmt.Fprintf(b, "- 全立方体最坏点 ΔE*ab=%.2f（%.1f 码值），属于肉眼明显可见的差异。\n",
		d.Cube.All.MaxDeltaE, d.Cube.All.MaxCodeDelta)
	fmt.Fprintf(b, "- `(AB+BA)/2` 的平均结果既不是 A→B 也不是 B→A；相对指定顺序 A→B，全立方体有 %d 个采样点平均误差超过 1 个码值，平均法不能掩盖分歧。\n\n",
		d.Cube.All.AvgRefOver1)
}

func writeSampleTable(b *strings.Builder, d *ReportData) {
	b.WriteString("## 1. 固定测试样本\n\n")
	b.WriteString("码值为 8-bit 0–255 量化结果；ΔE 为 CIE76（>2 通常一眼可辨）。“平均误差”列是把 `(AB+BA)/2` 当作 A→B 交付时的最大通道误差（码值）。\n\n")
	b.WriteString("| ID | 样本 | 类别 | 输入 | A→B 码值 | B→A 码值 | ΔE*ab | 最大码值差 | 平均相对 A→B 误差 |\n")
	b.WriteString("|---|---|---|---|---|---|---:|---:|---:|\n")
	for _, s := range d.Samples {
		fmt.Fprintf(b, "| %s | %s | %s | `%s` | `%s` | `%s` | %.2f | %.1f | %.1f |\n",
			s.ID, s.Name, groupsString(s.Groups), codeRGB(s.Color),
			codeRGB(s.AB), codeRGB(s.BA), s.DeltaE, s.CodeDelta, s.AvgRefCodeErr)
	}
	b.WriteString("\n原始浮点数据见 `samples.csv`。\n\n")
}
