package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"logshoulder/internal/display"
	"logshoulder/internal/pipeline"
	"logshoulder/internal/report"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "logfix:", err)
		os.Exit(1)
	}
}

func run() error {
	outDir := flag.String("out", "artifacts", "output directory")
	knee := flag.Float64("knee", 0.70, "scene-linear shoulder knee; values below it are unchanged")
	ceiling := flag.Float64("ceiling", 0.90, "exclusive display-linear shoulder ceiling")
	calInput := flag.Float64("cal-input", 128, "scene-linear value used to calibrate the shoulder")
	calOutput := flag.Float64("cal-output", 0.88, "display-linear output at -cal-input")
	flag.Parse()

	shoulder := display.Shoulder{
		Knee:              *knee,
		Ceiling:           *ceiling,
		CalibrationInput:  *calInput,
		CalibrationOutput: *calOutput,
	}
	if err := shoulder.Validate(); err != nil {
		return err
	}
	convs, err := report.Conversions(shoulder)
	if err != nil {
		return err
	}
	if err := verify(convs); err != nil {
		return err
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	if err := writeMarkdown(filepath.Join(*outDir, "report.md"), convs, shoulder); err != nil {
		return err
	}
	if err := writeCSV(filepath.Join(*outDir, "metrics.csv"), func(w io.Writer) error {
		return report.WriteMetricsCSV(w, convs)
	}); err != nil {
		return err
	}
	if err := writeCSV(filepath.Join(*outDir, "trace.csv"), func(w io.Writer) error {
		return report.WriteTraceCSV(w, convs)
	}); err != nil {
		return err
	}
	imagePath := filepath.Join(*outDir, "before-after.png")
	if err := report.WriteComparison(imagePath, convs); err != nil {
		return err
	}
	fmt.Printf("\n已写入 `%s`、`metrics.csv`、`trace.csv` 和 `before-after.png`。\n", filepath.Join(*outDir, "report.md"))
	return nil
}

func verify(convs []pipeline.Conversion) error {
	for _, conv := range convs {
		m := conv.Measure()
		if !m.FixedStrictlyOrdered || m.FixedLastGap <= 0 {
			return fmt.Errorf("%s: repaired highlights are not strictly ordered", conv.Case.Name)
		}
		if m.FixedMidtoneError > 1e-12 {
			return fmt.Errorf("%s: midtone moved by %.12f", conv.Case.Name, m.FixedMidtoneError)
		}
		if m.FixedShadowError > 1e-12 {
			return fmt.Errorf("%s: shadow moved by %.12f", conv.Case.Name, m.FixedShadowError)
		}
	}
	return nil
}

func writeMarkdown(path string, convs []pipeline.Conversion, shoulder display.Shoulder) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return report.WriteMarkdown(io.MultiWriter(os.Stdout, file), convs, shoulder)
}

func writeCSV(path string, write func(io.Writer) error) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return write(file)
}
