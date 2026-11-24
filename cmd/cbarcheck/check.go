package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"cbarcheck"
)

type checkOptions struct {
	input       string
	width       int
	height      int
	depthText   string
	rangeText   string
	matrixText  string
	targetDepth string
	patternText string
	lumaTol     float64
	chromaTol   float64
	out         string
	name        string
	source      string
}

func runCheck(args []string) int {
	var opt checkOptions
	fs := flag.NewFlagSet("cbarcheck check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opt.input, "input", "", "planar yuv444p frame (required)")
	fs.IntVar(&opt.width, "width", 0, "frame width")
	fs.IntVar(&opt.height, "height", 0, "frame height")
	fs.StringVar(&opt.depthText, "depth", "10", "frame storage depth: 8, 10, or 12")
	fs.StringVar(&opt.rangeText, "target-range", "limited", "target range: limited or full")
	fs.StringVar(&opt.matrixText, "target-matrix", "bt709", "target matrix: bt601, bt709, or bt2020")
	fs.StringVar(&opt.targetDepth, "target-depth", "", "target bit depth (default: storage depth)")
	fs.StringVar(&opt.patternText, "pattern", "ebu75", "test pattern: ebu75 or smpte100")
	fs.Float64Var(&opt.lumaTol, "luma-tol", 2, "luma tolerance in 8-bit code units")
	fs.Float64Var(&opt.chromaTol, "chroma-tol", 2, "chroma tolerance in 8-bit code units")
	fs.StringVar(&opt.out, "out", "", "optional output directory")
	fs.StringVar(&opt.name, "name", "", "artifact name prefix (default: input basename)")
	fs.StringVar(&opt.source, "source", "", "source label written to the report")
	fs.Usage = func() { checkUsage(fs) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n", fs.Args())
		return 2
	}
	if opt.input == "" {
		fmt.Fprintln(os.Stderr, "-input is required")
		checkUsage(fs)
		return 2
	}
	if opt.width <= 0 || opt.height <= 0 {
		fmt.Fprintln(os.Stderr, "-width and -height must be positive")
		return 2
	}

	depth, err := parseDepth(opt.depthText)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	targetDepth := depth
	if opt.targetDepth != "" {
		targetDepth, err = parseDepth(opt.targetDepth)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	rangeMode, err := parseRange(opt.rangeText)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	matrix, err := parseMatrix(opt.matrixText)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	pattern, err := parsePattern(opt.patternText)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	layout, err := cbarcheck.BarsLayout(pattern, opt.width, opt.height)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	f, err := os.Open(opt.input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	} else if st.Size() != cbarcheck.Raw444Size(opt.width, opt.height, depth) {
		fmt.Fprintf(os.Stderr, "input size %d does not match one %dx%d %d-bit yuv444p frame (%d bytes)\n",
			st.Size(), opt.width, opt.height, depth, cbarcheck.Raw444Size(opt.width, opt.height, depth))
		return 2
	}
	frame, err := cbarcheck.ReadRaw444(f, opt.width, opt.height, depth)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if extra, err := io.ReadAll(io.LimitReader(f, 1)); err != nil || len(extra) != 0 {
		fmt.Fprintln(os.Stderr, "input contains data after the first frame")
		return 2
	}

	target := cbarcheck.Config{Range: rangeMode, Matrix: matrix, Depth: targetDepth}
	thresholds := cbarcheck.Thresholds{Luma: opt.lumaTol, Chroma: opt.chromaTol}
	report, err := cbarcheck.Analyze(frame, layout, target, thresholds)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	report.Source = opt.source
	if report.Source == "" {
		report.Source = opt.input
	}
	if opt.out != "" {
		prefix := opt.name
		if prefix == "" {
			prefix = filepath.Base(opt.input)
			prefix = prefix[:len(prefix)-len(filepath.Ext(prefix))]
		}
		paths, err := writeArtifacts(opt.out, prefix, report, "彩条/灰阶合规报告")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Printf("reports: %s, %s\n", paths.JSON, paths.MD)
	}
	printFirstViolation("audit", report)
	fmt.Printf("target=%s best=%s scores target/best=%.3f/%.3f configMismatch=%t\n",
		target, report.Summary.Best, report.Summary.TargetScore,
		report.Summary.BestScore, report.Summary.ConfigurationMismatch)
	if !report.Summary.Compliant || report.Summary.ConfigurationMismatch {
		return 1
	}
	return 0
}

func checkUsage(fs *flag.FlagSet) {
	fmt.Fprint(fs.Output(), `Usage:
  cbarcheck check -input frame.yuv -width W -height H -depth 10 [options]

Input is exactly one planar yuv444p frame (Y, Cb, Cr). The built-in pattern is
eight vertical colour bars over an eleven-step grayscale ramp. The audit reports
the first failing pixel in raster order and compares every region against all
18 combinations of limited/full, BT.601/709/2020, and 8/10/12-bit.

Options:
`)
	fs.PrintDefaults()
}
