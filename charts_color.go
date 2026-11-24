package main

import (
	"fmt"
	"math"
)

func WriteColorChart(path string, cfgs []Config) error {
	const width, height = 1280, 850
	s := NewSVG(width, height)
	s.Rect(0, 0, width, height, "#f8fafc")
	s.Text(40, 38, 24, ink, "start", "700", "彩色高光：同一组 HLG 码值在三档峰值下的 OOTF 结果")
	ootfName := string(OOTFBT2100)
	if len(cfgs) > 0 {
		ootfName = string(cfgs[0].OOTF)
	}
	s.Text(40, 64, 14, muted, "start", "400",
		"色板按各显示器自身峰值归一化并近似转换到 sRGB；下方 Y 是 Rec.2020 亮度 nit，红框表示 OOTF 原始值超过物理峰值并被硬裁切。OOTF="+ootfName)

	levels := HighlightLevels()
	const left = 130.0
	const top = 136.0
	const colW = 220.0
	const rowH = 78.0

	for col, level := range levels {
		x := left + float64(col)*colW
		s.Text(x+colW/2, 112, 15, "#334155", "middle", "800", fmt.Sprintf("E′ = %.2f", level))
		s.Line(x+18, 122, x+colW-18, 122, grid, 1, "")
	}

	names := HighlightPatches(levels[0])
	for row := range names {
		y := top + float64(row)*rowH
		if row%2 == 0 {
			s.Rect(24, y-18, 1232, rowH-8, "#ffffff", `rx="12"`)
		}
		s.Text(48, y+18, 18, ink, "start", "800", names[row].Name)

		for col, level := range levels {
			patch := HighlightPatches(level)[row]
			colX := left + float64(col)*colW
			for i, cfg := range cfgs {
				sample := cfg.Decode(patch.Code)
				sw := 64.0
				sh := 38.0
				sx := colX + 8 + float64(i)*(sw+8)
				sy := y - 12
				preview := NitsToPreviewSRGB(sample.DisplayNits, cfg.PeakNits)
				fill := RGBHex(preview)
				stroke := "#cbd5e1"
				strokeWidth := 1.0
				if sample.OverPeak {
					stroke = "#dc2626"
					strokeWidth = 2.4
				}
				s.Rect(sx, sy, sw, sh, fill,
					fmt.Sprintf(`stroke="%s" stroke-width="%.1f" rx="9"`, stroke, strokeWidth))
				labelColor := "#334155"
				if luminanceContrast(fill) < 0.45 {
					labelColor = "#ffffff"
				}
				s.Text(sx+sw/2, sy+16, 10, labelColor, "middle", "800", cfg.Label)
				s.Text(sx+sw/2, sy+31, 9, labelColor, "middle", "600",
					"γ"+fmt.Sprintf("%.3f", cfg.Gamma()))
				if sample.OverPeak {
					s.Text(sx+sw-7, sy+11, 13, "#ffffff", "middle", "900", "!")
				}
				valueColor := "#64748b"
				if sample.OverPeak {
					valueColor = "#b91c1c"
				}
				s.Text(sx+sw/2, sy+sh+15, 10, valueColor, "middle", "700",
					"Y "+compactNits(sample.RawLumaNits)+" nit")
			}
		}
	}

	noteY := 806.0
	s.Rect(40, noteY, 1200, 30, "#eff6ff", `stroke="#bfdbfe" rx="10"`)
	s.Text(58, noteY+20, 13, "#1e3a8a", "start", "600",
		"每格三个色板对应 -peaks；完整 RGB/Y nit 与裁切状态见 highlights.csv、report.json。")
	s.Close()
	return s.Save(path)
}

func compactNits(v float64) string {
	if math.Abs(v) < 0.05 {
		return "0"
	}
	if v >= 10000 {
		return fmt.Sprintf("%.0fk", v/1000)
	}
	if v >= 1000 {
		return fmt.Sprintf("%.2fk", v/1000)
	}
	if v >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func luminanceContrast(hexColor string) float64 {
	if len(hexColor) != 7 || hexColor[0] != '#' {
		return 1
	}
	var r, g, b int
	fmt.Sscanf(hexColor[1:], "%02x%02x%02x", &r, &g, &b)
	return (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 255
}
