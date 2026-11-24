package main

import "math"

func main() {
	src := newSRGBGamut()
	// Narrower display (~half the chroma volume). Primaries move toward
	// D65 while paper white is kept fixed.
	dst := newNarrowGamut("NARROW-D65", 0.4)

	paperWhite := Lab{L: 100, A: 0, B: 0}
	redEnd := xyzToLab(srgbRGBToXYZ(LinRGB{R: 1, G: 0, B: 0}))
	blueEnd := xyzToLab(srgbRGBToXYZ(LinRGB{R: 0, G: 0, B: 1}))
	cyanEnd := xyzToLab(srgbRGBToXYZ(LinRGB{R: 0, G: 1, B: 1}))

	red := runTrajectory("RED", paperWhite, redEnd, dst)
	blue := runTrajectory("BLUE", paperWhite, blueEnd, dst)
	cyan := runTrajectory("CYAN", paperWhite, cyanEnd, dst)

	must(writeTrajectoryCSV("artifacts/track_red.csv", red))
	must(writeTrajectoryCSV("artifacts/track_blue.csv", blue))
	must(writeTrajectoryCSV("artifacts/track_cyan.csv", cyan))
	must(writeBoundaryCSV("artifacts/boundary_L50.csv", src, dst, 50))
	must(writeMetrics("artifacts/metrics.txt", red, blue, cyan, src, dst))
	must(renderBoundaryMap("artifacts/boundary_map.png", src, dst, red, blue, cyan, 50))
	must(renderHueContinuity("artifacts/hue_continuity.png", red, blue, cyan))

	printSummary(red, blue, cyan, src, dst)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// renderBoundaryMap draws the L=50 LCh boundary of both gamuts and overlays
// the red/blue trajectories (original, clipped and soft-compressed).
func renderBoundaryMap(path string, src, dst Gamut, red, blue, cyan trajectoryResult, L float64) error {
	c := newCanvas(1000, 760, colPaper)
	c.text(270, 18, "GAMUT BOUNDARY MAP (L*=50, LCh PLANE)", colInk, 2)

	box := plotBox{x: 90, y: 70, w: 860, h: 620}
	xMax, yMax := 135.0, 135.0

	mapX := func(p LCh) int {
		rad := deg2rad(p.H)
		return box.x + box.w/2 + int(p.C*math.Cos(rad)/xMax*float64(box.w/2))
	}
	mapY := func(p LCh) int {
		rad := deg2rad(p.H)
		return box.y + box.h/2 - int(p.C*math.Sin(rad)/yMax*float64(box.h/2))
	}

	// Light grid rings + spokes.
	for r := 25.0; r <= xMax; r += 25 {
		var ring []point2
		for h := 0; h <= 360; h += 3 {
			rad := deg2rad(float64(h))
			ring = append(ring, point2{
				X: float64(box.x+box.w/2) + r*math.Cos(rad)/xMax*float64(box.w/2),
				Y: float64(box.y+box.h/2) - r*math.Sin(rad)/yMax*float64(box.h/2),
			})
		}
		drawPolyline(c, ring, colGrid, 1, false)
	}
	for h := 0; h < 360; h += 30 {
		edge := LCh{L: L, C: xMax, H: float64(h)}
		c.line(box.x+box.w/2, box.y+box.h/2, mapX(edge), mapY(edge), colGrid, 1, false)
	}

	boundaryCurve := func(g Gamut, col rgbc, dashed bool) {
		var pts []point2
		for h := 0; h <= 360; h += 2 {
			bc := g.maxChroma(L, float64(h))
			rad := deg2rad(float64(h))
			pts = append(pts, point2{
				X: float64(box.x+box.w/2) + bc*math.Cos(rad)/xMax*float64(box.w/2),
				Y: float64(box.y+box.h/2) - bc*math.Sin(rad)/yMax*float64(box.h/2),
			})
		}
		drawPolyline(c, pts, col, 2, dashed)
	}
	boundaryCurve(src, colSrc, true)
	boundaryCurve(dst, colDst, false)

	drawTrack := func(r trajectoryResult, base rgbc, pick func(sample) Lab) {
		var pts []point2
		for _, s := range r.s {
			p := labToLCh(pick(s))
			pts = append(pts, point2{X: float64(mapX(p)), Y: float64(mapY(p))})
		}
		drawPolyline(c, pts, base, 2, false)
	}
	drawTrack(red, colRed, func(s sample) Lab { return s.orig })
	drawTrack(red, colOrange, func(s sample) Lab { return s.hard })
	drawTrack(red, colGreen, func(s sample) Lab { return s.soft })
	drawTrack(blue, colBlue, func(s sample) Lab { return s.orig })
	drawTrack(blue, colOrange, func(s sample) Lab { return s.hard })
	drawTrack(blue, colGreen, func(s sample) Lab { return s.soft })
	drawTrack(cyan, colCyan, func(s sample) Lab { return s.orig })
	drawTrack(cyan, colOrange, func(s sample) Lab { return s.hard })
	drawTrack(cyan, colGreen, func(s sample) Lab { return s.soft })

	// Legend.
	legend := []struct {
		label string
		col   rgbc
		dash  bool
	}{
		{"SRC BOUNDARY (SRGB)", colSrc, true},
		{"DST BOUNDARY (NARROW)", colDst, false},
		{"RED ORIGINAL", colRed, false},
		{"BLUE ORIGINAL", colBlue, false},
		{"CYAN ORIGINAL", colCyan, false},
		{"HARD CLIP (ALL)", colOrange, false},
		{"SOFT COMPRESS (ALL)", colGreen, false},
	}
	lx, ly := box.x+12, box.y+10
	for _, e := range legend {
		c.line(lx, ly+4, lx+26, ly+4, e.col, 2, e.dash)
		c.text(lx+34, ly, e.label, colInk, 1)
		ly += 15
	}
	c.text(box.x+box.w/2-30, box.y+box.h+28, "A* (HUE 0 DEG)", colInk, 1)
	c.text(box.x-70, box.y+box.h/2, "B* (HUE 90 DEG)", colInk, 1)
	return c.savePNG(path)
}
func drawPolyline(c *canvas, pts []point2, col rgbc, thick int, dashed bool) {
	if len(pts) < 2 {
		return
	}
	for i := 1; i < len(pts); i++ {
		c.line(int(pts[i-1].X), int(pts[i-1].Y), int(pts[i].X), int(pts[i].Y), col, thick, dashed)
	}
}
