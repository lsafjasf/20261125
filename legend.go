package lutdiff

import (
	"fmt"
	"image"
	"image/color"

	"image/draw"
)

var digitGlyphs = map[rune][]string{
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
	'.': {"00000", "00000", "00000", "00000", "00000", "00110", "00110"},
	'-': {"00000", "00000", "00000", "11111", "00000", "00000", "00000"},
}

func textWidth(s string, scale int) int {
	if len(s) == 0 {
		return 0
	}
	return (len(s)*6 - 1) * scale
}

func drawText(img *image.RGBA, x, y, scale int, s string, col color.RGBA) {
	cx := x
	for _, ch := range s {
		glyph, ok := digitGlyphs[ch]
		if !ok {
			cx += 6 * scale
			continue
		}
		for row, bits := range glyph {
			for colIdx := 0; colIdx < 5; colIdx++ {
				if bits[colIdx] != '1' {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						px := cx + colIdx*scale + dx
						py := y + row*scale + dy
						if image.Pt(px, py).In(img.Bounds()) {
							img.SetRGBA(px, py, col)
						}
					}
				}
			}
		}
		cx += 6 * scale
	}
}

// HeatmapWithLegend appends a labeled color scale strip to a heatmap. The
// labels show Euclidean RGB distance in normalized [0,1] units.
func HeatmapWithLegend(h *image.RGBA, scale float64) *image.RGBA {
	const stripH = 42
	out := image.NewRGBA(image.Rect(0, 0, h.Bounds().Dx(), h.Bounds().Dy()+stripH))
	draw.Draw(out, h.Bounds(), h, image.Point{}, draw.Src)

	w := out.Bounds().Dx()
	y0 := h.Bounds().Dy()
	white := color.RGBA{R: 248, G: 248, B: 248, A: 255}
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	draw.Draw(out, image.Rect(0, y0, w, y0+stripH), &image.Uniform{C: white}, image.Point{}, draw.Src)

	barX0, barX1 := 70, w-70
	barY0, barY1 := y0+7, y0+19
	for x := barX0; x < barX1; x++ {
		t := float64(x-barX0) / float64(barX1-barX0-1)
		c := HeatColor(t)
		cc := color.RGBA{R: byte(c[0] * 255), G: byte(c[1] * 255), B: byte(c[2] * 255), A: 255}
		for y := barY0; y < barY1; y++ {
			out.SetRGBA(x, y, cc)
		}
	}
	for x := barX0 - 1; x <= barX1; x++ {
		out.SetRGBA(x, barY0-1, dark)
		out.SetRGBA(x, barY1, dark)
	}
	for y := barY0 - 1; y <= barY1; y++ {
		out.SetRGBA(barX0-1, y, dark)
		out.SetRGBA(barX1, y, dark)
	}

	for _, frac := range []float64{0, 0.5, 1} {
		value := frac * scale
		label := fmt.Sprintf("%.3f", value)
		lw := textWidth(label, 2)
		tx := barX0 + int(frac*float64(barX1-barX0)) - lw/2
		drawText(out, tx, y0+23, 2, label, dark)
	}
	return out
}
