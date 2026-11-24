package report

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"logshoulder/internal/display"
	"logshoulder/internal/pipeline"
)

const (
	panelWidth  = 640
	panelHeight = 360
)

// WriteComparison renders three rows: one exposure sample each. The left panel
// is the legacy Log-code-as-display path; the right panel is the repaired path.
// Both use 16-bit sRGB codes so near-white separations are measurable.
func WriteComparison(path string, convs []pipeline.Conversion) error {
	width, height := panelWidth*2, panelHeight*len(convs)
	img := image.NewRGBA64(image.Rect(0, 0, width, height))
	for i, conv := range convs {
		y := i * panelHeight
		drawPanel(img, image.Pt(0, y), conv, false)
		drawPanel(img, image.Pt(panelWidth, y), conv, true)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func drawPanel(dst *image.RGBA64, origin image.Point, conv pipeline.Conversion, fixed bool) {
	baseScene := math.Max(conv.Case.Midtone*0.08, 0.001)
	fill(dst, rect(origin.X, origin.Y, panelWidth, panelHeight), func(x, y int) color.RGBA64 {
		return gray(renderScene(conv, baseScene, fixed))
	})

	accent := color.RGBA64{R: 0x8e00, G: 0x2a00, B: 0x2a00, A: 0xffff}
	if fixed {
		accent = color.RGBA64{R: 0x2100, G: 0x7400, B: 0x3900, A: 0xffff}
	}
	drawBorder(dst, image.Rect(origin.X, origin.Y, origin.X+panelWidth, origin.Y+panelHeight), 5, accent)

	for i, scene := range conv.Case.Shadows {
		r := image.Rect(origin.X+44+i*76, origin.Y+50, origin.X+44+i*76+60, origin.Y+104)
		code := renderScene(conv, scene, fixed)
		fill(dst, r, func(_, _ int) color.RGBA64 { return gray(code) })
		drawBorder(dst, r, 1, color.RGBA64{A: 0xffff})
	}

	mid := image.Rect(origin.X+48, origin.Y+142, origin.X+226, origin.Y+306)
	midCode := renderScene(conv, conv.Case.Midtone, fixed)
	fill(dst, mid, func(_, _ int) color.RGBA64 { return gray(midCode) })
	drawBorder(dst, mid, 1, color.RGBA64{A: 0xffff})

	const (
		winX = 330
		winY = 38
		winW = 260
		winH = 284
	)
	drawBorder(dst, image.Rect(origin.X+winX-3, origin.Y+winY-3, origin.X+winX+winW+3, origin.Y+winY+winH+3), 3, color.RGBA64{A: 0xffff})
	stripe := winW / len(conv.Case.Highlights)
	for i, scene := range conv.Case.Highlights {
		x0 := origin.X + winX + i*stripe
		x1 := x0 + stripe
		if i == len(conv.Case.Highlights)-1 {
			x1 = origin.X + winX + winW
		}
		code := renderScene(conv, scene, fixed)
		fill(dst, image.Rect(x0, origin.Y+winY, x1, origin.Y+winY+winH), func(_, _ int) color.RGBA64 {
			return gray(code)
		})
		fill(dst, image.Rect(x1-1, origin.Y+winY, x1, origin.Y+winY+winH), func(_, _ int) color.RGBA64 {
			return color.RGBA64{A: 0xffff}
		})
	}
}

func renderScene(c pipeline.Conversion, scene float64, fixed bool) float64 {
	code := clamp01(c.Case.Curve.Encode(scene))
	if fixed {
		return display.SRGBEncode(c.Shoulder.Apply(c.Case.Curve.Decode(code)))
	}
	return display.SRGBEncode(clamp01(code / pipeline.LegalWhiteCode))
}

func rect(x, y, w, h int) image.Rectangle {
	return image.Rect(x, y, x+w, y+h)
}

func fill(dst *image.RGBA64, r image.Rectangle, sample func(x, y int) color.RGBA64) {
	r = r.Intersect(dst.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst.SetRGBA64(x, y, sample(x, y))
		}
	}
}

func drawBorder(dst *image.RGBA64, r image.Rectangle, width int, c color.RGBA64) {
	fill(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+width), func(_, _ int) color.RGBA64 { return c })
	fill(dst, image.Rect(r.Min.X, r.Max.Y-width, r.Max.X, r.Max.Y), func(_, _ int) color.RGBA64 { return c })
	fill(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+width, r.Max.Y), func(_, _ int) color.RGBA64 { return c })
	fill(dst, image.Rect(r.Max.X-width, r.Min.Y, r.Max.X, r.Max.Y), func(_, _ int) color.RGBA64 { return c })
}

func gray(code float64) color.RGBA64 {
	v := uint16(math.Round(clamp01(code) * 65535))
	return color.RGBA64{R: v, G: v, B: v, A: 0xffff}
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
