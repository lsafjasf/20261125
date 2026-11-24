package main

import "fmt"

// renderHueContinuity plots the per-sample hue jump (degrees) along each
// trajectory for naive clipping versus soft compression. A flat line at
// zero means perfect hue continuity.
func renderHueContinuity(path string, red, blue, cyan trajectoryResult) error {
	c := newCanvas(1560, 560, colPaper)
	c.text(480, 16, "HUE CONTINUITY: STEP-WISE HUE JUMP (DEG)", colInk, 2)

	panels := []struct {
		title string
		res   trajectoryResult
		x0    int
	}{
		{"RED TRACK", red, 60},
		{"BLUE TRACK", blue, 560},
		{"CYAN TRACK", cyan, 1060},
	}

	for _, p := range panels {
		box := plotBox{x: p.x0, y: 80, w: 440, h: 380}
		xa := plotAxis{min: 0, max: 1, label: "T (PAPER WHITE -> SATURATED)",
			ticks: []float64{0, 0.25, 0.5, 0.75, 1}}
		ya := plotAxis{min: 0, max: hueAxisMax(p.res), label: "HUE JUMP (DEG)",
			ticks: hueTicks(p.res)}

		hardPts := make([]point2, len(p.res.s))
		softPts := make([]point2, len(p.res.s))
		for i, s := range p.res.s {
			hardPts[i] = point2{X: s.t, Y: s.hardHueStep}
			softPts[i] = point2{X: s.t, Y: s.softHueStep}
		}
		curves := []curve{
			{name: "HARD CLIP", pts: hardPts, col: colOrange, thick: 2, dots: true},
			{name: "SOFT COMPRESS", pts: softPts, col: colGreen, thick: 2, dots: true},
		}
		drawPlot(c, box, xa, ya, curves, p.title)
	}

	c.text(60, 510, "SOFT COMPRESSION HOLDS HUE FIXED IN LCh: EVERY SOFT STEP IS 0 DEG", colGreen, 1)
	return c.savePNG(path)
}

func hueAxisMax(r trajectoryResult) float64 {
	m := 1.0
	for _, s := range r.s {
		if s.hardHueStep > m {
			m = s.hardHueStep
		}
	}
	return m * 1.15
}

func hueTicks(r trajectoryResult) []float64 {
	m := hueAxisMax(r)
	step := m / 4
	out := make([]float64, 0, 5)
	for i := 0; i <= 4; i++ {
		out = append(out, float64(i)*step)
	}
	return out
}

func printSummary(red, blue, cyan trajectoryResult, src, dst Gamut) {
	fmt.Println("gamut mapping experiment")
	fmt.Printf("source=%s target=%s knee=%.2f samples=%d\n\n",
		src.name, dst.name, softKnee, samplesN)
	printOne := func(r trajectoryResult) {
		fmt.Printf("[%s] out-of-gamut samples: %d/%d\n", r.name, r.outCount, len(r.s))
		fmt.Printf("  hard clip     max dE00=%.4f  edge hue jump=%.4f deg  max shift=%.4f deg\n",
			r.maxHardDE, r.boundaryHardJump, r.maxHardShift)
		fmt.Printf("  soft compress max dE00=%.4f  edge hue jump=%.6f deg  max shift=%.6f deg\n",
			r.maxSoftDE, r.boundarySoftJump, r.maxSoftShift)
	}
	printOne(red)
	printOne(blue)
	printOne(cyan)
	fmt.Println("\nartifacts written to artifacts/")
}
