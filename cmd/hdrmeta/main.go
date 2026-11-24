// hdrmeta derives reviewable HDR static metadata candidates (MaxCLL /
// MaxFALL) from PQ-encoded PNG frame exports. It uses only the Go standard
// library and invokes no external media tools.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"hdrmeta/pkg/analyze"
	"hdrmeta/pkg/pq"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "analyze":
		err = cmdAnalyze(os.Args[2:])
	case "table":
		err = cmdTable(os.Args[2:])
	case "demo":
		err = cmdDemo(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hdrmeta:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `hdrmeta - HDR MaxCLL/MaxFALL candidate tool (Go stdlib only)

Usage:
  hdrmeta analyze [flags] frame.png [more frames...]
  hdrmeta table   [-depth 10] [-clip 1000]
  hdrmeta demo    [-out testdata/demo] [-depth 10]

analyze flags:
  -depth int   signal bit depth: 0=auto, 10/12/16 (default 10)
  -clip float  display clip point in nits (default 1000)
  -gate float  highlight region threshold in nits (default 1000)
  -area float  qualified region minimum area ratio (default 0.001)
`)
}

func cmdAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	depth := fs.Int("depth", 10, "signal bit depth: 0=auto, 10/12/16")
	clip := fs.Float64("clip", 1000, "display clip point in nits")
	gate := fs.Float64("gate", 1000, "highlight region threshold in nits")
	area := fs.Float64("area", 0.001, "qualified region minimum area ratio")
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths := fs.Args()
	if len(paths) == 0 {
		return fmt.Errorf("analyze requires at least one PNG frame")
	}

	frames := make([]*analyze.Frame, 0, len(paths))
	for _, p := range paths {
		f, err := analyze.LoadFrame(p, *depth)
		if err != nil {
			return err
		}
		frames = append(frames, f)
	}
	sort.SliceStable(frames, func(i, j int) bool { return frames[i].Name < frames[j].Name })
	ordered := make([]string, len(frames))
	for i, f := range frames {
		ordered[i] = f.Name
	}

	cfg := analyze.Config{
		HighlightNits: *gate,
		ClipNits:      *clip,
		MinAreaRatio:  *area,
	}
	seq := analyze.AnalyzeSequence(frames, cfg)
	writeReport(os.Stdout, seq, ordered, cfg)
	return nil
}

func cmdTable(args []string) error {
	fs := flag.NewFlagSet("table", flag.ContinueOnError)
	depth := fs.Int("depth", 10, "signal bit depth: 10 or 12")
	clip := fs.Float64("clip", 1000, "clip point in nits")
	if err := fs.Parse(args); err != nil {
		return err
	}

	maxCode := pq.MaxCode(*depth)
	clipCode := pq.NitsToCode(*clip, *depth)
	fmt.Printf("PQ code -> nits table (SMPTE ST 2084, %d-bit, peak 10000 nits)\n", *depth)
	fmt.Printf("clip reference: %.0f nits ~= code %d; codes > %d are clipped on a %.0f-nit display\n\n",
		*clip, clipCode, clipCode, *clip)
	fmt.Printf("%6s %12s %12s\n", "code", "nits", "status")
	step := 1
	if *depth > 10 {
		step = 16
	}
	for code := 0; code <= maxCode; code += step {
		nits := pq.CodeToNits(code, *depth)
		status := ""
		if code > clipCode {
			status = "CLIPPED"
		} else if code == clipCode {
			status = "~clip"
		}
		fmt.Printf("%6d %12.3f %12s\n", code, nits, status)
	}
	return nil
}

func cmdDemo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	out := fs.String("out", "testdata/demo", "output directory for PNG frames")
	depth := fs.Int("depth", 10, "signal bit depth muxed into 16-bit PNG")
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths, err := generateDemoFrames(*out, *depth)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %d demo frames to %s:\n", len(paths), *out)
	for _, p := range paths {
		fmt.Println("  " + p)
	}
	return nil
}
