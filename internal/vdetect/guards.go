package vdetect

import (
	"math"
)

type motionInfo struct {
	guards     Guards
	mask       []bool
	gw, gh     int
	dx, dy     int
	effect     []EffectRegion
	foreground []EffectRegion
}

func analyzeMotion(f *Frame, g, prevG *GrayFrame, brightnessSlope float64) motionInfo {
	mean := f.MeanLuma()
	info := motionInfo{gw: g.W, gh: g.H}
	info.guards = Guards{MeanLuma: round2(mean), BrightnessSlope: round3(brightnessSlope)}
	if brightnessSlope > 0.35 {
		info.guards.GlobalBrightening = true
	}
	if prevG == nil || prevG.W != g.W || prevG.H != g.H {
		return info
	}

	prevMean := 0.0
	for _, v := range prevG.Y {
		prevMean += float64(v)
	}
	prevMean /= float64(len(prevG.Y))
	info.guards.LumaDelta = round2(mean - prevMean)
	if info.guards.LumaDelta > 1.2 && brightnessSlope >= 0.25 {
		info.guards.GlobalBrightening = true
	}

	maxD := clampInt(g.W/38, 5, 14)
	bestDX, bestDY, bestCost := 0, 0, math.MaxFloat64
	curMean := mean
	for dy := -1; dy <= 1; dy++ {
		for dx := -maxD; dx <= maxD; dx++ {
			cost := shiftedSAD(g, prevG, dx, dy, curMean, prevMean)
			if cost < bestCost {
				bestCost, bestDX, bestDY = cost, dx, dy
			}
		}
	}
	info.dx, info.dy = bestDX, bestDY
	info.guards.CameraShiftPx = round2(float64(bestDX) * g.ScaleX)

	mask := make([]bool, g.W*g.H)
	var motionCount, foregroundCount, valid int
	for y := 1; y < g.H-1; y++ {
		py := y - bestDY
		if py < 0 || py >= prevG.H {
			continue
		}
		for x := 1; x < g.W-1; x++ {
			px := x - bestDX
			if px < 0 || px >= prevG.W {
				continue
			}
			i, j := y*g.W+x, py*prevG.W+px
			cv := math.Abs((float64(g.Y[i]) - curMean) - (float64(prevG.Y[j]) - prevMean))
			valid++
			if cv > 24 {
				mask[i] = true
				motionCount++
				if cv > 34 {
					foregroundCount++
				}
			}
		}
	}
	info.mask = mask
	info.guards.MotionCoverage = round3(float64(motionCount) / float64(maxInt(1, valid)))
	info.guards.ForegroundCoverage = round3(float64(foregroundCount) / float64(maxInt(1, valid)))

	effectCells, fgCells := classifyMotionCells(f, g, mask)
	info.effect = cellsToRegions(effectCells, g, "game_effect")
	info.foreground = cellsToRegions(fgCells, g, "moving_foreground")
	effectPixels := 0
	for _, c := range effectCells {
		effectPixels += (c.x1 - c.x0) * (c.y1 - c.y0)
	}
	info.guards.EffectCoverage = round3(float64(effectPixels) / float64(g.W*g.H))
	return info
}

func shiftedSAD(g, p *GrayFrame, dx, dy int, meanG, meanP float64) float64 {
	var sum float64
	n := 0
	for y := 2; y < g.H-2; y++ {
		py := y + dy
		if py < 1 || py >= p.H-1 {
			continue
		}
		for x := 2; x < g.W-2; x++ {
			px := x + dx
			if px < 1 || px >= p.W-1 {
				continue
			}
			a := float64(g.Y[y*g.W+x]) - meanG
			b := float64(p.Y[py*p.W+px]) - meanP
			sum += math.Abs(a - b)
			n++
		}
	}
	return sum / float64(maxInt(1, n))
}

type gridCell struct {
	x0, y0, x1, y1 int
	effect         bool
}

func classifyMotionCells(f *Frame, g *GrayFrame, mask []bool) ([]gridCell, []gridCell) {
	const cw, ch = 28, 20
	cols := (g.W + cw - 1) / cw
	rows := (g.H + ch - 1) / ch
	effectMark := make([][]bool, rows)
	fgMark := make([][]bool, rows)
	for cy := 0; cy < rows; cy++ {
		effectMark[cy] = make([]bool, cols)
		fgMark[cy] = make([]bool, cols)
		for cx := 0; cx < cols; cx++ {
			x0, y0 := cx*cw, cy*ch
			x1, y1 := minInt(g.W, x0+cw), minInt(g.H, y0+ch)
			var motion, n int
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					n++
					if mask[y*g.W+x] {
						motion++
					}
				}
			}
			ratio := float64(motion) / float64(maxInt(1, n))
			if ratio < 0.22 {
				continue
			}
			sat, bright := sampleColorStats(f, g, x0, y0, x1, y1)
			if ratio > 0.30 && sat > 48 && bright > 105 {
				effectMark[cy][cx] = true
			} else if ratio > 0.34 {
				fgMark[cy][cx] = true
			}
		}
	}
	return markedCellComponents(effectMark, cw, ch, true), markedCellComponents(fgMark, cw, ch, false)
}

func sampleColorStats(f *Frame, g *GrayFrame, x0, y0, x1, y1 int) (float64, float64) {
	if len(f.R) == 0 {
		return 0, float64(200)
	}
	var satSum, brightSum float64
	var n int
	stepX := maxInt(1, int(g.ScaleX))
	stepY := maxInt(1, int(g.ScaleY))
	for gy := y0; gy < y1; gy += 4 {
		sy := clampInt(int(float64(gy)*g.ScaleY), 0, f.H-1)
		for gx := x0; gx < x1; gx += 4 {
			sx := clampInt(int(float64(gx)*g.ScaleX), 0, f.W-1)
			i := sy*f.W + sx
			r, gg, b := int(f.R[i]), int(f.G[i]), int(f.B[i])
			mx := maxInt(r, maxInt(gg, b))
			mn := minInt(r, minInt(gg, b))
			satSum += float64(mx - mn)
			brightSum += float64(mx)
			n++
		}
	}
	_ = stepX
	_ = stepY
	return satSum / float64(maxInt(1, n)), brightSum / float64(maxInt(1, n))
}

func markedCellComponents(mark [][]bool, cw, ch int, _ bool) []gridCell {
	if len(mark) == 0 {
		return nil
	}
	rows, cols := len(mark), len(mark[0])
	seen := make([][]bool, rows)
	for r := range seen {
		seen[r] = make([]bool, cols)
	}
	var out []gridCell
	for sy := 0; sy < rows; sy++ {
		for sx := 0; sx < cols; sx++ {
			if !mark[sy][sx] || seen[sy][sx] {
				continue
			}
			queue := [][2]int{{sy, sx}}
			seen[sy][sx] = true
			minX, minY, maxX, maxY := sx, sy, sx+1, sy+1
			count := 0
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				y, x := cur[0], cur[1]
				count++
				minX, minY = minInt(minX, x), minInt(minY, y)
				maxX, maxY = maxInt(maxX, x+1), maxInt(maxY, y+1)
				for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					ny, nx := y+d[0], x+d[1]
					if ny >= 0 && ny < rows && nx >= 0 && nx < cols && mark[ny][nx] && !seen[ny][nx] {
						seen[ny][nx] = true
						queue = append(queue, [2]int{ny, nx})
					}
				}
			}
			if count >= 1 {
				out = append(out, gridCell{x0: minX * cw, y0: minY * ch, x1: maxX * cw, y1: maxY * ch})
			}
		}
	}
	return out
}

func cellsToRegions(cells []gridCell, g *GrayFrame, kind string) []EffectRegion {
	out := make([]EffectRegion, 0, len(cells))
	for _, c := range cells {
		x := int(math.Floor(float64(c.x0) * g.ScaleX))
		y := int(math.Floor(float64(c.y0) * g.ScaleY))
		x2 := int(math.Ceil(float64(c.x1) * g.ScaleX))
		y2 := int(math.Ceil(float64(c.y1) * g.ScaleY))
		out = append(out, EffectRegion{
			Box:  Box{X: x, Y: y, W: maxInt(1, x2-x), H: maxInt(1, y2-y), Conf: 0.55, Reason: kind},
			Kind: kind,
		})
	}
	return out
}

func brightnessSlope(values []float64) float64 {
	n := len(values)
	if n < 3 {
		return 0
	}
	if n > 12 {
		values = values[n-12:]
		n = len(values)
	}
	var sx, sy, sxx, sxy float64
	for i, v := range values {
		x := float64(i)
		sx += x
		sy += v
		sxx += x * x
		sxy += x * v
	}
	den := float64(n)*sxx - sx*sx
	if math.Abs(den) < 1e-9 {
		return 0
	}
	return (float64(n)*sxy - sx*sy) / den
}
