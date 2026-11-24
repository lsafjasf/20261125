package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"rangeaudit/internal/audit"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "rangeaudit:", err)
		os.Exit(2)
	}
}

func run() error {
	var (
		manifest   = flag.String("manifest", "", "JSON batch manifest")
		declared   = flag.String("declared", "unknown", "player tag: full, limited, or unknown")
		width      = flag.Int("width", 0, "raw YUV frame width")
		height     = flag.Int("height", 0, "raw YUV frame height")
		pixelFmt   = flag.String("pixfmt", "", "raw pixel format: gray, yuv420p, yuv422p, yuv444p")
		maxSamples = flag.Uint64("max-samples", audit.DefaultMaxSamples, "maximum sampled luma values per clip")
		mdPath     = flag.String("md", "-", "Markdown report path, or - for stdout")
		jsonPath   = flag.String("json", "", "JSON report path, or - for stdout")
		version    = flag.Bool("version", false, "print version")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: rangeaudit [flags] FILE...\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Analyze luma range evidence and black level from PNG/JPEG/GIF, raw YUV, or Y4M.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *version {
		fmt.Println(audit.ToolVersion)
		return nil
	}
	if *mdPath == "-" && *jsonPath == "-" {
		return fmt.Errorf("only one report may be written to stdout")
	}

	declaredRange, err := audit.ParseDeclared(*declared)
	if err != nil {
		return err
	}
	var (
		specs   []audit.ClipSpec
		baseDir string
	)
	if *manifest != "" {
		if flag.NArg() > 0 {
			return fmt.Errorf("use either -manifest or positional files, not both")
		}
		specs, baseDir, err = audit.LoadManifest(*manifest)
		if err != nil {
			return err
		}
		if declaredRange != audit.DeclaredUnknown {
			for i := range specs {
				specs[i].Declared = declaredRange
			}
		}
	} else {
		if flag.NArg() == 0 {
			flag.Usage()
			return fmt.Errorf("no input files")
		}
		specs = audit.SpecsFromPaths(flag.Args(), declaredRange, *width, *height, *pixelFmt, *maxSamples)
		baseDir = "."
	}

	report := audit.Run(specs, baseDir, *maxSamples)
	if *jsonPath != "" {
		if err := writeOutput(*jsonPath, func(w io.Writer) error {
			return audit.WriteJSON(w, report)
		}); err != nil {
			return err
		}
	}
	if err := writeOutput(*mdPath, func(w io.Writer) error {
		return audit.WriteMarkdown(w, report)
	}); err != nil {
		return err
	}
	return nil
}

func writeOutput(path string, write func(io.Writer) error) error {
	if path == "-" {
		return write(os.Stdout)
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return write(f)
}
