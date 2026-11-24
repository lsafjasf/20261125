package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
)

type ExtractedFrame struct {
	Path   string
	Image  image.Image
	Bounds image.Rectangle
}

func extractFrame(ffmpegBin, input, at, outPath string, autorotate bool) error {
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if !autorotate {
		args = append(args, "-noautorotate")
	}
	if at != "" {
		args = append(args, "-ss", at)
	}
	args = append(args, "-i", input, "-frames:v", "1", "-an", "-sn", outPath)
	cmd := exec.Command(ffmpegBin, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w (%s)", ffmpegBin, err, string(output))
	}
	return nil
}

func readImageConfig(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func savePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	return f.Close()
}

func rotate90CW(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			c := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
			dx := bounds.Max.Y - 1 - y
			dy := x - bounds.Min.X
			dst.SetRGBA(dx, dy, c)
		}
	}
	return dst
}

func rotateTurns(src image.Image, turns int) image.Image {
	img := src
	for i := 0; i < normTurnsCW(turns); i++ {
		img = rotate90CW(img)
	}
	return img
}

func inferCorrectionFromReference(rawPath, referencePath string) (int, []float64, error) {
	raw, err := loadImage(rawPath)
	if err != nil {
		return 0, nil, fmt.Errorf("decode raw frame: %w", err)
	}
	ref, err := loadImage(referencePath)
	if err != nil {
		return 0, nil, fmt.Errorf("decode reference: %w", err)
	}
	refSmall := downsampleSquare(ref, 72)
	maes := make([]float64, 4)
	bestTurns := 0
	best := math.Inf(1)
	for turns := 0; turns < 4; turns++ {
		candidate := downsampleSquare(rotateTurns(raw, turns), 72)
		maes[turns] = imageMAE(candidate, refSmall)
		if maes[turns] < best {
			best = maes[turns]
			bestTurns = turns
		}
	}
	return bestTurns, maes, nil
}

func downsampleSquare(src image.Image, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	b := src.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	for y := 0; y < size; y++ {
		sy := b.Min.Y + int(float64(y)*float64(srcH)/float64(size))
		for x := 0; x < size; x++ {
			sx := b.Min.X + int(float64(x)*float64(srcW)/float64(size))
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func imageMAE(a, b *image.RGBA) float64 {
	bounds := a.Bounds()
	var sum float64
	var count float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			sum += math.Abs(float64(ar>>8) - float64(br>>8))
			sum += math.Abs(float64(ag>>8) - float64(bg>>8))
			sum += math.Abs(float64(ab>>8) - float64(bb>>8))
			count += 3
		}
	}
	if count == 0 {
		return math.MaxFloat64
	}
	return sum / count
}

type ComparisonPanel struct {
	Number int
	Path   string
	Border color.RGBA
}

func WriteComparisonSheet(outPath string, panels []ComparisonPanel) error {
	const columns = 2
	const cellW = 460
	const cellH = 310
	const thumbW = 416
	const thumbH = 244
	rows := (len(panels) + columns - 1) / columns
	sheet := image.NewRGBA(image.Rect(0, 0, columns*cellW, rows*cellH))
	fillRect(sheet, sheet.Bounds(), color.RGBA{R: 248, G: 249, B: 251, A: 255})

	for i, panel := range panels {
		col := i % columns
		row := i / columns
		x0 := col * cellW
		y0 := row * cellH
		cellRect := image.Rect(x0+10, y0+10, x0+cellW-10, y0+cellH-10)
		fillRect(sheet, cellRect, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		drawRectBorder(sheet, cellRect, 5, panel.Border)

		img, err := loadImage(panel.Path)
		if err != nil {
			continue
		}
		imgRect := fitRect(img.Bounds(), image.Rect(x0+22, y0+48, x0+22+thumbW, y0+48+thumbH))
		drawNearestContain(sheet, img, imgRect)
		drawDigitBadge(sheet, x0+20, y0+20, panel.Number)
	}
	return savePNG(outPath, sheet)
}

func fitRect(srcBounds image.Rectangle, box image.Rectangle) image.Rectangle {
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	boxW := box.Dx()
	boxH := box.Dy()
	scale := math.Min(float64(boxW)/float64(srcW), float64(boxH)/float64(srcH))
	w := int(math.Round(float64(srcW) * scale))
	h := int(math.Round(float64(srcH) * scale))
	x := box.Min.X + (boxW-w)/2
	y := box.Min.Y + (boxH-h)/2
	return image.Rect(x, y, x+w, y+h)
}

func drawNearestContain(dst *image.RGBA, src image.Image, rect image.Rectangle) {
	b := src.Bounds()
	for dy := rect.Min.Y; dy < rect.Max.Y; dy++ {
		sy := b.Min.Y + (dy-rect.Min.Y)*b.Dy()/rect.Dy()
		for dx := rect.Min.X; dx < rect.Max.X; dx++ {
			sx := b.Min.X + (dx-rect.Min.X)*b.Dx()/rect.Dx()
			dst.Set(dx, dy, src.At(sx, sy))
		}
	}
}

func fillRect(dst *image.RGBA, rect image.Rectangle, c color.Color) {
	rgba := color.RGBAModel.Convert(c).(color.RGBA)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			dst.SetRGBA(x, y, rgba)
		}
	}
}

func drawRectBorder(dst *image.RGBA, rect image.Rectangle, thickness int, c color.Color) {
	fillRect(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+thickness), c)
	fillRect(dst, image.Rect(rect.Min.X, rect.Max.Y-thickness, rect.Max.X, rect.Max.Y), c)
	fillRect(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+thickness, rect.Max.Y), c)
	fillRect(dst, image.Rect(rect.Max.X-thickness, rect.Min.Y, rect.Max.X, rect.Max.Y), c)
}

var digitPatterns = map[rune][5]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "010", "010", "010"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
}

func drawDigitBadge(dst *image.RGBA, x, y, digit int) {
	fillRect(dst, image.Rect(x, y, x+34, y+42), color.RGBA{R: 17, G: 24, B: 39, A: 255})
	pattern, ok := digitPatterns[rune('0'+digit%10)]
	if !ok {
		return
	}
	scale := 6
	for row, line := range pattern {
		for col, cell := range line {
			if cell == '1' {
				fillRect(dst, image.Rect(x+8+col*scale, y+6+row*scale, x+8+(col+1)*scale, y+6+(row+1)*scale),
					color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
}

func ensureTools(ffprobeBin, ffmpegBin string) error {
	if _, err := exec.LookPath(ffprobeBin); err != nil {
		return fmt.Errorf("ffprobe is required: %w", err)
	}
	if _, err := exec.LookPath(ffmpegBin); err != nil {
		return fmt.Errorf("ffmpeg is required: %w", err)
	}
	return nil
}

func chooseFrameTime(probed *ProbedVideo, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if probed != nil && probed.DurationSeconds > 1 {
		return "0.5"
	}
	return ""
}
