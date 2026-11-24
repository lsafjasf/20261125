package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"colorcal"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "colorcal:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "sample":
		return runSample(args[1:], stdout)
	case "fit":
		return runFit(args[1:], stdout)
	case "verify":
		return runVerify(args[1:], stdout)
	case "demo":
		return runDemo(args[1:], stdout)
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `colorcal — robust camera chart calibration

Usage:
  colorcal sample -out observations.json [-truth generated-truth.json] [-seed 42]
  colorcal fit    -input observations.json -report report.json [-corrected corrected.json]
  colorcal verify -truth generated-truth.json -report report.json
  colorcal demo   -out demo-dir [-seed 42]

The calibrator reads only observations.json. Generated truth is written only
for external acceptance comparison and is never read by "fit".`)
}

func runSample(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("sample", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("out", "observations.json", "observation chart JSON path")
	truthPath := fs.String("truth", "", "optional separate acceptance truth path")
	seed := fs.Int64("seed", 42, "deterministic generator seed")
	if err := fs.Parse(args); err != nil {
		return err
	}

	chart, truth := colorcal.NewSyntheticScenario(*seed)
	if err := writeJSON(*out, chart); err != nil {
		return err
	}
	if *truthPath != "" {
		if err := writeJSON(*truthPath, truth); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "wrote %s (%d patches)\n", *out, len(chart.Patches))
	if *truthPath != "" {
		fmt.Fprintf(stdout, "wrote acceptance-only truth %s\n", *truthPath)
	}
	return nil
}

func runFit(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("fit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	input := fs.String("input", "", "observation chart JSON")
	reportPath := fs.String("report", "", "write full JSON report here")
	correctedPath := fs.String("corrected", "", "optional corrected colors JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return errors.New("fit requires -input")
	}

	chart, err := loadChart(*input)
	if err != nil {
		return err
	}
	report, err := colorcal.Fit(chart)
	if err != nil {
		return err
	}
	if *reportPath != "" {
		if err := writeJSON(*reportPath, report); err != nil {
			return err
		}
	}
	if *correctedPath != "" {
		if err := writeCorrected(*correctedPath, report); err != nil {
			return err
		}
	}
	printSummary(stdout, report)
	if *reportPath != "" {
		fmt.Fprintf(stdout, "report: %s\n", *reportPath)
	}
	return nil
}

func runDemo(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outDir := fs.String("out", "colorcal-demo", "directory for generated files")
	seed := fs.Int64("seed", 42, "deterministic generator seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}

	chart, truth := colorcal.NewSyntheticScenario(*seed)
	paths := map[string]any{
		"observations.json":    chart,
		"generated-truth.json": truth,
	}
	report, err := colorcal.Fit(chart)
	if err != nil {
		return err
	}
	paths["report.json"] = report
	paths["corrected.json"] = correctedOutput(report)

	for name, value := range paths {
		if err := writeJSON(filepath.Join(*outDir, name), value); err != nil {
			return err
		}
	}
	printSummary(stdout, report)
	fmt.Fprintf(stdout, "demo files written to %s\n", *outDir)
	fmt.Fprintln(stdout, "truth was kept separate and was not read by Fit")
	return nil
}

type correctedPatch struct {
	ID           string       `json:"id"`
	Status       string       `json:"status"`
	CorrectedRGB colorcal.RGB `json:"corrected_linear_rgb"`
	DeltaE2000   float64      `json:"delta_e2000"`
}

func correctedOutput(report *colorcal.Report) []correctedPatch {
	out := make([]correctedPatch, 0, len(report.Patches))
	for _, patch := range report.Patches {
		out = append(out, correctedPatch{
			ID: patch.ID, Status: patch.Status,
			CorrectedRGB: patch.CorrectedRGB, DeltaE2000: patch.ReferenceDeltaE,
		})
	}
	return out
}

func writeCorrected(path string, report *colorcal.Report) error {
	return writeJSON(path, correctedOutput(report))
}

func loadChart(path string) (colorcal.Chart, error) {
	var chart colorcal.Chart
	data, err := os.ReadFile(path)
	if err != nil {
		return chart, err
	}
	if err := json.Unmarshal(data, &chart); err != nil {
		return chart, fmt.Errorf("parse chart %s: %w", path, err)
	}
	return chart, nil
}

func writeJSON(path string, value any) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func printSummary(w io.Writer, report *colorcal.Report) {
	p := report.Params
	fmt.Fprintf(w, "exposure=%.4f WB=[%.4f %.4f %.4f] gamma=[%.4f %.4f %.4f]\n",
		p.Exposure, p.WhiteBalance[0], p.WhiteBalance[1], p.WhiteBalance[2],
		p.Gamma[0], p.Gamma[1], p.Gamma[2])
	fmt.Fprintf(w, "condition=%.3g chi2/dof=%.3f identifiable=%v couplings=%d\n",
		report.Diagnostics.MatrixCondition, report.Diagnostics.ReducedChiSquare,
		report.Diagnostics.Identifiable, len(report.Diagnostics.Couplings))
	for i, coupling := range report.Diagnostics.Couplings {
		if i >= 3 {
			break
		}
		fmt.Fprintf(w, "coupling %s<->%s r=%+.3f %s\n",
			coupling.Parameters[0], coupling.Parameters[1],
			coupling.Correlation, coupling.Strength)
	}
	if report.NaiveMeanAlign != nil && report.JointFit != nil {
		fmt.Fprintf(w, "skin DeltaE2000: mean-align=%.2f -> joint=%.2f\n",
			report.NaiveMeanAlign.SkinDeltaE, report.JointFit.SkinDeltaE)
	}
	var unreliable, downweighted int
	for _, patch := range report.Patches {
		switch patch.Status {
		case "unreliable":
			unreliable++
		case "downweighted":
			downweighted++
		}
	}
	fmt.Fprintf(w, "patches: reliable=%d downweighted=%d unreliable=%d\n",
		len(report.Patches)-unreliable-downweighted, downweighted, unreliable)
}
