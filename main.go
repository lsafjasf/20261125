package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

type Report struct {
	Width   int       `json:"width"`
	Height  int       `json:"height"`
	Edges   []float64 `json:"edges_luma_x"`
	Metrics []Metrics `json:"metrics"`
	Files   []string  `json:"files"`
}

func main() {
	outputDir := flag.String("outdir", "artifacts", "directory for PNG frames and JSON metrics")
	flag.Parse()

	if err := run(*outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "chromasiting: %v\n", err)
		os.Exit(1)
	}
}

func run(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	pattern := NewTestPattern()
	leftFrame, err := EncodeYUV422(pattern, ChromaLeft)
	if err != nil {
		return err
	}
	centerFrame, err := EncodeYUV422(pattern, ChromaCenter)
	if err != nil {
		return err
	}

	leftCorrect := Reconstruct(leftFrame, ChromaLeft)
	centerWrong := Reconstruct(centerFrame, ChromaLeft)
	centerCorrect := Reconstruct(centerFrame, ChromaCenter)
	blurredWrong := Reconstruct(BlurChromaPlane(centerFrame), ChromaLeft)

	files := []string{
		"test_frame_source.png",
		"left_siting_correct.png",
		"center_siting_wrong_left.png",
		"center_siting_corrected.png",
		"blur_only_control.png",
		"comparison.png",
	}
	if err := writePNG(filepath.Join(outputDir, files[0]), pattern); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(outputDir, files[1]), annotate(leftCorrect, "L-OK")); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(outputDir, files[2]), annotate(centerWrong, "BAD")); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(outputDir, files[3]), annotate(centerCorrect, "FIX")); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(outputDir, files[4]), annotate(blurredWrong, "BLUR")); err != nil {
		return err
	}

	comparison := composeComparison(leftCorrect, centerWrong, centerCorrect, blurredWrong)
	if err := writePNG(filepath.Join(outputDir, files[5]), comparison); err != nil {
		return err
	}

	width := 320
	height := 32
	edges := []float64{31.5, 79.5, 127.5, 175.5, 223.5, 271.5}
	synthetic := NewSyntheticChromaFrame(width, height, ChromaCenter, edges)
	ramp := NewSyntheticChromaFrame(width, height, ChromaCenter, nil)
	report := Report{
		Width:  width,
		Height: height,
		Edges:  edges,
		Files:  files,
		Metrics: []Metrics{
			EvaluateDecoder("center_data_assumed_left", synthetic, ramp, ChromaLeft, edges, width, height),
			EvaluateDecoder("center_data_site_aware", synthetic, ramp, ChromaCenter, edges, width, height),
			EvaluateDecoder("center_data_blurred_then_left", BlurChromaPlane(synthetic), BlurChromaPlane(ramp), ChromaLeft, edges, width, height),
		},
	}

	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "metrics.json"), append(jsonData, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Println("chroma siting test artifacts:")
	for _, name := range files {
		fmt.Println("  ", filepath.Join(outputDir, name))
	}
	fmt.Println("  ", filepath.Join(outputDir, "metrics.json"))
	printMetrics(report.Metrics)
	return nil
}

func writePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func annotate(img *image.RGBA, label string) *image.RGBA {
	annotated := image.NewRGBA(img.Bounds())
	copy(annotated.Pix, img.Pix)
	fillRect(annotated, image.Rect(4, 29, 70, 38), rgbBlack)
	drawText(annotated, 8, 30, label, rgbYellow, 1)
	return annotated
}

func composeComparison(images ...*image.RGBA) *image.RGBA {
	gap := 12
	width := patternWidth*2 + gap*3
	height := patternHeight*2 + gap*3
	sheet := image.NewRGBA(image.Rect(0, 0, width, height))
	fillRect(sheet, sheet.Bounds(), color.RGBA{R: 45, G: 45, B: 45, A: 255})

	for i, source := range images {
		column := i % 2
		row := i / 2
		origin := image.Pt(gap+column*(patternWidth+gap), gap+row*(patternHeight+gap))
		drawImage(sheet, source, origin)
	}
	return sheet
}

func drawImage(dst *image.RGBA, src *image.RGBA, origin image.Point) {
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			dst.SetRGBA(origin.X+x, origin.Y+y, src.RGBAAt(x, y))
		}
	}
}

func printMetrics(metrics []Metrics) {
	fmt.Println("\nedge metrics (lower is better):")
	for _, item := range metrics {
		fmt.Printf("  %-30s edge=%6.3f  vertical=%6.3f  chromaMAE=%7.3f  transition=%5.2f\n",
			item.Name, item.EdgeMAE, item.VerticalEdgeMAE, item.ChromaMAE, item.MeanTransition)
	}
}
