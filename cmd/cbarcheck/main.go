package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"cbarcheck"
)

const version = "1.0.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "demo":
		return runDemo(args[1:])
	case "check":
		return runCheck(args[1:])
	case "version", "-version", "--version":
		fmt.Printf("cbarcheck %s (schema %s)\n", version, cbarcheck.Schema)
		return 0
	case "help", "-h", "--help":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `cbarcheck - broadcast colour-bar and grayscale compliance audit

Usage:
  cbarcheck check -input frame.yuv -width W -height H -depth 8|10|12 [options]
  cbarcheck demo [-out artifacts/cbarcheck-demo] [options]
  cbarcheck version

Input is one planar yuv444p frame. 8-bit uses one byte per sample; 10/12-bit
use little-endian uint16 words. Export a decoded frame with ffmpeg, for example:

  ffmpeg -i input.mp4 -frames:v 1 -pix_fmt yuv444p10le -f rawvideo frame.yuv

Run "cbarcheck check -h" for audit options.
`)
}

func parseRange(v string) (cbarcheck.RangeMode, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "limited", "tv", "video", "l":
		return cbarcheck.RangeLimited, nil
	case "full", "pc", "f":
		return cbarcheck.RangeFull, nil
	default:
		return "", fmt.Errorf("range must be limited or full, got %q", v)
	}
}

func parseMatrix(v string) (cbarcheck.Matrix, error) {
	s := strings.NewReplacer(".", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(v)))
	switch s {
	case "601", "bt601", "rec601", "smpte170m":
		return cbarcheck.MatrixBT601, nil
	case "709", "bt709", "rec709":
		return cbarcheck.MatrixBT709, nil
	case "2020", "bt2020", "rec2020":
		return cbarcheck.MatrixBT2020, nil
	default:
		return "", fmt.Errorf("matrix must be bt601, bt709, or bt2020, got %q", v)
	}
}

func parseDepth(v string) (cbarcheck.BitDepth, error) {
	s := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), "-bit")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("bit depth must be 8, 10, or 12, got %q", v)
	}
	switch cbarcheck.BitDepth(n) {
	case cbarcheck.Depth8, cbarcheck.Depth10, cbarcheck.Depth12:
		return cbarcheck.BitDepth(n), nil
	default:
		return 0, fmt.Errorf("bit depth must be 8, 10, or 12, got %d", n)
	}
}

func parsePattern(v string) (cbarcheck.Pattern, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "ebu75", "75", "ebu":
		return cbarcheck.PatternEBU75, nil
	case "smpte100", "100", "smpte":
		return cbarcheck.PatternSMPTE100, nil
	default:
		return "", fmt.Errorf("pattern must be ebu75 or smpte100, got %q", v)
	}
}
