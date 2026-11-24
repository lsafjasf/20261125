package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"cbarcheck"
)

func demoFlags(args []string) (*flag.FlagSet, *string, *string, *int, *int, *bool) {
	fs := flag.NewFlagSet("cbarcheck demo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("out", "artifacts/cbarcheck-demo", "output directory")
	pattern := fs.String("pattern", "smpte100", "colour-bar pattern: ebu75 or smpte100")
	width := fs.Int("width", 640, "demo frame width")
	height := fs.Int("height", 360, "demo frame height")
	writeYUV := fs.Bool("write-yuv", false, "also write clean.yuv and bad.yuv")
	return fs, out, pattern, width, height, writeYUV
}

func runDemo(args []string) int {
	fs, out, patternText, width, height, writeYUV := demoFlags(args)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	pattern, err := parsePattern(*patternText)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	layout, err := cbarcheck.BarsLayout(pattern, *width, *height)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	target := cbarcheck.Config{Range: cbarcheck.RangeLimited, Matrix: cbarcheck.MatrixBT709, Depth: cbarcheck.Depth10}
	observed := cbarcheck.Config{Range: cbarcheck.RangeFull, Matrix: cbarcheck.MatrixBT601, Depth: cbarcheck.Depth10}
	clean, err := cbarcheck.GenerateFrame(layout, target)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	bad, err := cbarcheck.GenerateFrame(layout, observed)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	cleanReport, err := cbarcheck.Analyze(clean, layout, target, cbarcheck.DefaultThresholds())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	badReport, err := cbarcheck.Analyze(bad, layout, target, cbarcheck.DefaultThresholds())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	cleanReport.Source = "synthetic/clean-limited-bt709-10"
	badReport.Source = "synthetic/transcode-full-bt601-10"
	if !cleanReport.Summary.Compliant || badReport.FirstViolation == nil {
		fmt.Fprintln(os.Stderr, "demo self-check failed: expected clean PASS and defective FAIL")
		return 2
	}

	cleanPaths, err := writeArtifacts(*out, "clean", cleanReport, "彩条/灰阶合规报告：正确 limited/BT.709 10-bit")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	badPaths, err := writeArtifacts(*out, "bad", badReport, "彩条/灰阶合规报告：full/BT.601 错配")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *writeYUV {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		cleanYUV := filepath.Join(*out, "clean.yuv")
		badYUV := filepath.Join(*out, "bad.yuv")
		if err := writeYUVFile(cleanYUV, clean); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if err := writeYUVFile(badYUV, bad); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Printf("YUV: %s, %s\n", cleanYUV, badYUV)
	}

	fmt.Printf("pattern=%s size=%dx%d target=%s observed=%s\n",
		pattern, *width, *height, target, observed)
	printFirstViolation("clean", cleanReport)
	printFirstViolation("bad", badReport)
	fmt.Printf("bad inferred best=%s score=%.3f targetScore=%.3f chromaIllegal=%d\n",
		badReport.Summary.Best, badReport.Summary.BestScore,
		badReport.Summary.TargetScore, badReport.Summary.IllegalChromaPixels)
	fmt.Printf("reports: %s, %s, %s, %s\n",
		cleanPaths.JSON, cleanPaths.MD, badPaths.JSON, badPaths.MD)
	return 0
}
