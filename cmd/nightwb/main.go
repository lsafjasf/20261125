package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"nightwb"
)

func main() {
	in := flag.String("in", "", "input image path (PNG/JPEG)")
	outDir := flag.String("out", "wb_out", "output directory")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "nightwb: -in is required")
		flag.Usage()
		os.Exit(2)
	}

	f, err := os.Open(*in)
	if err != nil {
		fatal(err)
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		fatal(err)
	}

	frame := nightwb.DecodeFrame(img)
	p := nightwb.Run(frame)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}
	writePNG(filepath.Join(*outDir, "conservative.png"),
		func(w *os.File) error {
			return nightwb.EncodePNG(w, p.Conservative, frame.Width, frame.Height)
		})
	writePNG(filepath.Join(*outDir, "aggressive.png"),
		func(w *os.File) error {
			return nightwb.EncodePNG(w, p.Aggressive, frame.Width, frame.Height)
		})
	writePNG(filepath.Join(*outDir, "mask_source.png"),
		func(w *os.File) error {
			return nightwb.EncodeMaskPNG(w, p.Masks[0], frame.Width, frame.Height)
		})
	writePNG(filepath.Join(*outDir, "mask_adaptation.png"),
		func(w *os.File) error {
			return nightwb.EncodeMaskPNG(w, p.Masks[1], frame.Width, frame.Height)
		})

	reportPath := filepath.Join(*outDir, "report.json")
	rf, err := os.Create(reportPath)
	if err != nil {
		fatal(err)
	}
	enc := json.NewEncoder(rf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p.Report); err != nil {
		rf.Close()
		fatal(err)
	}
	rf.Close()

	printSummary(p)
	fmt.Printf("outputs written to %s\n", *outDir)
}

func writePNG(path string, fn func(*os.File) error) {
	w, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	if err := fn(w); err != nil {
		w.Close()
		fatal(err)
	}
	w.Close()
}

func printSummary(p *nightwb.Pipeline) {
	ill := p.Report.Illuminant
	fmt.Printf("illuminant: method=%s confidence=%.2f samples=%d cct=%.0fK mixed=%v\n",
		ill.Method, ill.Confidence, ill.SampleCount, ill.CCT, ill.MixedLight)
	fmt.Printf("exposure:   state=%s clipped=%.3f crushed=%.3f usable=%.3f\n",
		p.Report.Exposure.ExposureState,
		p.Report.Exposure.HighlightClippedFrac,
		p.Report.Exposure.ShadowCrushedFrac,
		p.Report.Exposure.UsableFrac)
	for _, s := range p.Report.Schemes {
		fmt.Printf("scheme %-12s strength=%.2f outOfGamut=%.4f\n",
			s.Name, s.Strength, s.OutOfGamutFrac)
	}
	for _, w := range ill.Warnings {
		fmt.Printf("warning: %s\n", w)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "nightwb:", err)
	os.Exit(1)
}
