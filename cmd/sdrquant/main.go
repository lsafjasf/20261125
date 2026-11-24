package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"sdrquant"
)

func main() {
	var (
		input          = flag.String("input", "", "input PFM/PNG/JPEG/GIF; omit with -synthetic")
		output         = flag.String("output", "sdr.png", "8-bit SDR PNG output")
		diagDir        = flag.String("diag", "diagnostics", "diagnostic output directory")
		ditherName     = flag.String("dither", "tpdf", "dither: none, rectangular, tpdf")
		shapingName    = flag.String("shaping", "floyd-steinberg", "shaping: none, floyd-steinberg, atkinson, sierra-lite, jjn, stucki")
		ditherStrength = flag.Float64("dither-strength", 1.0, "dither amplitude in output LSB")
		seed           = flag.Int64("seed", 1, "deterministic dither seed")
		serpentine     = flag.Bool("serpentine", true, "alternate error-diffusion scan direction")
		transferName   = flag.String("transfer", "display", "input transfer: display (none) or srgb (linear source)")
		spectrumSize   = flag.Int("spectrum-size", 512, "FFT size, rounded to a power of two")
		synthetic      = flag.Bool("synthetic", false, "generate a dark high-bit-depth test gradient")
		width          = flag.Int("width", 960, "synthetic width")
		height         = flag.Int("height", 540, "synthetic height")
		minValue       = flag.Float64("synthetic-min", 0.001, "synthetic minimum value")
		maxValue       = flag.Float64("synthetic-max", 0.08, "synthetic maximum value")
		texture        = flag.Float64("synthetic-texture", 0.0006, "synthetic low-frequency texture")
		step           = flag.Float64("synthetic-step", 0.004, "synthetic genuine source step (0 disables)")
		stepAt         = flag.Float64("synthetic-step-at", 0.75, "synthetic source step position")
		quiet          = flag.Bool("quiet", false, "suppress summary")
	)
	flag.Parse()

	dither, err := sdrquant.ParseDither(*ditherName)
	must(err)
	shaping, err := sdrquant.ParseShaping(*shapingName)
	must(err)
	transfer, err := sdrquant.ParseTransfer(*transferName)
	must(err)

	var src *sdrquant.FloatImage
	if *synthetic || *input == "" {
		src, err = sdrquant.GenerateDarkGradient(sdrquant.SyntheticConfig{
			Width: *width, Height: *height,
			Min: *minValue, Max: *maxValue, Texture: *texture,
			Step: *step, StepAt: *stepAt,
		})
		must(err)
	} else {
		src, err = sdrquant.LoadFloatImage(*input)
		must(err)
	}

	cfg := sdrquant.DefaultConfig()
	cfg.Dither = dither
	cfg.DitherStrength = *ditherStrength
	cfg.Shaping = shaping
	cfg.Seed = *seed
	cfg.Serpentine = *serpentine
	cfg.Transfer = transfer
	result, err := sdrquant.Quantize(src, cfg)
	must(err)
	diag, err := sdrquant.Analyze(result, sdrquant.DiagnosticOptions{SpectrumSize: *spectrumSize})
	must(err)

	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		must(err)
	}
	must(sdrquant.SaveResultPNG(*output, result))
	must(sdrquant.WriteDiagnostics(*diagDir, diag))

	if !*quiet {
		fmt.Printf("SDR: %s\n", *output)
		fmt.Printf("dither=%s shaping=%s transfer=%s seed=%d\n", dither, shaping, transfer, *seed)
		fmt.Printf("residual RMS=%.4f LSB  feedback RMS=%.4f LSB\n",
			diag.Metrics.Residual.RMSLSB, diag.Metrics.Feedback.RMSLSB)
		fmt.Printf("source edges=%d  baseline synthetic edges=%d  dither-altered pixels=%d\n",
			diag.Metrics.Distinction.SourceEdges,
			diag.Metrics.Distinction.BaselineSyntheticEdges,
			diag.Metrics.Distinction.DitherAlteredPixels)
		fmt.Printf("spectrum high-frequency ratio=%.3f  diagnostics=%s\n",
			diag.Metrics.Spectrum.HighFrequencyRatio, *diagDir)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "sdrquant:", err)
		os.Exit(1)
	}
}
