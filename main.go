package main

import (
	"errors"
	"flag"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const toolVersion = "rotationaudit v0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "rotationaudit:", err)
		os.Exit(1)
	}
}

type cliConfig struct {
	input             string
	outDir            string
	at                string
	reference         string
	pixelCorrection   int
	ffprobeBin        string
	ffmpegBin         string
	makeDemo          string
	now               string
	writeContactSheet bool
}

func run(args []string) error {
	fs := flag.NewFlagSet("rotationaudit", flag.ContinueOnError)
	var cfg cliConfig
	fs.StringVar(&cfg.input, "input", "", "video file to inspect")
	fs.StringVar(&cfg.input, "i", "", "video file to inspect (alias)")
	fs.StringVar(&cfg.input, "video", "", "video file to inspect (alias)")
	fs.StringVar(&cfg.outDir, "out", "rotation-out", "output directory for report and comparison frames")
	fs.StringVar(&cfg.outDir, "o", "rotation-out", "output directory (alias)")
	fs.StringVar(&cfg.outDir, "output", "rotation-out", "output directory (alias)")
	fs.StringVar(&cfg.at, "at", "", "frame time (seconds or FFmpeg time syntax); default picks 0.5s for longer clips")
	fs.StringVar(&cfg.reference, "reference", "", "upright reference image used to infer true semantic pixel rotation")
	fs.IntVar(&cfg.pixelCorrection, "pixel-correction-cw", -1, "known clockwise degrees needed to make raw pixels upright: 0, 90, 180, or 270")
	fs.StringVar(&cfg.ffprobeBin, "ffprobe", envOr("ROTATIONAUDIT_FFPROBE", "ffprobe"), "ffprobe binary path")
	fs.StringVar(&cfg.ffmpegBin, "ffmpeg", envOr("ROTATIONAUDIT_FFMPEG", "ffmpeg"), "ffmpeg binary path")
	fs.StringVar(&cfg.makeDemo, "make-demo", "", "create a synthetic landscape-claim/double-rotation MP4 at this path and exit")
	fs.StringVar(&cfg.now, "now", "", "fixed RFC3339 UTC timestamp for reproducible reports")
	fs.BoolVar(&cfg.writeContactSheet, "contact-sheet", true, "write comparison.png contact sheet")
	args = reorderFlags(args, fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cfg.input == "" && fs.NArg() > 0 {
		cfg.input = fs.Arg(0)
	}

	if cfg.makeDemo != "" {
		if err := ensureTools(cfg.ffprobeBin, cfg.ffmpegBin); err != nil {
			return err
		}
		ref, err := MakeDemoFixture(cfg.makeDemo, cfg.ffmpegBin)
		if err != nil {
			return err
		}
		fmt.Printf("demo video: %s\nupright reference: %s\n", cfg.makeDemo, ref)
		return nil
	}
	if cfg.input == "" {
		return errors.New("usage: rotationaudit -input <video> [-out dir] [-reference upright.png] [-pixel-correction-cw 0|90|180|270]")
	}
	if cfg.pixelCorrection != -1 {
		if cfg.pixelCorrection < 0 || cfg.pixelCorrection > 270 || cfg.pixelCorrection%90 != 0 {
			return errors.New("-pixel-correction-cw must be one of 0, 90, 180, 270")
		}
	}

	now := time.Now().UTC()
	if cfg.now != "" {
		parsed, err := time.Parse(time.RFC3339, cfg.now)
		if err != nil {
			return fmt.Errorf("invalid -now: %w", err)
		}
		now = parsed.UTC()
	}
	return auditInput(cfg, now)
}

// reorderFlags lets users put positional input before options. The standard
// flag package stops at the first positional argument.
func reorderFlags(args []string, fs *flag.FlagSet) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}
		flags = append(flags, arg)
		nameValue := strings.TrimLeft(arg, "-")
		if strings.Contains(nameValue, "=") {
			continue
		}
		if defined := fs.Lookup(nameValue); defined != nil {
			boolFlag, isBool := defined.Value.(interface{ IsBoolFlag() bool })
			if !(isBool && boolFlag.IsBoolFlag()) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	return append(flags, positionals...)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func auditInput(cfg cliConfig, now time.Time) error {
	if err := ensureTools(cfg.ffprobeBin, cfg.ffmpegBin); err != nil {
		return err
	}
	probed, err := runFFProbe(cfg.input, cfg.ffprobeBin)
	if err != nil {
		return err
	}
	tracks, mp4Err := ReadMP4TrackDisplays(cfg.input)

	if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
		return err
	}
	at := chooseFrameTime(probed, cfg.at)
	rawPath := filepath.Join(cfg.outDir, "01-raw-pixels-noautorotate.png")
	if err := extractFrame(cfg.ffmpegBin, cfg.input, at, rawPath, false); err != nil {
		return fmt.Errorf("extract raw no-autorotate frame: %w", err)
	}
	rawW, rawH, err := readImageConfig(rawPath)
	if err != nil {
		return fmt.Errorf("decode extracted raw frame dimensions: %w", err)
	}

	evidence, err := buildEvidence(cfg, rawPath)
	if err != nil {
		return err
	}
	observation := buildObservation(cfg.input, probed, tracks, rawW, rawH, evidence)

	playerPath := filepath.Join(cfg.outDir, "02-matrix-respecting-player.png")
	if err := extractFrame(cfg.ffmpegBin, cfg.input, at, playerPath, true); err != nil {
		return fmt.Errorf("extract autorotated frame: %w", err)
	}

	frames := []FrameArtifact{}
	addFrame := func(role, path, description string) error {
		artifact, err := frameArtifact(role, path, description)
		if err != nil {
			return err
		}
		frames = append(frames, artifact)
		return nil
	}
	if err := addFrame("raw-pixels", rawPath, "FFmpeg -noautorotate: true coded pixel grid, with no display matrix applied."); err != nil {
		return err
	}
	if err := addFrame("matrix-respecting-player", playerPath, "FFmpeg default autorotate: the same matrix-aware behavior expected from a compliant player."); err != nil {
		return err
	}

	doublePath := filepath.Join(cfg.outDir, "03-double-rotated-autorotate-plus-manual.png")
	if observation.MatrixActive {
		playerImg, err := loadImage(playerPath)
		if err != nil {
			return err
		}
		doubleImg := rotateTurns(playerImg, observation.MatrixTurnsCW)
		if err := savePNG(doublePath, doubleImg); err != nil {
			return err
		}
		if err := addFrame("double-rotation-pipeline", doublePath, "Default autorotated frame rotated again by the same matrix quarter-turn: the C3 pipeline mistake."); err != nil {
			return err
		}
	}

	correctedPath, _, err := writeCorrectedFrames(cfg, observation, rawPath, addFrame)
	if err != nil {
		return err
	}

	sheetPath := filepath.Join(cfg.outDir, "comparison.png")
	if cfg.writeContactSheet {
		panels := []ComparisonPanel{
			{Number: 1, Path: rawPath, Border: colorBlue()},
			{Number: 2, Path: playerPath, Border: colorAmber()},
		}
		if observation.MatrixActive {
			panels = append(panels, ComparisonPanel{Number: 3, Path: doublePath, Border: colorRed()})
		}
		if correctedPath != "" {
			panels = append(panels, ComparisonPanel{Number: 4, Path: correctedPath, Border: colorGreen()})
		}
		if err := WriteComparisonSheet(sheetPath, panels); err != nil {
			return err
		}
		if err := addFrame("comparison-sheet", sheetPath, "Numbered contact sheet: 1 raw pixels, 2 matrix-aware output, 3 double-rotation pipeline, 4 selected correction."); err != nil {
			return err
		}
	}

	combos := DiagnoseCombos(observation)
	report := &Report{
		SchemaVersion:  "rotationaudit.report/v1",
		Tool:           toolVersion,
		GoVersion:      runtime.Version(),
		CreatedUTC:     now.Format(time.RFC3339),
		OutputDir:      cfg.outDir,
		Observation:    observation,
		Combinations:   combos,
		HighRisk:       HighRiskCombos(combos),
		Frames:         frames,
		Conclusion:     buildConclusion(observation, combos),
		NextSteps:      buildNextSteps(cfg, observation),
		RepairCommands: buildRepairCommands(cfg, observation, at),
	}
	if mp4Err != nil {
		report.Warnings = append(report.Warnings, "MP4 tkhd parse skipped/failed: "+mp4Err.Error())
	}
	report.FFmpegVersion = firstLine(cfg.ffmpegBin, "-version")
	report.FFProbeVersion = firstLine(cfg.ffprobeBin, "-version")

	if err := writeReportFiles(cfg.outDir, report); err != nil {
		return err
	}
	printConsoleSummary(report)
	return nil
}

func buildEvidence(cfg cliConfig, rawPath string) (PixelEvidence, error) {
	if cfg.pixelCorrection != -1 {
		turns := normTurnsCW(cfg.pixelCorrection / 90)
		deg := turns * 90
		return PixelEvidence{
			CorrectionTurnsCW: &turns,
			CorrectionDegCW:   &deg,
			Source:            "explicit-cli",
			Note:              "Provided via -pixel-correction-cw.",
		}, nil
	}
	if cfg.reference != "" {
		turns, maes, err := inferCorrectionFromReference(rawPath, cfg.reference)
		if err != nil {
			return PixelEvidence{}, err
		}
		deg := turns * 90
		return PixelEvidence{
			CorrectionTurnsCW: &turns,
			CorrectionDegCW:   &deg,
			Source:            "reference-image-mae",
			ReferencePath:     cfg.reference,
			CandidateMAE:      maes,
			Note:              "Lowest 72x72 RGB mean absolute error among raw/cw0,cw90,cw180,cw270 candidates.",
		}, nil
	}
	return PixelEvidence{Source: "unknown", Note: "No semantic orientation source supplied; pixel grid orientation is known, upright direction is not."}, nil
}

func colorBlue() color.RGBA  { return color.RGBA{R: 37, G: 99, B: 235, A: 255} }
func colorAmber() color.RGBA { return color.RGBA{R: 217, G: 119, B: 6, A: 255} }
func colorRed() color.RGBA   { return color.RGBA{R: 220, G: 38, B: 38, A: 255} }
func colorGreen() color.RGBA { return color.RGBA{R: 22, G: 163, B: 74, A: 255} }

func printConsoleSummary(r *Report) {
	o := r.Observation
	fmt.Printf("%s\n", r.Tool)
	fmt.Printf("input=%s\n", o.InputPath)
	fmt.Printf("pixels=%dx%d(%s) coded=%dx%d tkhd=%.2fx%.2f(%s) matrixCW=%d matrixOutput=%dx%d\n",
		o.PixelWidth, o.PixelHeight, o.PixelOrient,
		o.CodedWidth, o.CodedHeight,
		o.TrackDisplayWidth, o.TrackDisplayHeight, o.DisplayClaimOrient,
		o.MatrixTurnsCW, o.MatrixOutputWidth, o.MatrixOutputHeight)
	if o.ResidualTurnsCW != nil {
		fmt.Printf("semanticResidual=%d quarter-turn(s)\n", *o.ResidualTurnsCW)
	}
	fmt.Printf("highRiskCombos=%d\n", len(r.HighRisk))
	for _, c := range r.HighRisk {
		fmt.Printf("  ! %s [%s] %s\n", c.ID, c.Status, c.Title)
	}
	fmt.Printf("report=%s\n", filepath.Join(r.OutputDir, "rotation-report.json"))
}
