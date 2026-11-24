package vdetect

import (
	"math"
)

// detectBlockDefects finds grid-aligned coding damage rather than scene edges.
func detectBlockDefects(f, prev *Frame, opt Options) []BlockDefect {
	delta := 0.0
	if prev != nil && prev.W == f.W && prev.H == f.H {
		delta = f.MeanLuma() - prev.MeanLuma()
	}
	return detectBlockDefectsDelta(f, prev, delta, opt)
}

func detectBlockDefectsDelta(f, prev *Frame, lumaDelta float64, opt Options) []BlockDefect {
	if f.W < 32 || f.H < 32 {
		return nil
	}
	sensitivity := clamp(opt.Sensitivity, 0.1, 1)
	type candidate struct {
		bd BlockDefect
	}
	var cands []candidate

	for _, size := range []int{16, 8} {
		// Eight-pixel responses are retained only when they reinforce a 16px region.
		minConf := 0.50 - 0.10*sensitivity
		if size == 8 {
			minConf += 0.08
		}
		for y := 0; y+size <= f.H; y += size {
			for x := 0; x+size <= f.W; x += size {
				bd, ok := scoreBlock(f, prev, x, y, size, sensitivity, lumaDelta)
				if !ok || bd.Conf < minConf {
					continue
				}
				cands = append(cands, candidate{bd})
			}
		}
	}

	// Suppress isolated 8px reports unless they overlap a 16px report; return 16px units.
	var out []BlockDefect
	for _, c := range cands {
		if c.bd.BlockSize == 16 {
			out = append(out, c.bd)
		}
	}
	for _, c := range cands {
		if c.bd.BlockSize != 8 {
			continue
		}
		reinforced := false
		for i := range out {
			if boxesOverlap(c.bd.Box, out[i].Box) {
				reinforced = true
				out[i].Conf = clamp01(out[i].Conf + 0.08)
				out[i].Evidence = appendUnique(out[i].Evidence, "eight_pixel_grid_response")
				break
			}
		}
		if reinforced {
			continue
		}
		near16 := false
		for ox := -16; ox <= 16; ox += 16 {
			for oy := -16; oy <= 16; oy += 16 {
				if ox == 0 && oy == 0 {
					continue
				}
				nx, ny := c.bd.X+ox, c.bd.Y+oy
				if nx&15 == 0 && ny&15 == 0 {
					if blockActuallyStrong(f, nx, ny, 16, lumaDelta) {
						near16 = true
					}
				}
			}
		}
		if !near16 && c.bd.Conf >= 0.80 {
			out = append(out, c.bd)
		}
	}

	for i := range out {
		out[i].Conf = round3(out[i].Conf)
		out[i].Boundary = round2(out[i].Boundary)
		out[i].Internal = round2(out[i].Internal)
		out[i].ColorScore = round2(out[i].ColorScore)
		out[i].Temporal = round2(out[i].Temporal)
	}
	return out
}

func scoreBlock(f, prev *Frame, x0, y0, s int, sensitivity, lumaDelta float64) (BlockDefect, bool) {
	w, h := f.W, f.H
	var innerSum float64
	var innerN int
	for y := y0; y < y0+s; y++ {
		for x := x0; x < x0+s; x++ {
			i := y*w + x
			if x+1 < x0+s && x+1 < w {
				innerSum += math.Abs(float64(f.Y[i]) - float64(f.Y[i+1]))
				innerN++
			}
			if y+1 < y0+s && y+1 < h {
				innerSum += math.Abs(float64(f.Y[i]) - float64(f.Y[i+w]))
				innerN++
			}
		}
	}
	inner := innerSum / float64(maxInt(1, innerN))

	sides := make([]float64, 0, 4)
	strongSides := 0
	perimeterSides := 0
	addSide := func(v float64, coverage float64) {
		sides = append(sides, v)
		threshold := math.Max(22, inner*1.8+8)
		if v > threshold {
			strongSides++
		}
		if coverage >= 0.55 && v >= 18 {
			perimeterSides++
		}
	}
	if y0 > 0 {
		var sum, hot float64
		for x := x0; x < x0+s; x++ {
			d := math.Abs(float64(f.Y[(y0-1)*w+x]) - float64(f.Y[y0*w+x]))
			sum += d
			if d > 22 {
				hot++
			}
		}
		addSide(sum/float64(s), hot/float64(s))
	}
	if y0+s < h {
		var sum, hot float64
		for x := x0; x < x0+s; x++ {
			d := math.Abs(float64(f.Y[(y0+s-1)*w+x]) - float64(f.Y[(y0+s)*w+x]))
			sum += d
			if d > 22 {
				hot++
			}
		}
		addSide(sum/float64(s), hot/float64(s))
	}
	if x0 > 0 {
		var sum, hot float64
		for y := y0; y < y0+s; y++ {
			d := math.Abs(float64(f.Y[y*w+x0-1]) - float64(f.Y[y*w+x0]))
			sum += d
			if d > 22 {
				hot++
			}
		}
		addSide(sum/float64(s), hot/float64(s))
	}
	if x0+s < w {
		var sum, hot float64
		for y := y0; y < y0+s; y++ {
			d := math.Abs(float64(f.Y[y*w+x0+s-1]) - float64(f.Y[y*w+x0+s]))
			sum += d
			if d > 22 {
				hot++
			}
		}
		addSide(sum/float64(s), hot/float64(s))
	}
	if len(sides) < 2 {
		return BlockDefect{}, false
	}
	var boundary float64
	for _, v := range sides {
		boundary += v
	}
	boundary /= float64(len(sides))

	color := colorArtifact(f, x0, y0, s)
	temporal := temporalBlockScore(f, prev, x0, y0, s, lumaDelta)
	variance := blockVariance(f, x0, y0, s)
	flatAnomaly := inner < 6.5 && boundary > 22 && strongSides >= 1
	noiseAnomaly := inner > 16 && variance > 150 && boundary > 18
	edgeSpike := boundary > math.Max(24, inner*1.55) && strongSides >= 2
	temporalAnomaly := temporal >= 30 && inner >= 12 && boundary >= 18 && perimeterSides >= 2
	colorConcealment := color >= 0.72 && boundary >= 22 && perimeterSides >= 2
	if !(flatAnomaly || noiseAnomaly || edgeSpike || temporalAnomaly || colorConcealment) {
		dbg := BlockDefect{Box: Box{X: x0, Y: y0, W: s, H: s}, BlockSize: s, Boundary: boundary, Internal: inner, ColorScore: color, Temporal: temporal}
		return dbg, false
	}

	sideFraction := float64(maxInt(strongSides, perimeterSides)) / float64(len(sides))
	boundaryScore := clamp01(boundary / 55)
	excess := clamp01((boundary - inner*1.25) / 28)
	anomaly := 0.0
	if flatAnomaly {
		anomaly = 0.75
	}
	if noiseAnomaly {
		anomaly = math.Max(anomaly, clamp01((inner-14)/28)+0.25)
	}
	if edgeSpike {
		anomaly = math.Max(anomaly, 0.55*excess+0.2)
	}
	if temporalAnomaly {
		anomaly = math.Max(anomaly, 0.72)
	}
	if colorConcealment {
		anomaly = math.Max(anomaly, 0.82)
	}
	conf := 0.10 + 0.24*sideFraction + 0.20*boundaryScore + 0.20*clamp01(anomaly) +
		0.13*clamp01(color/0.55) + 0.13*clamp01(temporal/45)
	conf *= 0.88 + 0.24*sensitivity
	if s == 8 {
		conf *= 0.92
	}
	evidence := []string{"macroblock_grid"}
	if temporalAnomaly {
		evidence = append(evidence, "abrupt_temporal_content")
	}
	if colorConcealment {
		evidence = append(evidence, "concealment_color_cast")
	}
	if edgeSpike || flatAnomaly {
		evidence = append(evidence, "block_boundary_spike")
	}
	if flatAnomaly {
		evidence = append(evidence, "flat_concealment_texture")
	}
	if noiseAnomaly {
		evidence = append(evidence, "high_frequency_noise_texture")
	}
	if temporal < 8 && color < 0.45 {
		conf *= 0.70
		evidence = append(evidence, "single_frame_ambiguity")
	}

	bd := BlockDefect{
		Box:       Box{X: x0, Y: y0, W: s, H: s, Conf: round3(clamp01(conf)), Reason: "grid_aligned_coding_anomaly"},
		BlockSize: s, Boundary: boundary, Internal: inner, ColorScore: color, Temporal: temporal,
		GridAligned: true, Evidence: evidence,
	}
	return bd, conf >= 0.48
}

func blockVariance(f *Frame, x0, y0, s int) float64 {
	var sum, sum2 float64
	n := 0
	for y := y0; y < y0+s; y++ {
		for x := x0; x < x0+s; x++ {
			v := float64(f.Y[y*f.W+x])
			sum += v
			sum2 += v * v
			n++
		}
	}
	mean := sum / float64(n)
	return math.Max(0, sum2/float64(n)-mean*mean)
}

func colorArtifact(f *Frame, x0, y0, s int) float64 {
	if len(f.R) == 0 || len(f.G) == 0 || len(f.B) == 0 {
		return 0
	}
	var r, g, b float64
	n := 0
	for y := y0; y < y0+s; y++ {
		for x := x0; x < x0+s; x++ {
			i := y*f.W + x
			r += float64(f.R[i])
			g += float64(f.G[i])
			b += float64(f.B[i])
			n++
		}
	}
	r, g, b = r/float64(n), g/float64(n), b/float64(n)
	greenCast := clamp01((g - math.Max(r, b) - 28) / 45)
	magentaCast := 0.0
	if r > 110 && b > 110 && math.Abs(r-b) < 85 {
		magentaCast = clamp01((math.Min(r, b) - g - 28) / 45)
	}
	chroma := math.Sqrt((r-g)*(r-g)+(b-g)*(b-g)) / 180
	return clamp01(math.Max(greenCast, magentaCast)*0.85 + chroma*0.18)
}

func temporalBlockScore(f, prev *Frame, x0, y0, s int, lumaDelta float64) float64 {
	if prev == nil || prev.W != f.W || prev.H != f.H {
		return 0
	}
	var sum float64
	n := 0
	for y := y0; y < y0+s; y++ {
		for x := x0; x < x0+s; x++ {
			i := y*f.W + x
			j := y*prev.W + x
			diff := float64(int(f.Y[i])-int(prev.Y[j])) - lumaDelta
			sum += math.Abs(diff)
			n++
		}
	}
	return sum / float64(maxInt(1, n))
}

func blockActuallyStrong(f *Frame, x, y, s int, lumaDelta float64) bool {
	if x < 0 || y < 0 || x+s > f.W || y+s > f.H {
		return false
	}
	bd, ok := scoreBlock(f, nil, x, y, s, DefaultOptions().Sensitivity, lumaDelta)
	return ok && bd.Conf >= 0.5
}

func boxesOverlap(a, b Box) bool {
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}

func appendUnique(in []string, s string) []string {
	for _, v := range in {
		if v == s {
			return in
		}
	}
	return append(in, s)
}
