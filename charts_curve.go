package main

import (
	"fmt"
	"math"
)

func WriteGrayscaleChart(path string, cfgs []Config, wrong Config) error {
	const width, height = 1200, 720
	s := NewSVG(width, height)
	s.Rect(0, 0, width, height, "#f8fafc")
	s.Text(40, 38, 24, ink, "start", "700", "HLG 灰阶曲线：系统 gamma / 显示峰值 / OOTF")
	s.Text(40, 64, 14, muted, "start", "400",
		"同一 HLG 码值经 inverse OETF 与 BT.2100 OOTF 后，必须在绝对 nit 轴上验收；归一化形状平滑不等于亮度标定正确。")

	drawCurveLegend(s, cfgs, wrong)

	maxPeak := wrong.PeakNits
	for _, cfg := range cfgs {
		maxPeak = math.Max(maxPeak, cfg.PeakNits)
	}
	yMax, yTicks := niceBounds(maxPeak, 4)
	absolute := plotBox{X: 82, Y: 126, W: 610, H: 386, XMin: 0, XMax: 1, YMin: 0, YMax: yMax}
	normalized := plotBox{X: 790, Y: 126, W: 350, H: 386, XMin: 0, XMax: 1, YMin: 0, YMax: 1}

	drawPlotFrame(s, absolute, "HLG 码值 E′", "绝对亮度 L (nit)", yTicks, true)
	drawNormalizedFrame(s, normalized)

	for i, cfg := range cfgs {
		color := seriesColors[i%len(seriesColors)]
		drawSampleCurve(s, cfg, absolute, color, false, false)
		drawSampleCurve(s, cfg, normalized, color, true, false)
		drawEndpoint(s, cfg, absolute, color, false)
	}
	drawSampleCurve(s, wrong, absolute, "#dc2626", false, true)
	drawSampleCurve(s, wrong, normalized, "#dc2626", true, true)
	drawEndpoint(s, wrong, absolute, "#dc2626", true)

	drawCheckpointCards(s, cfgs, wrong)
	drawSmoothWarning(s)

	s.Close()
	return s.Save(path)
}

func drawCurveLegend(s *SVG, cfgs []Config, wrong Config) {
	positions := []float64{40, 215, 390, 590, 800}
	for i, cfg := range cfgs {
		x := positions[i%len(positions)]
		y := 91.0
		color := seriesColors[i%len(seriesColors)]
		s.Line(x, y-4, x+30, y-4, color, 4, "")
		label := fmt.Sprintf("%s · γ=%.3f", cfg.Label, cfg.Gamma())
		s.Text(x+38, y, 13, ink, "start", "600", label)
	}
	x := positions[len(cfgs)%len(positions)]
	s.Line(x, 87, x+30, 87, "#dc2626", 4, "8 5")
	s.Text(x+38, 91, 13, "#b91c1c", "start", "700",
		fmt.Sprintf("错误 α=%s · γ=%.2f", commaNits(wrong.Alpha()), wrong.ManualGamma))
}

func drawPlotFrame(s *SVG, p plotBox, xTitle, yTitle string, yTicks []float64, nitLabels bool) {
	s.Rect(p.X, p.Y, p.W, p.H, panelFill, `stroke="`+cardStroke+`" rx="14"`)
	for _, yv := range yTicks {
		y := p.MapY(yv)
		s.Line(p.X, y, p.X+p.W, y, grid, 1, "")
		label := fmt.Sprintf("%.1f", yv)
		if nitLabels {
			label = commaNits(yv)
		}
		s.Text(p.X-10, y+4, 12, muted, "end", "400", label)
	}
	for _, xv := range []float64{0, 0.25, 0.5, 0.75, 1} {
		x := p.MapX(xv)
		s.Line(x, p.Y, x, p.Y+p.H, grid, 1, "")
		s.Text(x, p.Y+p.H+22, 12, muted, "middle", "400", fmt.Sprintf("%.2f", xv))
	}
	s.Line(p.X, p.Y, p.X, p.Y+p.H, "#94a3b8", 1.5, "")
	s.Line(p.X, p.Y+p.H, p.X+p.W, p.Y+p.H, "#94a3b8", 1.5, "")
	s.Text(p.X+p.W/2, p.Y+p.H+48, 14, "#334155", "middle", "600", xTitle)
	s.WriteString(fmt.Sprintf(
		`<text transform="translate(%.1f %.1f) rotate(-90)" text-anchor="middle" font-size="14" font-weight="600" fill="#334155">%s</text>`+"\n",
		p.X-58, p.Y+p.H/2, yTitle))
}

func drawNormalizedFrame(s *SVG, p plotBox) {
	s.Text(p.X, p.Y-12, 15, ink, "start", "700", "归一化形状 L / α（看似都平滑）")
	drawPlotFrame(s, p, "HLG 码值 E′", "归一化亮度", []float64{0, 0.25, 0.5, 0.75, 1}, false)
}

func drawSampleCurve(s *SVG, cfg Config, p plotBox, color string, normalized, dashed bool) {
	samples := grayCurve(cfg, 240)
	points := make([]Point, 0, len(samples))
	for _, sample := range samples {
		yValue := sample.RawLumaNits
		if normalized {
			yValue = sample.RawLumaNits / sample.AlphaNits
		}
		points = append(points, Point{X: p.MapX(sample.Code.R), Y: p.MapY(yValue)})
	}
	dash := ""
	if dashed {
		dash = "8 6"
	}
	s.Path(points, color, 3, dash)
}

func drawEndpoint(s *SVG, cfg Config, p plotBox, color string, wrong bool) {
	sample := cfg.DecodeGray(1)
	x := p.MapX(1)
	y := p.MapY(sample.RawLumaNits)
	s.Circle(x, y, 5, color, "#ffffff", 2)
	label := commaNits(sample.RawLumaNits) + " nit"
	weight := "700"
	if wrong {
		s.Text(x-12, y+20, 12, "#b91c1c", "end", weight, "错误端点 "+label)
		return
	}
	s.Text(x-12, y-10, 12, color, "end", weight, label)
}

func drawCheckpointCards(s *SVG, cfgs []Config, wrong Config) {
	y := 574.0
	cardW := 263.0
	for i, cfg := range cfgs {
		x := 40 + float64(i)*(cardW+18)
		s.Rect(x, y, cardW, 58, "#ffffff", `stroke="`+seriesColors[i]+`55" rx="12"`)
		s.Text(x+14, y+23, 13, seriesColors[i], "start", "700",
			fmt.Sprintf("%s · E′=0.75", cfg.Label))
		sample := checkpoint(cfg, 0.75)
		s.Text(x+14, y+45, 13, ink, "start", "600",
			fmt.Sprintf("绝对 Y = %s nit", formatNitValue(sample.RawLumaNits)))
	}

	ref := checkpoint(cfgs[0], 0.75)
	bad := checkpoint(wrong, 0.75)
	x := 40 + float64(len(cfgs))*(cardW+18)
	if len(cfgs) >= 3 {
		x = 40 + 3*(cardW+18)
	}
	s.Rect(x, y, cardW, 58, "#fef2f2", `stroke="#fca5a5" rx="12"`)
	s.Text(x+14, y+23, 13, "#b91c1c", "start", "700", "错误管线 · E′=0.75")
	s.Text(x+14, y+45, 12, "#7f1d1d", "start", "600",
		fmt.Sprintf("%s nit；扩展档应≈%s nit", formatNitValue(bad.RawLumaNits), formatNitValue(checkpoint(cfgs[len(cfgs)-1], .75).RawLumaNits)))
	_ = ref
}

func drawSmoothWarning(s *SVG) {
	x, y, w, h := 40.0, 646.0, 1120.0, 70.0
	s.Rect(x, y, w, h, "#fff7ed", `stroke="#fdba74" rx="14"`)
	s.Text(x+18, y+23, 14, "#9a3412", "start", "800", "为什么“曲线平滑”仍可能是错的？")
	s.Text(x+18, y+45, 13, "#7c2d12", "start", "400",
		"平滑只说明 inverse OETF/OOTF 连续、没有色带；绝对亮度由 α 与 Lw 决定。")
	s.Text(x+18, y+64, 13, "#7c2d12", "start", "400",
		"把 4000 nit 面板的 α 留在 1000，或只按各自峰值归一化绘图，形状会重合，但 E′=1 实际差 4 倍。")
}
