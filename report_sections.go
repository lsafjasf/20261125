package lutdiff

import (
	"fmt"
	"strings"
)

func writeRegionSection(b *strings.Builder, d *ReportData) {
	b.WriteString("## 2. 哪些输入区域不可交换（RGB 立方体密集扫描）\n\n")
	fmt.Fprintf(b, "在 %d³=%d 个均匀采样颜色上分别运行两条真实 LUT 管线，按输入分类统计：\n\n",
		d.Cube.Grid, d.Cube.Total)
	b.WriteString("| 输入区域 | 采样数 | 平均 ΔE | P95 距离 | 最大 ΔE | 最大码值差 | 差>3 码值点数 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
	rows := []RegionStat{d.Cube.All}
	for _, bit := range []Region{RegionNeutralBit, RegionSaturatedBit, RegionBoundaryBit} {
		if st := d.Cube.Regions[bit]; st != nil {
			rows = append(rows, *st)
		}
	}
	for _, st := range rows {
		fmt.Fprintf(b, "| %s | %d | %.2f | %.4f | %.2f | %.1f | %d |\n",
			st.Label, st.Count, st.MeanDeltaE, st.P95Dist, st.MaxDeltaE,
			st.MaxCodeDelta, st.Over3Code)
	}
	b.WriteString("\n分类阈值：中性色=通道极差≤0.04；高饱和=HSV 饱和度≥0.70 且明度≥0.20；接近边界=任一通道≤0.03 或 ≥0.97。类别可重叠。\n\n")

	b.WriteString("**区域结论**\n\n")
	neu := d.Cube.Regions[RegionNeutralBit]
	sat := d.Cube.Regions[RegionSaturatedBit]
	bnd := d.Cube.Regions[RegionBoundaryBit]
	b.WriteString("1. **中性色总体可交换，内部灰轴严格可交换**：B 是以亮度为中心的饱和度矩阵，对 R=G=B 的灰色不产生偏移；A 是逐通道同一非线性曲线，灰轴始终保持 R=G=B。固定中性样本（黑、18%/50%/80% 灰、白）两种顺序码值完全相同，v∈[0.05,0.95] 的内部灰轴误差为 0。\n")
	fmt.Fprintf(b, "   - 唯一例外是近黑端第一个 LUT 格：B 在零通道附近的角点会产生负通道并被裁剪回 [0,1]，裁剪后的节点再经三线性插值，会给灰轴带来轻微色偏，257 级灰阶扫描的全轴最大值为 %.2f 码值（出现在约 v=0.016）。这是“边界 + LUT 量化插值”的次级效应，量级远小于高饱和区的 %.1f 码值。\n",
		d.Cube.GrayAxisMaxCode, sat.MaxCodeDelta)
	fmt.Fprintf(b, "   - 放宽后的“中性色”统计桶包含极差≤0.04 的近中性色，最大 %.1f 码值，但平均 ΔE 仅 %.2f。\n",
		neu.MaxCodeDelta, neu.MeanDeltaE)
	fmt.Fprintf(b, "2. **高饱和区域是主要不可交换区**：A 的伽马会改变三通道的相对比例，之后再做饱和度矩阵，与先做矩阵再做伽马得到的比例不同。该区域平均 ΔE=%.2f、最坏 %.2f、最大码值差 %.1f。\n",
		sat.MeanDeltaE, sat.MaxDeltaE, sat.MaxCodeDelta)
	fmt.Fprintf(b, "3. **接近边界值（0 或 1 附近）不可交换且更不稳定**：饱和度增益把通道推出 [0,1] 后发生裁剪，裁剪与非线性曲线的先后顺序会改变“哪些通道被裁、裁前数值是多少”。边界区域最大码值差 %.1f。注意完全落在主色边界（如纯红 1,0,0）时两种顺序可能被裁剪成同一个点，真正敏感的是边界内侧的近零/近顶颜色（见样本 E2、E3、E5）。\n\n",
		bnd.MaxCodeDelta)

	b.WriteString("**感知差异（ΔE*ab）最大的 12 个立方体采样点**\n\n")
	b.WriteString("| 输入 RGB | 区域标记 | ΔE*ab | 码值差 |\n")
	b.WriteString("|---|---|---:|---:|\n")
	for _, p := range d.Cube.Top {
		tags := regionTags(p.Regions)
		fmt.Fprintf(b, "| `%s` | %s | %.2f | %.1f |\n",
			rgbString(p.C), tags, p.DeltaE, p.CodeDelta)
	}
	b.WriteString("\n这些感知最差点集中在蓝紫与红—品红方向（绿通道为 0 的次通道近零高饱和色），以及暗部高饱和蓝；按欧氏距离排序的最差点则出现在亮绿边界。两种排序都落在“高饱和 + 至少一个通道贴近 0/1”的交集，和区域统计一致。\n\n")
}

func regionTags(r Region) string {
	var tags []string
	if r&RegionNeutralBit != 0 {
		tags = append(tags, "中性")
	}
	if r&RegionSaturatedBit != 0 {
		tags = append(tags, "高饱和")
	}
	if r&RegionBoundaryBit != 0 {
		tags = append(tags, "边界")
	}
	if len(tags) == 0 {
		return "一般中间调"
	}
	return strings.Join(tags, "/")
}

func writeAverageSection(b *strings.Builder, d *ReportData) {
	b.WriteString("## 3. 为什么不能用“把两个结果平均”来掩盖\n\n")
	b.WriteString("设两种顺序的结果为 `X = B(A(x))` 与 `Y = A(B(x))`，平均结果为 `M = (X+Y)/2`。则恒有：\n\n")
	b.WriteString("```text\n")
	b.WriteString("M - X = (Y - X) / 2\n")
	b.WriteString("|M - X| = |X - Y| / 2\n")
	b.WriteString("```\n\n")
	b.WriteString("也就是说，平均只把“相对任一端点的数值误差”减半，并没有消除顺序分歧：\n\n")
	all := d.Cube.All
	fmt.Fprintf(b, "- 平均结果 `M` 一般既不等于 `X` 也不等于 `Y`，它不是任何一条被实现/规定的 LUT 管线的输出，无法通过顺序正确性测试。\n")
	fmt.Fprintf(b, "- 若规格指定 A→B 为正式顺序，用 `M` 交付会在全立方体 %d 个采样点中的 %d 个点产生超过 1 个 8-bit 码值的系统性偏差，最大偏差 %.1f 码值（恰好是两顺序最大码值差 %.1f 的一半）。\n",
		all.Count, all.AvgRefOver1, all.MaxAvgRefCode, all.MaxCodeDelta)
	sat := d.Cube.Regions[RegionSaturatedBit]
	fmt.Fprintf(b, "- 高饱和区域中平均相对 A→B 的最大偏差仍有 %.1f 码值；固定样本里 S2（翠绿）平均偏差约 %.1f 码值、S1/S6 也都超过严格的 ±1 码值验收线。\n",
		sat.MaxCodeDelta/2, sampleAvgErr(d, "S2"))
	b.WriteString("- 视觉上平均图会显得“正常”，因为两张相近图的平均仍是一张合理图像；这正是它的迷惑性——它掩盖的是管线歧义，而不是解决歧义。正确做法是明确唯一顺序、让两种实现对齐到同一顺序，而不是输出平均。\n\n")
	b.WriteString("产物 `average_AB_BA.png` 是平均图，`average_error_vs_AB.png` 用与 `diff_map.png` 完全相同的色标刻度显示“把平均当 A→B”的误差：误差图案与差异图相同、强度恰好减半。\n\n")
}

func sampleAvgErr(d *ReportData, id string) float64 {
	for _, s := range d.Samples {
		if s.ID == id {
			return s.AvgRefCodeErr
		}
	}
	return 0
}

func writeImageSection(b *strings.Builder, d *ReportData) {
	b.WriteString("## 4. 差异图与图像产物\n\n")
	fmt.Fprintf(b, "测试图卡 `source_chart.png` 上方是色相×饱和度平面（顶边全饱和、底边零饱和中性轴），下方六条色带依次为：%s。\n\n",
		strings.Join(RampNames, "、"))
	b.WriteString("- `order_A_then_B.png`：先 A 后 B 的真实结果。\n")
	b.WriteString("- `order_B_then_A.png`：先 B 后 A 的真实结果。\n")
	fmt.Fprintf(b, "- `diff_map.png`（同时另存为 `diff.png`）：逐像素欧氏 RGB 距离热图，色标上限固定为 %.3f（全立方体最坏欧氏距离），图下有绝对刻度；图卡内最大欧氏距离 %.3f、最大单通道差 %.1f 个 8-bit 码值。\n",
		d.Art.HeatScale, d.Art.ImageMaxDist, d.Art.ImageMaxCode)
	fmt.Fprintf(b, "- `diff_amplified_x%.0f.png`：逐像素最大通道差放大 %.0f 倍的灰度图，用于看清微弱差异的空间分布。\n",
		d.Art.Gain, d.Art.Gain)
	b.WriteString("- `average_AB_BA.png`：两个顺序的逐像素平均（仅用于反证，不是交付管线）。\n")
	b.WriteString("- `average_error_vs_AB.png`：平均结果相对 A→B 的误差热图，与差异图同刻度。\n\n")
	b.WriteString("在差异图上可以直接看到：色相平面顶部的高饱和带最亮，向底部中性轴衰减为零；下方纯红/绿/蓝渐变带的中后段明显发亮，纯灰阶带几乎全零（仅最左端近黑首格有极弱热区，放大图中可见），近黑 tint 与近白暖带整体较弱，可在放大灰度图中定位。\n\n")
}

func writeReproSection(b *strings.Builder, d *ReportData) {
	b.WriteString("## 5. 复现与验证\n\n")
	b.WriteString("```bash\n")
	b.WriteString("go run ./cmd/lutdiff     # 重新生成全部 PNG、samples.csv 与本报告\n")
	b.WriteString("go test ./...            # 恒等 LUT、区域可交换/不可交换、平均反证等测试\n")
	b.WriteString("```\n\n")
	b.WriteString("测试显式覆盖三类样本：中性色（断言两顺序一致）、高饱和色（断言显著不一致）、接近边界值（断言顺序敏感），并包含“平均无法通过参考顺序精度验收”的反掩盖测试。\n")
}
