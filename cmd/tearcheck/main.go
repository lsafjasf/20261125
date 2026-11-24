package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"tearcheck/internal/analyze"
	"tearcheck/internal/detect"
	"tearcheck/internal/frameio"
	"tearcheck/internal/review"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
	exitTear  = 3
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "analyze":
		return runAnalyze(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "review":
		return runReview(args[1:], stdin, stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "tearcheck %s (%s)\n", analyze.ToolVersion, analyze.SchemaVersion)
		return exitOK
	case "help", "-h", "--help":
		printUsage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return exitUsage
	}
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, `tearcheck - deterministic tearing evidence for decoded frame sequences

Usage:
  tearcheck analyze --input DIR [--manifest FILE] [--review FILE] [--out FILE]
  tearcheck verify  --input DIR [--manifest FILE] [--review FILE]
  tearcheck review  set|unset|list|walk [flags]
  tearcheck version

Input:
  DIR/frames.json maps image files to integer microsecond timestamps. If the
  manifest is absent, sorted PNG/JPEG files are assigned timestamps from
  --fps-num/--fps-den. Go's standard library does not demux MP4/MOV; decode the
  video to frames first and preserve its PTS in the manifest.

Determinism:
  Reports contain no wall-clock time, random IDs, or absolute paths. Running
  analyze twice over the same inputs produces byte-identical JSON.`)
}

type commonFlags struct {
	input      *string
	manifest   *string
	review     *string
	fpsNum     *int
	fpsDen     *int
	bandRows   *int
	maxShift   *int
	minShift   *int
	minMatch   *int
	minGain    *int
	maxBias    *int
	minTexture *int
	eventGap   *int
	rowJump    *int
	shiftJump  *int
	minRun     *int
}

func newCommonFlags() *commonFlags {
	return &commonFlags{}
}

func (flags *commonFlags) register(set *flag.FlagSet) {
	defaults := detect.DefaultSettings()
	flags.input = set.String("input", "", "directory containing frames and optional frames.json")
	flags.manifest = set.String("manifest", "", "manifest path (relative to --input, or absolute)")
	flags.review = set.String("review", "", "per-frame review JSON to apply")
	flags.fpsNum = set.Int("fps-num", 30, "fallback frame rate numerator when frames.json is absent")
	flags.fpsDen = set.Int("fps-den", 1, "fallback frame rate denominator when frames.json is absent")
	flags.bandRows = set.Int("band-rows", defaults.BandRows, "rows sampled above and below each seam")
	flags.maxShift = set.Int("max-shift", defaults.MaxShiftPX, "maximum horizontal shift searched, in pixels")
	flags.minShift = set.Int("min-shift", defaults.MinShiftPX, "minimum horizontal shift for a seam")
	flags.minMatch = set.Int("min-match-bp", defaults.MinMatchScoreBP, "minimum shifted-profile match score (basis points)")
	flags.minGain = set.Int("min-gain-bp", defaults.MinShiftGainBP, "minimum improvement over zero-shift match (basis points)")
	flags.maxBias = set.Int("max-bias-bp", defaults.MaxBiasRatioBP, "maximum uniform-row-difference ratio for a structural seam")
	flags.minTexture = set.Int("min-texture", defaults.MinTextureMean, "minimum mean horizontal gradient near a seam")
	flags.eventGap = set.Int("event-gap", defaults.MaxEventGapFrames, "maximum unique-frame gap inside one event")
	flags.rowJump = set.Int("row-jump", defaults.MaxRowJumpPX, "maximum seam row movement between frames")
	flags.shiftJump = set.Int("shift-jump", defaults.MaxShiftJumpPX, "maximum seam offset movement between frames")
	flags.minRun = set.Int("min-continuous", defaults.MinContinuousFrames, "unique frames required for a continuous event")
}

func (flags *commonFlags) validate() error {
	if flags.input == nil || *flags.input == "" {
		return errors.New("--input is required")
	}
	return nil
}

func (flags *commonFlags) settings() detect.Settings {
	settings := detect.DefaultSettings()
	settings.BandRows = *flags.bandRows
	settings.MaxShiftPX = *flags.maxShift
	settings.MinShiftPX = *flags.minShift
	settings.MinMatchScoreBP = *flags.minMatch
	settings.MinShiftGainBP = *flags.minGain
	settings.MaxBiasRatioBP = *flags.maxBias
	settings.MinTextureMean = *flags.minTexture
	settings.MaxEventGapFrames = *flags.eventGap
	settings.MaxRowJumpPX = *flags.rowJump
	settings.MaxShiftJumpPX = *flags.shiftJump
	settings.MinContinuousFrames = *flags.minRun
	return settings
}

func (flags *commonFlags) analyze() (*analyze.Report, error) {
	if err := flags.validate(); err != nil {
		return nil, err
	}
	plan, err := frameio.LoadPlan(*flags.input, *flags.manifest, *flags.fpsNum, *flags.fpsDen)
	if err != nil {
		return nil, err
	}
	reviewFile, err := loadReview(*flags.review)
	if err != nil {
		return nil, err
	}
	return analyze.Analyze(plan, analyze.Options{
		Settings:   flags.settings(),
		ReviewFile: reviewFile,
	})
}

func runAnalyze(args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("analyze", flag.ContinueOnError)
	set.SetOutput(stderr)
	common := newCommonFlags()
	common.register(set)
	out := set.String("out", "-", "report path, or - for stdout")
	failOnTear := set.Bool("fail-on-tear", false, "exit 3 when the final report contains tears")
	quiet := set.Bool("quiet", false, "suppress the summary line when writing a file")
	if err := set.Parse(args); err != nil {
		return exitUsage
	}
	report, err := common.analyze()
	if err != nil {
		fmt.Fprintf(stderr, "analyze: %v\n", err)
		return exitError
	}
	raw, err := analyze.MarshalReport(report)
	if err != nil {
		fmt.Fprintf(stderr, "analyze: %v\n", err)
		return exitError
	}
	if *out == "-" {
		if _, err := stdout.Write(raw); err != nil {
			fmt.Fprintf(stderr, "analyze: write stdout: %v\n", err)
			return exitError
		}
	} else {
		if err := writeAtomic(*out, raw); err != nil {
			fmt.Fprintf(stderr, "analyze: %v\n", err)
			return exitError
		}
		if !*quiet {
			fmt.Fprintf(stdout, "wrote %s: frames=%d tears=%d uncertain=%d light_effects=%d duplicates=%d events=%d\n",
				*out,
				report.Summary.Frames,
				report.Summary.FinalTearFrames,
				report.Summary.FinalUncertainFrames,
				report.Summary.MachineLightEffectFrames,
				report.Summary.DuplicateFrames,
				len(report.Events))
		}
	}
	if *failOnTear && report.Summary.FinalTearFrames > 0 {
		return exitTear
	}
	return exitOK
}

func runVerify(args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("verify", flag.ContinueOnError)
	set.SetOutput(stderr)
	common := newCommonFlags()
	common.register(set)
	if err := set.Parse(args); err != nil {
		return exitUsage
	}
	first, err := common.analyze()
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitError
	}
	second, err := common.analyze()
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitError
	}
	firstRaw, err := analyze.MarshalReport(first)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitError
	}
	secondRaw, err := analyze.MarshalReport(second)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitError
	}
	sum := sha256.Sum256(firstRaw)
	digest := hex.EncodeToString(sum[:])
	if string(firstRaw) != string(secondRaw) {
		fmt.Fprintf(stdout, "identical=false\nsha256=%s\n", digest)
		return exitError
	}
	fmt.Fprintf(stdout, "identical=true\nsha256=%s\n", digest)
	return exitOK
}

func loadReview(path string) (*review.File, error) {
	if path == "" {
		return nil, nil
	}
	file, _, err := review.Load(path)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".tearcheck-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary report: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary report: %w", err)
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return fmt.Errorf("set report permissions: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary report: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace report: %w", err)
	}
	return nil
}
