package main

import (
	"image"
	"image/color"
	"math"
	"strconv"
)

const (
	patternWidth  = 320
	patternHeight = 192
)

var (
	rgbRed     = color.RGBA{R: 220, G: 24, B: 42, A: 255}
	rgbCyan    = color.RGBA{R: 18, G: 190, B: 210, A: 255}
	rgbBlue    = color.RGBA{R: 24, G: 62, B: 210, A: 255}
	rgbYellow  = color.RGBA{R: 245, G: 220, B: 32, A: 255}
	rgbGreen   = color.RGBA{R: 28, G: 175, B: 64, A: 255}
	rgbMagenta = color.RGBA{R: 210, G: 28, B: 180, A: 255}
	rgbBlack   = color.RGBA{R: 20, G: 24, B: 28, A: 255}
	rgbWhite   = color.RGBA{R: 245, G: 245, B: 245, A: 255}
	rgbGray    = color.RGBA{R: 210, G: 210, B: 210, A: 255}
)

var glyphs = map[rune][]string{
	' ': {"00000", "00000", "00000", "00000", "00000", "00000", "00000"},
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01111", "10000", "10000", "10011", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'I': {"11111", "00100", "00100", "00100", "00100", "00100", "11111"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'O': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	':': {"00000", "00100", "00100", "00000", "00100", "00100", "00000"},
	'-': {"00000", "00000", "00000", "11111", "00000", "00000", "00000"},
	'.': {"00000", "00000", "00000", "00000", "00000", "00110", "00110"},
}

func NewTestPattern() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, patternWidth, patternHeight))
	fillRect(img, img.Bounds(), rgbGray)

	fillRect(img, image.Rect(0, 0, patternWidth, 28), rgbBlack)
	drawRuler(img, 24, patternWidth-12)
	drawText(img, 8, 5, "422 CHROMA SITING", rgbWhite, 1)

	drawText(img, 14, 38, "COLORED TEXT EDGE", rgbRed, 2)
	drawText(img, 14, 60, "COLORED TEXT EDGE", rgbCyan, 2)

	verticals := []struct {
		x     int
		color color.RGBA
	}{
		{18, rgbBlue}, {19, rgbYellow}, {70, rgbMagenta}, {71, rgbGreen},
		{140, rgbRed}, {141, rgbCyan}, {210, rgbBlue}, {211, rgbYellow},
		{278, rgbMagenta}, {279, rgbGreen},
	}
	for _, line := range verticals {
		fillRect(img, image.Rect(line.x, 88, line.x+1, 166), line.color)
	}

	for i := 0; i < 72; i++ {
		x := 36 + i
		y := 100 + i/2
		img.SetRGBA(x, y, rgbBlue)
		img.SetRGBA(x, y+1, rgbYellow)
	}

	barColors := []color.RGBA{rgbRed, rgbYellow, rgbGreen, rgbCyan, rgbBlue, rgbMagenta}
	for i, c := range barColors {
		x := 130 + i*18
		fillRect(img, image.Rect(x, 126, x+17, 156), c)
	}

	drawText(img, 130, 160, "18PX COLOR BARS", rgbBlack, 1)
	return img
}

func drawRuler(img *image.RGBA, left, right int) {
	for x := left; x <= right; x++ {
		height := 3
		if x%10 == 0 {
			height = 11
		}
		fillRect(img, image.Rect(x, 17-height, x+1, 17), rgbWhite)
		if x%40 == 0 {
			drawText(img, x-8, 18, strconv.Itoa(x), rgbYellow, 1)
		}
	}
}

func drawText(img *image.RGBA, x, y int, text string, c color.RGBA, scale int) {
	cursor := x
	for _, r := range text {
		glyph, ok := glyphs[r]
		if !ok {
			r = ' '
			glyph = glyphs[r]
		}
		for row, bits := range glyph {
			for col := 0; col < len(bits); col++ {
				if bits[col] == '1' {
					rect := image.Rect(cursor+col*scale, y+row*scale, cursor+(col+1)*scale, y+(row+1)*scale)
					if rect.Overlaps(img.Bounds()) {
						fillRect(img, rect, c)
					}
				}
			}
		}
		cursor += 6 * scale
	}
}

func fillRect(img *image.RGBA, rect image.Rectangle, c color.RGBA) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func triangleChroma(x float64) float64 {
	period := 16.0
	position := math.Mod(x+period/2, period)
	if position < 0 {
		position += period
	}
	if position < period/2 {
		return 40 + position*22
	}
	return 216 - (position-period/2)*22
}
