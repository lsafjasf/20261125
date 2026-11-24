package cutluma

import (
	"fmt"
	"os"
	"strings"
)

var kindCN = map[string]string{
	KindConversion:  "转换失配",
	KindNatural:     "正常光变",
	KindMixed:       "混合(光变+转换台阶)",
	KindPreexisting: "切点前/源端已有差异",
	KindConsistent:  "一致无台阶",
	KindAmbiguous:   "证据不足",
}

func cnKind(k string) string {
	if v, ok := kindCN[k]; ok {
		return v
	}
	return k
}

// WriteMarkdownReport renders the human-readable Chinese diagnostic report.
func WriteMarkdownReport(path string, job *Job, rep *Report, plan *RepairPlan, sum *Summary) error {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	w("# 十段分切色彩转换：切点亮度台阶诊断与修复")
	w("")
	w("任务：检查每段黑点、白点、曝光与转换参数一致性，定位台阶来自切点前的真实光变还是分段转换，")
	w("并输出跨切点曲线与修复参数。全部算法仅使用 Go 标准库。")
	w("")
	w("## 一、结论")
	w("")
	w("- 帧/段/切点：%d 帧，%d 段，%d 个切点；稳健帧间亮度噪声 σ=%.5f（归一化亮度，约 %.2f/255 8-bit 码值）。",
		sum.FrameCount, sum.SegmentCount, sum.CutCount, sum.SigmaLuma, sum.SigmaLuma*255)
	w("- 判定分布：转换失配 %d 处，混合 %d 处，正常光变 %d 处，源端已有差异 %d 处。",
		len(sum.ConversionCuts), len(sum.MixedCuts), len(sum.NaturalCuts), len(sum.PreexistingCuts))
	if sum.Truth != nil {
		mark := "全部与脚本注入真因一致 ✅"
		if !sum.AllTruthMatched {
			mark = "存在与真因不一致项 ⚠️"
		}
		w("- 合成场景自检：%s", mark)
	}
	w("- 转换类台阶最大幅度：修复前 %.4f，修复后 %.5f；平均消除 %.1f%%。",
		sum.MaxConversionGapBefore, sum.MaxConversionGapAfter, sum.MeanConversionReductionPct)
	w("- 正常光变切点的趋势保持率最低 %.1f%%（静态 LUT 只统一转换配方，不压平光变）。", sum.NaturalSlopeKeptMinPct)
	w("")

	w("## 二、每段黑点 / 白点 / 曝光 / 转换参数")
	w("")
	w("目标统一配方：black=%.4f white=%.4f exposureEV=%+.3f power=%.3f。",
		rep.TargetParams.Black, rep.TargetParams.White, rep.TargetParams.ExposureEV, rep.TargetParams.TransferPower)
	w("")
	w("| 段 | 实测黑点 | 实测白点 | 中位亮度 | 曝光差(档) | 相对γ | 声明 B/W/EV/P | 一致 | 失配字段 |")
	w("|---:|---:|---:|---:|---:|---:|---|:--:|---|")
	for _, s := range rep.Segments {
		decl := "—"
		if s.Declared != nil {
			decl = fmt.Sprintf("%.3f/%.3f/%+.2f/%.2f", s.Declared.Black, s.Declared.White,
				s.Declared.ExposureEV, s.Declared.TransferPower)
		}
		ok := "➖未提供"
		if s.DeclaredAvailable {
			if s.ParamsConsistent {
				ok = "✅"
			} else {
				ok = "❌"
			}
		}
		bad := strings.Join(s.InconsistentFields, ",")
		w("| %d | %.4f | %.4f | %.4f | %+.3f | %.3f | %s | %s | %s |",
			s.Index, s.BlackPoint, s.WhitePoint, s.MedianLuma, s.ExposureStopsVsRef, s.GammaRel, decl, ok, bad)
	}
	w("")

	w("## 三、切点取证（跨切点曲线特征）")
	w("")
	w("判定门：切点两侧 Theil–Sen 稳健直线在切点处的趋势校正间隙 `fitGap` 必须同时满足绝对幅度、")
	w("稳健 z 分数、边界增量离群分数 `outlierZ`（真台阶约 10–37，光变 <1，门限 6）与符号一致性才算“台阶”；")
	w("窗内斜率 `trendChange` 与切点前 48 帧漂移 `preCutDrift` 用于识别连续光变。")
	w("源曲线存在时，同一分析直接在转换前域复核。")
	w("")
	w("| 切点@帧 | 判定 | 置信 | 原始跳变 | 校正间隙 | z | 趋势量 | 切前漂移 | 参数失配 | 预测跳变 | 源间隙 |")
	w("|---:|---|---:|---:|---:|---:|---:|---:|:--:|---:|---:|")
	for _, c := range rep.Cuts {
		srcGap := "—"
		if c.Source.Available {
			srcGap = fmt.Sprintf("%+.4f", c.Source.FitGap)
		}
		w("| %d@%d | %s | %.2f | %+.4f | %+.4f | %.1f | %.4f | %+.4f | %s | %+.4f | %s |",
			c.CutIndex, c.Frame, cnKind(c.Kind), c.Confidence, c.EdgeJump, c.FitGap, c.ZScore,
			c.TrendChange, c.PreCutDrift, mapMark(c.ParamMismatch), c.PredictedStep, srcGap)
	}
	w("")

	w("## 四、修复参数与闭环验证")
	w("")
	w("首选修复 = 用每段声明配方求逆，再套用统一目标配方：`y' = target(segment^-1(y))`，")
	w("导出为 257 点 1D-LUT（见 `repair_params.json` 中每段 `lut`），可直接加载到色彩管线；")
	w("无声明参数时退化为分位数锚点单调 LUT。残留间隙由短接缝斜坡 `trims` 补偿。")
	w("")
	w("| 切点 | 判定 | 修复前间隙 | 修复后间隙 | 消除率 | 趋势保持 |")
	w("|---:|---|---:|---:|---:|---:|")
	for _, v := range plan.Verification {
		w("| %d | %s | %+.5f | %+.5f | %.1f%% | %.1f%% |",
			v.CutIndex, cnKind(v.Kind), v.GapBefore, v.GapAfter, v.ReductionPct, v.SlopeKeptPct)
	}
	w("")
	w("`preexisting_step` 切点的源端硬跳变被刻意保留（消除率接近 0），因为那不是转换造成的缺陷；")
	w("`natural_light_change` 同理，只做全段统一配方映射，不做切点拉平。")
	w("")

	w("## 五、产物文件")
	w("")
	for _, name := range []string{
		"summary.json", "report.json", "repair_params.json", "input_job.json",
		"segment_report.csv", "cut_report.csv", "curves_cross_cut.csv", "curve_timeline.csv", "curves.svg",
	} {
		if p, ok := sum.Files[nameKey(name)]; ok {
			w("- `%s` → `%s`", name, p)
		} else {
			w("- `%s`", name)
		}
	}
	w("- `curves.png` → SVG 的位图预览（随产物提供；可用 rsvg/librsvg 由 `curves.svg` 重新渲染）。")
	w("")
	w("`curves_cross_cut.csv` 字段：cut, rel_frame(-24..23), abs_frame, segment, luma_raw, luma_repaired, source_luma。")
	w("")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func mapMark(b bool) string {
	if b {
		return "是"
	}
	return "—"
}

func nameKey(name string) string {
	switch name {
	case "summary.json":
		return "summary"
	case "report.json":
		return "report"
	case "repair_params.json":
		return "repair"
	case "input_job.json":
		return "input"
	case "segment_report.csv":
		return "segmentsCSV"
	case "cut_report.csv":
		return "cutsCSV"
	case "curves_cross_cut.csv":
		return "crossCutCSV"
	case "curve_timeline.csv":
		return "timelineCSV"
	case "curves.svg":
		return "svg"
	}
	return name
}
