package main

import (
	"image"
	"image/color"
)

var labelColors = map[string][3]uint8{
	LabelBlocking:    {255, 64, 64},
	LabelRinging:     {255, 160, 0},
	LabelBandingComp: {80, 160, 255},
	LabelBandingTone: {180, 110, 255},
	LabelMotion:      {60, 220, 120},
	LabelRealEdge:    {200, 200, 200},
	LabelTextEdge:    {230, 230, 120},
	LabelClean:       {0, 0, 0},
}

// overlayImage draws the luma frame plus semi-transparent label heatmap.
func overlayImage(f *Frame, tiles []TileReport, regions []Region, grid bool) image.Image {
	w, h := f.W, f.H
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, v := range f.Y {
		img.Pix[i*4] = v
		img.Pix[i*4+1] = v
		img.Pix[i*4+2] = v
		img.Pix[i*4+3] = 255
	}
	alpha := uint8(110)
	for _, t := range tiles {
		rgb, ok := labelColors[t.Label]
		if !ok || !isArtifact(t.Label) {
			continue
		}
		a := alpha
		if t.Persistent {
			a = 150
		}
		blendTile(img, t.X, t.Y, t.Size, rgb, a)
	}
	if grid {
		drawGrid(img, 8, color.RGBA{255, 255, 255, 26})
		drawGrid(img, 16, color.RGBA{255, 255, 255, 45})
	}
	for idx, r := range regions {
		if idx >= 8 {
			break
		}
		drawRect(img, r.X, r.Y, r.W, r.H, color.RGBA{255, 255, 255, 220})
	}
	return img
}

func blendTile(img *image.RGBA, x, y, size int, rgb [3]uint8, a uint8) {
	for yy := y; yy < y+size; yy++ {
		for xx := x; xx < x+size; xx++ {
			i := img.PixOffset(xx, yy)
			af := float64(a) / 255
			img.Pix[i] = uint8(float64(img.Pix[i])*(1-af) + float64(rgb[0])*af)
			img.Pix[i+1] = uint8(float64(img.Pix[i+1])*(1-af) + float64(rgb[1])*af)
			img.Pix[i+2] = uint8(float64(img.Pix[i+2])*(1-af) + float64(rgb[2])*af)
		}
	}
}

func drawGrid(img *image.RGBA, step int, c color.RGBA) {
	for x := 0; x < img.Bounds().Dx(); x += step {
		drawVLine(img, x, c)
	}
	for y := 0; y < img.Bounds().Dy(); y += step {
		drawHLine(img, y, c)
	}
}

func drawVLine(img *image.RGBA, x int, c color.RGBA) {
	for y := 0; y < img.Bounds().Dy(); y++ {
		set(img, x, y, c)
	}
}

func drawHLine(img *image.RGBA, y int, c color.RGBA) {
	for x := 0; x < img.Bounds().Dx(); x++ {
		set(img, x, y, c)
	}
}

func drawRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for xx := x; xx < x+w; xx++ {
		set(img, xx, y, c)
		set(img, xx, y+h-1, c)
	}
	for yy := y; yy < y+h; yy++ {
		set(img, x, yy, c)
		set(img, x+w-1, yy, c)
	}
}

func set(img *image.RGBA, x, y int, c color.RGBA) {
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return
	}
	i := img.PixOffset(x, y)
	af := float64(c.A) / 255
	img.Pix[i] = uint8(float64(img.Pix[i])*(1-af) + float64(c.R)*af)
	img.Pix[i+1] = uint8(float64(img.Pix[i+1])*(1-af) + float64(c.G)*af)
	img.Pix[i+2] = uint8(float64(img.Pix[i+2])*(1-af) + float64(c.B)*af)
}

// zoomImage returns a nearest-neighbor enlarged crop with optional codec grid,
// keeping individual pixels visible for manual artifact inspection.
func zoomImage(f *Frame, region Region, tileSize, scale int, grid bool) image.Image {
	pad := tileSize
	x0 := maxInt(region.X-pad, 0)
	y0 := maxInt(region.Y-pad, 0)
	x1 := minInt(region.X+region.W+pad, f.W)
	y1 := minInt(region.Y+region.H+pad, f.H)
	cw, ch := x1-x0, y1-y0
	out := image.NewRGBA(image.Rect(0, 0, cw*scale, ch*scale))
	for yy := 0; yy < ch; yy++ {
		for xx := 0; xx < cw; xx++ {
			v := f.Y[(y0+yy)*f.W+x0+xx]
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					i := out.PixOffset(xx*scale+sx, yy*scale+sy)
					out.Pix[i], out.Pix[i+1], out.Pix[i+2] = v, v, v
					out.Pix[i+3] = 255
				}
			}
		}
	}
	if grid {
		drawScaledGrid(out, x0, y0, cw, ch, scale)
	}
	rx0 := (region.X - x0) * scale
	ry0 := (region.Y - y0) * scale
	drawRect(out, rx0, ry0, region.W*scale, region.H*scale,
		color.RGBA{255, 255, 255, 255})
	return out
}

func drawScaledGrid(img *image.RGBA, ox, oy, cw, ch, scale int) {
	for gx := (ox / 8) * 8; gx < ox+cw; gx++ {
		if gx%8 != 0 || gx < ox {
			continue
		}
		x := (gx - ox) * scale
		for k := 0; k < scale; k++ {
			drawVLine(img, minInt(x+k, img.Bounds().Dx()-1),
				color.RGBA{255, 255, 255, 40})
		}
	}
	for gy := (oy / 8) * 8; gy < oy+ch; gy++ {
		if gy%8 != 0 || gy < oy {
			continue
		}
		y := (gy - oy) * scale
		for k := 0; k < scale; k++ {
			drawHLine(img, minInt(y+k, img.Bounds().Dy()-1),
				color.RGBA{255, 255, 255, 40})
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
