package main

import (
	"image"
	"image/png"
	"math"
	"os"
	"strconv"
)

type rgbc struct{ R, G, B uint8 }

var (
	colInk    = rgbc{34, 34, 34}
	colPaper  = rgbc{250, 250, 248}
	colGrid   = rgbc{214, 214, 210}
	colAxis   = rgbc{120, 120, 116}
	colRed    = rgbc{200, 36, 36}
	colBlue   = rgbc{36, 72, 200}
	colSrc    = rgbc{150, 150, 150}
	colDst    = rgbc{20, 20, 20}
	colGreen  = rgbc{34, 140, 80}
	colOrange = rgbc{220, 130, 20}
	colCyan   = rgbc{16, 150, 160}
)

type canvas struct {
	w, h int
	px   []uint8 // RGBA, fully opaque
}

func newCanvas(w, h int, bg rgbc) *canvas {
	c := &canvas{w: w, h: h, px: make([]uint8, w*h*4)}
	for i := 0; i < w*h; i++ {
		c.px[4*i] = bg.R
		c.px[4*i+1] = bg.G
		c.px[4*i+2] = bg.B
		c.px[4*i+3] = 255
	}
	return c
}

func (c *canvas) set(x, y int, col rgbc) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	i := 4 * (y*c.w + x)
	c.px[i], c.px[i+1], c.px[i+2], c.px[i+3] = col.R, col.G, col.B, 255
}

func (c *canvas) fillRect(x, y, w, h int, col rgbc) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			c.set(xx, yy, col)
		}
	}
}

func (c *canvas) line(x0, y0, x1, y1 int, col rgbc, thick int, dashed bool) {
	dx := math.Abs(float64(x1 - x0))
	dy := -math.Abs(float64(y1 - y0))
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	dist := 0
	for {
		if !dashed || (dist/9)%2 == 0 {
			for ox := -thick / 2; ox <= thick/2; ox++ {
				for oy := -thick / 2; oy <= thick/2; oy++ {
					c.set(x0+ox, y0+oy, col)
				}
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
		dist++
	}
}

func (c *canvas) dot(cx, cy, r int, col rgbc) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				c.set(cx+x, cy+y, col)
			}
		}
	}
}

func textWidth(s string, scale int) int {
	return len(s) * 6 * scale
}

func (c *canvas) text(x, y int, s string, col rgbc, scale int) {
	for _, r := range s {
		glyph, ok := font5x7[r]
		if !ok {
			glyph = font5x7['*']
		}
		for row := 0; row < 7; row++ {
			for gx := 0; gx < 5; gx++ {
				if glyph[row][gx] == '#' {
					for oy := 0; oy < scale; oy++ {
						for ox := 0; ox < scale; ox++ {
							c.set(x+gx*scale+ox, y+row*scale+oy, col)
						}
					}
				}
			}
		}
		x += 6 * scale
	}
}

func (c *canvas) savePNG(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
	copy(img.Pix, c.px)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

type point2 struct{ X, Y float64 }

type curve struct {
	name   string
	pts    []point2
	col    rgbc
	thick  int
	dashed bool
	dots   bool
}

type plotBox struct{ x, y, w, h int }

type plotAxis struct {
	min, max float64
	label    string
	ticks    []float64
}

func tickLabel(v float64) string {
	if math.Abs(v-math.Round(v)) < 1e-9 {
		return strconv.Itoa(int(math.Round(v)))
	}
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', 1, 64)
}

// drawPlot renders a 2D line chart with axes, grid, ticks and a legend.
func drawPlot(c *canvas, box plotBox, xa, ya plotAxis, curves []curve, title string) {
	c.text(box.x, box.y-26, title, colInk, 2)

	mapX := func(v float64) int {
		return box.x + int((v-xa.min)/(xa.max-xa.min)*float64(box.w))
	}
	mapY := func(v float64) int {
		return box.y + box.h - int((v-ya.min)/(ya.max-ya.min)*float64(box.h))
	}

	// Grid + tick labels.
	for _, t := range xa.ticks {
		px := mapX(t)
		c.line(px, box.y, px, box.y+box.h, colGrid, 1, false)
		lbl := tickLabel(t)
		c.text(px-textWidth(lbl, 1)/2, box.y+box.h+8, lbl, colInk, 1)
	}
	for _, t := range ya.ticks {
		py := mapY(t)
		c.line(box.x, py, box.x+box.w, py, colGrid, 1, false)
		lbl := tickLabel(t)
		c.text(box.x-textWidth(lbl, 1)-6, py-4, lbl, colInk, 1)
	}

	// Frame.
	c.line(box.x, box.y, box.x, box.y+box.h, colAxis, 1, false)
	c.line(box.x, box.y+box.h, box.x+box.w, box.y+box.h, colAxis, 1, false)

	// Axis labels.
	c.text(box.x+box.w/2-textWidth(xa.label, 1)/2, box.y+box.h+24, xa.label, colInk, 1)
	yl := ya.label
	for i, r := range yl {
		c.text(box.x-44, box.y+box.h/2-len(yl)*4+i*9, string(r), colInk, 1)
	}

	// Curves.
	for _, cv := range curves {
		var prevX, prevY int
		have := false
		for _, p := range cv.pts {
			px, py := mapX(p.X), mapY(p.Y)
			if have {
				c.line(prevX, prevY, px, py, cv.col, cv.thick, cv.dashed)
			}
			if cv.dots {
				c.dot(px, py, 2, cv.col)
			}
			prevX, prevY = px, py
			have = true
		}
	}

	// Legend (top-left inside the box).
	ly := box.y + 8
	for _, cv := range curves {
		c.line(box.x+10, ly+4, box.x+34, ly+4, cv.col, 2, cv.dashed)
		c.text(box.x+42, ly, cv.name, colInk, 1)
		ly += 14
	}
}
