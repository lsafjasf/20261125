package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"videotear/internal/vdetect"
)

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "-version" || os.Args[1] == "--version") {
		fmt.Println("videotear 1.0.0")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "detect" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}

	fs := flag.NewFlagSet("videotear", flag.ExitOnError)
	var inputs stringSlice
	fs.Var(&inputs, "i", "input image/glob/dir/video/raw path; repeatable")
	fs.Var(&inputs, "input", "alias for -i")
	format := fs.String("format", "json", "output format: json, jsonl, text")
	output := fs.String("o", "", "output file (default stdout)")
	sensitivity := fs.Float64("sensitivity", 0.62, "detection sensitivity 0..1")
	minConfidence := fs.Float64("min-confidence", 0.55, "minimum reported confidence")
	analysisWidth := fs.Int("analysis-width", 448, "luma analysis width")
	maxShiftRatio := fs.Float64("max-shift-ratio", 0.09, "maximum seam shift as width ratio")
	width := fs.Int("width", 0, "raw frame width")
	height := fs.Int("height", 0, "raw frame height")
	pixelFormat := fs.String("pixel-format", "rgb24", "raw format: gray, rgb24, rgba, yuv420p")
	fps := fs.Float64("fps", 30, "FPS for PTS/track gaps (raw/image fallback)")
	failOnFindings := fs.Bool("fail-on-findings", false, "exit 2 when report contains findings")
	_ = fs.Parse(reorderArgs(fs, os.Args[1:]))

	args := append([]string(nil), inputs...)
	args = append(args, fs.Args()...)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: videotear detect [flags] INPUT [INPUT...]")
		fmt.Fprintln(os.Stderr, "inputs: PNG/JPEG sequence/dir/glob, .gray/.rgb/.yuv with dimensions, or video via ffmpeg")
		os.Exit(2)
	}

	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })

	opt := vdetect.DefaultOptions()
	opt.Sensitivity = *sensitivity
	opt.MinConfidence = *minConfidence
	opt.AnalysisWidth = *analysisWidth
	opt.MaxShiftRatio = *maxShiftRatio
	opt.Width = *width
	opt.Height = *height
	opt.PixelFormat = *pixelFormat
	opt.FPS = *fps

	rep, err := run(args, opt, visited)
	if err != nil {
		fmt.Fprintf(os.Stderr, "videotear: %v\n", err)
		os.Exit(1)
	}
	if err := writeReport(rep, *format, *output); err != nil {
		fmt.Fprintf(os.Stderr, "videotear: write output: %v\n", err)
		os.Exit(1)
	}
	if *failOnFindings && rep.Summary.FramesWithFindings > 0 {
		os.Exit(2)
	}
}

func reorderArgs(fs *flag.FlagSet, args []string) []string {
	valueFlags := map[string]bool{
		"i": true, "input": true, "format": true, "o": true,
		"sensitivity": true, "min-confidence": true, "analysis-width": true,
		"max-shift-ratio": true, "width": true, "height": true,
		"pixel-format": true, "fps": true,
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if before, _, ok := strings.Cut(name, "="); ok {
			name = before
			flags = append(flags, arg)
			continue
		}
		flags = append(flags, arg)
		if valueFlags[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	_ = fs
	return append(flags, positional...)
}

func run(inputs []string, opt vdetect.Options, visited map[string]bool) (vdetect.Report, error) {
	analyzer := vdetect.NewAnalyzer(opt)
	label := strings.Join(inputs, ", ")

	imagePaths, err := vdetect.ListImagePaths(inputs)
	if err != nil {
		return vdetect.Report{}, err
	}
	videoInputs := filterPaths(inputs, isVideoPath)
	rawInputs := filterPaths(inputs, isRawPath)

	if len(imagePaths) > 0 {
		for _, p := range imagePaths {
			f, err := vdetect.LoadImage(p)
			if err != nil {
				return vdetect.Report{}, err
			}
			analyzer.Process(f)
		}
		return analyzer.Report(label), nil
	}

	if len(videoInputs) == 1 && len(inputs) == 1 {
		meta, err := probeVideo(videoInputs[0])
		if err == nil {
			if meta.Width > 0 {
				opt.Width, opt.Height = meta.Width, meta.Height
			}
			if meta.FPS > 0 && !visited["fps"] {
				opt.FPS = meta.FPS
			}
			opt.PixelFormat = "rgb24"
		} else if opt.Width <= 0 || opt.Height <= 0 {
			return vdetect.Report{}, err
		}
		reader, cleanup, err := startVideoReader(videoInputs[0])
		if err != nil {
			return vdetect.Report{}, err
		}
		defer cleanup()
		if err := readRawSequence(reader, analyzer, opt); err != nil {
			return vdetect.Report{}, err
		}
		return analyzer.Report(label), nil
	}
	if len(videoInputs) > 0 {
		return vdetect.Report{}, errors.New("pass one video input per invocation")
	}

	if len(rawInputs) >= 1 {
		if opt.Width <= 0 || opt.Height <= 0 {
			return vdetect.Report{}, errors.New("raw input requires -width and -height")
		}
		for _, p := range rawInputs {
			var r io.Reader
			var closer io.Closer
			if p == "-" {
				r = os.Stdin
			} else {
				f, err := os.Open(p)
				if err != nil {
					return vdetect.Report{}, err
				}
				r, closer = f, f
			}
			if err := readRawSequence(r, analyzer, opt); err != nil {
				if closer != nil {
					_ = closer.Close()
				}
				return vdetect.Report{}, err
			}
			if closer != nil {
				_ = closer.Close()
			}
		}
		return analyzer.Report(label), nil
	}

	return vdetect.Report{}, fmt.Errorf("no PNG/JPEG frames found and input is not a supported video/raw path: %s", label)
}

func readRawSequence(r io.Reader, a *vdetect.Analyzer, opt vdetect.Options) error {
	index := 0
	for {
		f, err := vdetect.ReadRawFrame(r, index, opt)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return fmt.Errorf("truncated raw frame at index %d", index)
			}
			return nil
		}
		if err != nil {
			return err
		}
		a.Process(f)
		index++
	}
}

func filterPaths(paths []string, keep func(string) bool) []string {
	var out []string
	for _, p := range paths {
		if keep(p) {
			out = append(out, p)
		}
	}
	return out
}
