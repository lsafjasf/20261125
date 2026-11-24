package main

import (
	"fmt"
	"math"
)

func WritePeakComparisonChart(path string, cfgs []Config) error {
	const width, height = 1280, 820
	s := NewSVG(width, height)
	s.Rect(0, 0, width, height, "#f8fafc")
	s.Text(40, 38, 24, ink, "start", "700", "三档显示峰值对照：同一段 HLG 测试素材")
	s.Text(40, 64, 14, muted, "start", "400",
		"每列使用同一个 HLG 码值数组，只改变 Lw、由 Lw 推导出的系统 gamma 与 OOTF α。上灰阶按本档峰值归一化，下灰阶按共同 1000 nit 标尺显示。")

	n := len(cfgs)
	gap := 22.0
	side := 30.0
	cardW := (float64(width) - 2*side - float64(n-1)*gap) / float64(n)
	cardTop := 84.0
	cardH := 686.0

	for i, cfg := range cfgs {
		x := side + float64(i)*(cardW+gap)
		drawPeakCard(s, cfg, i, x, cardTop, cardW, cardH)
	}

	s.Rect(40, 786, 1200, 24, "#ecfdf5", `stroke="#a7f3d0" rx="9"`)
	s.Text(56, 803, 12, "#065f46", "start", "700",
		"结论：相对预览可以都很平滑；共同绝对标尺和 CSV/JSON 的 nit 数值才会暴露峰值尺度或 α 标定错误。")
	s.Close()
	return s.Save(path)
}

func drawPeakCard(s *SVG, cfg Config, index int, x, y, w, h float64) {
	color := seriesColors[index%len(seriesColors)]
	s.Rect(x, y, w, h, "#ffffff", `stroke="`+cardStroke+`" stroke-width="1.2" rx="20"`)
	s.Rect(x, y, 8, h, color, `rx="4"`)
	s.Text(x+26, y+40, 23, ink, "start", "800", cfg.Label+" 显示器")
	s.Text(x+26, y+66, 13, muted, "start", "600",
		fmt.Sprintf("Lw=%s · γ=%.4f · α=%s", commaNits(cfg.PeakNits), cfg.Gamma(), commaNits(cfg.Alpha())))

	rampX := x + 26
	rampW := w - 52
	relID := fmt.Sprintf("relRamp%d", index)
	absID := fmt.Sprintf("absRamp%d", index)
	addRampGradient(s, relID, cfg, cfg.PeakNits)
	addRampGradient(s, absID, cfg, cfg.ReferenceNits)

	s.Text(rampX, y+103, 14, "#334155", "start", "800", "面板相对灰阶 L/Lw")
	rampY := y + 116
	s.Rect(rampX, rampY, rampW, 32, "url(#"+relID+")", `stroke="#cbd5e1" rx="8"`)
	drawRampTicks(s, cfg, rampX, rampY, rampW, 32, false)

	s.Text(rampX, y+204, 14, "#334155", "start", "800",
		fmt.Sprintf("共同绝对标尺（%s=白）", commaNits(cfg.ReferenceNits)+" nit"))
	absY := y + 216
	s.Rect(rampX, absY, rampW, 32, "url(#"+absID+")", `stroke="#cbd5e1" rx="8"`)
	drawRampTicks(s, cfg, rampX, absY, rampW, 32, true)
	s.Text(rampX, y+303, 12, absoluteClipColor(cfg), "start", "700", absoluteClipNote(cfg))

	s.Text(rampX, y+332, 14, "#334155", "start", "800", "彩色高光 E′=0.95（相对本档峰值）")
	drawCardColors(s, cfg, rampX, y+360, rampW)
	drawMetricBox(s, cfg, x+26, y+520, w-52)
}

func addRampGradient(s *SVG, id string, cfg Config, whiteNits float64) {
	samples := grayCurve(cfg, 48)
	stops := make([]GradientStop, 0, len(samples))
	for _, sample := range samples {
		preview := NitsToPreviewSRGB(sample.DisplayNits, whiteNits)
		stops = append(stops, GradientStop{Offset: sample.Code.R, Color: RGBHex(preview)})
	}
	s.LinearGradient(id, true, stops)
}

func drawRampTicks(s *SVG, cfg Config, x, y, w, h float64, absolute bool) {
	codes := []float64{0, 0.5, 0.75, 1}
	anchors := []string{"start", "middle", "middle", "end"}
	for i, code := range codes {
		tx := x + code*w
		s.Text(tx, y+h+18, 11, muted, anchors[i], "600", fmt.Sprintf("%.2f", code))
		sample := cfg.DecodeGray(code)
		nit := sample.RawLumaNits
		color := "#334155"
		if absolute && nit > cfg.ReferenceNits*1.0001 {
			color = "#b91c1c"
		}
		s.Text(tx, y+h+35, 10, color, anchors[i], "800", compactNits(nit)+" nt")
	}
}

func absoluteClipNote(cfg Config) string {
	codes := GrayCodes(200)
	first := -1.0
	for _, code := range codes {
		if cfg.DecodeGray(code).RawLumaNits > cfg.ReferenceNits*1.0001 {
			first = code
			break
		}
	}
	if first < 0 {
		return "本档峰值即参考标尺，端点为参考白。"
	}
	return fmt.Sprintf("E′ > %.3f 的灰阶超过参考白，共同 SDR 预览裁切。", first)
}

func absoluteClipColor(cfg Config) string {
	for _, code := range GrayCodes(200) {
		if cfg.DecodeGray(code).RawLumaNits > cfg.ReferenceNits*1.0001 {
			return "#b91c1c"
		}
	}
	return "#047857"
}

func drawCardColors(s *SVG, cfg Config, x, y, w float64) {
	patches := HighlightPatches(0.95)
	cols := 4
	gapX := 10.0
	sw := (w - float64(cols-1)*gapX) / float64(cols)
	sh := 42.0
	for i, patch := range patches {
		col := i % cols
		row := i / cols
		sx := x + float64(col)*(sw+gapX)
		sy := y + float64(row)*78
		sample := cfg.Decode(patch.Code)
		fill := RGBHex(NitsToPreviewSRGB(sample.DisplayNits, cfg.PeakNits))
		s.Text(sx, sy-8, 12, "#334155", "start", "800", patch.Name)
		stroke := "#cbd5e1"
		swWidth := 1.0
		if sample.OverPeak {
			stroke = "#dc2626"
			swWidth = 2
		}
		s.Rect(sx, sy, sw, sh, fill,
			fmt.Sprintf(`stroke="%s" stroke-width="%.1f" rx="9"`, stroke, swWidth))
		textColor := "#ffffff"
		if luminanceContrast(fill) > 0.55 {
			textColor = "#0f172a"
		}
		s.Text(sx+sw/2, sy+26, 11, textColor, "middle", "900", compactNits(sample.RawLumaNits))
		s.Text(sx+sw/2, sy+sh+15, 10, "#64748b", "middle", "700", "Y nit")
	}
}

func drawMetricBox(s *SVG, cfg Config, x, y, w float64) {
	const h = 126.0
	s.Rect(x, y, w, h, "#f8fafc", `stroke="#e2e8f0" rx="14"`)
	gray75 := cfg.DecodeGray(0.75)
	white95 := cfg.Decode(HighlightPatches(0.95)[0].Code)
	superWhite := cfg.DecodeGray(1.05)
	lines := []string{
		fmt.Sprintf("gamma_law=%s  OOTF=%s  beta=%.4f", cfg.GammaLaw, cfg.OOTF, cfg.Beta()),
		fmt.Sprintf("E′=0.75 灰阶 → %.1f nit", gray75.RawLumaNits),
		fmt.Sprintf("E′=0.95 白高光 → %.1f nit", white95.RawLumaNits),
		fmt.Sprintf("E′=1.05 超白 raw=%.0f nit", superWhite.RawLumaNits),
		fmt.Sprintf("硬裁切后通道上限 = %.0f nit", math.Min(superWhite.RawLumaNits, cfg.PeakNits)),
	}
	for i, line := range lines {
		color := "#334155"
		weight := "600"
		if i >= 3 {
			color = "#b45309"
			weight = "800"
		}
		s.Text(x+14, y+24+float64(i)*20, 12, color, "start", weight, line)
	}
}
