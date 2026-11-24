package cutluma

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type svgBox struct {
	x, y, w, h float64
	yMin, yMax float64
}

func (b svgBox) yMap(v float64) float64 {
	return b.y + b.h - (clampF(v, b.yMin, b.yMax)-b.yMin)/(b.yMax-b.yMin)*b.h
}

func linePath(vals []float64, xAt func(int) float64, b svgBox) string {
	var sb strings.Builder
	for i, v := range vals {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%.1f,%.1f", xAt(i), b.yMap(v))
	}
	return sb.String()
}

const (
	colorRaw      = "#dc2626"
	colorRepaired = "#1d4ed8"
	colorSource   = "#059669"
	colorCut      = "#6b7280"
	colorGrid     = "#e5e7eb"
)

// WriteSVG renders the full timeline and nine cross-cut zoom panels.
func WriteSVG(path string, job *Job, rep *Report, plan *RepairPlan) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprint(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="1240" height="1060" font-family="Helvetica,Arial,sans-serif">`)
	fmt.Fprint(&b, `<rect width="1240" height="1060" fill="#ffffff"/>`)
	fmt.Fprint(&b, `<text x="20" y="30" font-size="18" font-weight="bold" fill="#111827">Ten-segment color conversion: luma across cut points</text>`)
	fmt.Fprint(&b, `<line x1="850" y1="24" x2="880" y2="24" stroke="`+colorRaw+`" stroke-width="2"/><text x="886" y="28" font-size="12" fill="#374151">raw output</text>`)
	fmt.Fprint(&b, `<line x1="970" y1="24" x2="1000" y2="24" stroke="`+colorRepaired+`" stroke-width="2"/><text x="1006" y="28" font-size="12" fill="#374151">repaired</text>`)
	fmt.Fprint(&b, `<line x1="1090" y1="24" x2="1120" y2="24" stroke="`+colorSource+`" stroke-width="2" stroke-dasharray="4 3"/><text x="1126" y="28" font-size="12" fill="#374151">source</text>`)

	// Full timeline.
	qi := job.QIndex(0.50)
	raw := quantileColumn(job, qi, false)
	src := quantileColumn(job, qi, true)
	final := CorrectedCurve(job, plan)
	tLo, tHi := 2.0, -1.0
	for t := 0; t < job.FrameCount && t < len(raw); t++ {
		for _, v := range []float64{raw[t], src[t], final[t]} {
			if v <= 0 {
				continue
			}
			tLo, tHi = minF(tLo, v), maxF(tHi, v)
		}
	}
	if tHi > tLo {
		pad := (tHi - tLo) * 0.12
		tLo, tHi = tLo-pad, tHi+pad
	}
	tb := svgBox{x: 60, y: 50, w: 1140, h: 240, yMin: tLo, yMax: tHi}
	drawAxes(&b, tb, "full timeline (frame index)")
	xFrame := func(t int) float64 { return tb.x + float64(t)/float64(job.FrameCount-1)*tb.w }
	for _, c := range job.Cuts {
		x := xFrame(c)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1" stroke-dasharray="5 4"/>`, x, tb.y, x, tb.y+tb.h, colorCut)
	}
	fmt.Fprintf(&b, `<polyline fill="none" stroke="%s" stroke-width="1.1" points="%s"/>`, colorSource, linePath(src, xFrame, tb))
	fmt.Fprintf(&b, `<polyline fill="none" stroke="%s" stroke-width="1.2" points="%s"/>`, colorRaw, linePath(raw, xFrame, tb))
	fmt.Fprintf(&b, `<polyline fill="none" stroke="%s" stroke-width="1.4" points="%s"/>`, colorRepaired, linePath(final, xFrame, tb))

	// 3x3 cut windows.
	panelW, panelH := 370.0, 200.0
	gapX, gapY := 25.0, 40.0
	startX, startY := 30.0, 350.0
	for ci := 0; ci < len(job.Cuts); ci++ {
		col := ci % 3
		row := ci / 3
		px := startX + float64(col)*(panelW+gapX)
		py := startY + float64(row)*(panelH+gapY)
		drawCutPanel(&b, job, rep, plan, ci, svgBox{x: px + 46, y: py + 24, w: panelW - 58, h: panelH - 58, yMin: 0.25, yMax: 0.72})
	}
	b.WriteString(`</svg>`)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func drawAxes(b *strings.Builder, box svgBox, title string) {
	fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#fafafa" stroke="%s"/>`, box.x, box.y, box.w, box.h, colorGrid)
	for gi := 0; gi <= 4; gi++ {
		v := box.yMin + (box.yMax-box.yMin)*float64(gi)/4
		y := box.yMap(v)
		fmt.Fprintf(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s"/>`, box.x, y, box.x+box.w, y, colorGrid)
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="10" fill="#6b7280" text-anchor="end">%.2f</text>`, box.x-5, y+3, v)
	}
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="12" fill="#111827">%s</text>`, box.x, box.y-8, title)
}

var kindColor = map[string]string{
	KindConversion:  "#b91c1c",
	KindNatural:     "#047857",
	KindMixed:       "#b45309",
	KindPreexisting: "#6d28d9",
	KindConsistent:  "#374151",
	KindAmbiguous:   "#9ca3af",
}

func drawCutPanel(b *strings.Builder, job *Job, rep *Report, plan *RepairPlan, ci int, box svgBox) {
	c := job.Cuts[ci]
	d := rep.Cuts[ci]
	w := rep.Config.Window
	gapAfter := 0.0
	if ci < len(plan.Verification) {
		gapAfter = plan.Verification[ci].GapAfter
	}
	title := fmt.Sprintf("cut %d @%d  %s  gap %+.3f -> %+.3f", ci, c, d.Kind, d.FitGap, gapAfter)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="11.5" font-weight="bold" fill="%s">%s</text>`, box.x-44, box.y-10, kindColor[d.Kind], xmlEscape(title))
	drawAxes(b, box, "")
	fmt.Fprintf(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.2" stroke-dasharray="4 3"/>`,
		box.x+box.w/2, box.y, box.x+box.w/2, box.y+box.h, colorCut)

	qi := job.QIndex(0.50)
	raw := quantileColumn(job, qi, false)
	src := quantileColumn(job, qi, true)
	final := CorrectedCurve(job, plan)
	lo, hi := 2.0, -1.0
	for rel := -w; rel < w; rel++ {
		t := c + rel
		if t < 0 || t >= len(raw) {
			continue
		}
		for _, v := range []float64{raw[t], src[t], final[t]} {
			if v <= 0 {
				continue
			}
			lo, hi = minF(lo, v), maxF(hi, v)
		}
	}
	if hi > lo {
		pad := (hi - lo) * 0.18
		box.yMin, box.yMax = lo-pad, hi+pad
	}
	xRel := func(rel int) float64 {
		return box.x + (float64(rel+w)/float64(2*w-1))*box.w
	}
	plotRange := func(curve []float64) string {
		var sb strings.Builder
		for rel := -w; rel < w; rel++ {
			t := c + rel
			if t < 0 || t >= len(curve) {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			fmt.Fprintf(&sb, "%.1f,%.1f", xRel(rel), box.yMap(curve[t]))
		}
		return sb.String()
	}
	fmt.Fprintf(b, `<polyline fill="none" stroke="%s" stroke-width="1" stroke-dasharray="4 3" points="%s"/>`, colorSource, plotRange(src))
	fmt.Fprintf(b, `<polyline fill="none" stroke="%s" stroke-width="1.3" points="%s"/>`, colorRaw, plotRange(raw))
	fmt.Fprintf(b, `<polyline fill="none" stroke="%s" stroke-width="1.3" points="%s"/>`, colorRepaired, plotRange(final))
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
