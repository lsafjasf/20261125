package main

import (
	"fmt"
	"time"

	"rec2020-to-rec709-shoulder/internal/pipeline"
	"rec2020-to-rec709-shoulder/internal/report"
)

func buildReport(o *options, generated bool, jobs []frameJob, decisions []pipeline.Decision, metrics []pipeline.FrameMetrics, contact contactSet) report.Report {
	input := o.Input
	title := "Rec.2020 竖屏转 Rec.709：硬裁切 / 固定肩 / 自适应肩对比"
	if generated {
		input = "<程序生成的确定性 Rec.2020 竖屏诊断片>"
	}
	width, height := o.Width, o.Height
	if contact.source != nil {
		width, height = contact.source.Width, contact.source.Height
	}
	mode := "逐通道压缩（可观察色相漂移）"
	if o.PreserveHue {
		mode = "max-RGB 比例压缩（保持 RGB 比例与色相）"
	}
	assumptions := []string{
		"PNG/JPEG 默认按 full-range Rec.2020 R'G'B' 处理，并用 BT.709 风格反 OETF 转到线性光；16-235 legal range 素材请加 -range limited。",
		"PFM 帧按场景线性 Rec.2020 RGB 处理，允许像素值大于 1，可表达超白。",
		"三版先共享 D65 Rec.2020→XYZ→Rec.709 矩阵，以及保持 Rec.709 亮度的负通道色域压缩，确保差异主要来自高光处理。",
		fmt.Sprintf("软肩模式：%s；输出最终编码为 Rec.709 signal。", mode),
		"硬裁切是零宽度 knee/clamp；固定肩全片使用同一 knee；自适应从鲁棒场景统计求 knee，并经过时间中值、滞回和每帧限速。",
	}
	rules := []string{
		"每帧计算 P99.5 max-RGB、max-RGB>1 超域比例、max-RGB>0.75 高光面积，以及高光/全局 Sobel 梯度能量比。",
		fmt.Sprintf("各标量先经过 %d 帧居中中位数稳定；单个椒盐/噪声帧不会成为中位数。", o.Window),
		"texture_guard 需 pressure≥0.58 且连续两次；specular_guard 需 pressure≤0.20 且连续两次；退出滞回阈值分别为 0.43 和 0.34。",
		fmt.Sprintf("连续 knee=0.86-0.24*pressure，裁剪到 [0.62,0.86]，再限制每帧变化不超过 %.3f。", o.MaxSlew),
		"显式审计触发：超域像素≥1.20%、P99.5≥1.20、高光面积≥12%，或高光/全局纹理比≥1.25 且高光面积≥2.5%。",
		"亮度直方图距离≥0.32 会标记为场景切换，但 knee 仍受每帧限速约束，不会瞬时乱跳。",
	}
	return report.Report{
		Title: title, CreatedAt: time.Now().UTC(), Input: input, FrameCount: len(jobs),
		Width: width, Height: height, InputTransfer: o.Transfer, SignalRange: o.SignalRange,
		OutputTransfer: "rec709", FixedKnee: o.FixedKnee, HuePreserveShoulder: o.PreserveHue,
		Assumptions: assumptions, AdaptiveRules: rules,
		Summaries: report.Summarize(metrics), Decisions: decisions, FrameMetrics: metrics,
	}
}

func printSummary(out string, summaries []report.VariantSummary, decisions []pipeline.Decision) {
	fmt.Printf("\nwrote three versions and report to %s\n", out)
	fmt.Printf("%-18s %8s %8s %9s %9s %10s %10s\n", "variant", "hueMean", "hueP95", "hue>10°%", "clip%", "hiTexRet", "intTex")
	for _, s := range summaries {
		fmt.Printf("%-18s %8.2f %8.2f %9.2f %9.3f %10.3f %10.3f\n",
			s.Variant, s.HueMeanAbsDegrees, s.HueP95AbsDegrees, s.HueOver10Pct,
			s.OutputClipPct, s.HighlightTextureRetention, s.SevereInteriorRetention)
	}
	if len(decisions) > 0 {
		first := decisions[0]
		last := decisions[len(decisions)-1]
		fmt.Printf("adaptive knee: frame0 %.3f/%s -> last %.3f/%s; details in %s/adaptive_decisions.json\n",
			first.Knee, first.Profile, last.Knee, last.Profile, out)
	}
}
