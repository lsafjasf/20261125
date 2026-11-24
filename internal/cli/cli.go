package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"ffv1compare/internal/compare"
	"ffv1compare/internal/report"
)

const usage = `ffv1compare - compare two FFV1/Matroska captures frame by frame

Usage:
  ffv1compare -a machine-a.mkv -b machine-b.mkv [-hash-a a.framemd5] [-hash-b b.framemd5]
  ffv1compare machine-a.mkv machine-b.mkv

The optional hash files use FFmpeg's framemd5/framecrc text format. They are
inputs produced by the machines under test; this tool never invokes ffmpeg.
`

func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ffv1compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	aPath := fs.String("a", "", "capture A (.mkv with FFV1)")
	bPath := fs.String("b", "", "capture B (.mkv with FFV1)")
	hashA := fs.String("hash-a", "", "optional decoded-frame hash manifest for A")
	hashB := fs.String("hash-b", "", "optional decoded-frame hash manifest for B")
	jsonOut := fs.Bool("json", false, "write JSON instead of text")
	outPath := fs.String("o", "", "write report to file instead of stdout")
	maxFrames := fs.Int("max-frames", 0, "limit comparison to the first N frames")
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	pos := fs.Args()
	if *aPath == "" && len(pos) > 0 {
		*aPath = pos[0]
		pos = pos[1:]
	}
	if *bPath == "" && len(pos) > 0 {
		*bPath = pos[0]
		pos = pos[1:]
	}
	if *aPath == "" || *bPath == "" || len(pos) > 0 {
		fs.Usage()
		return 2
	}

	a, err := compare.LoadCapture(*aPath, *hashA)
	if err != nil {
		fmt.Fprintf(stderr, "A: %v\n", err)
		return 2
	}
	b, err := compare.LoadCapture(*bPath, *hashB)
	if err != nil {
		fmt.Fprintf(stderr, "B: %v\n", err)
		return 2
	}
	if *maxFrames > 0 {
		if len(a.Frames) > *maxFrames {
			a.Frames = a.Frames[:*maxFrames]
		}
		if len(b.Frames) > *maxFrames {
			b.Frames = b.Frames[:*maxFrames]
		}
	}
	res := compare.Compare(a, b)

	var out io.Writer = stdout
	var f *os.File
	if *outPath != "" {
		f, err = os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(stderr, "output: %v\n", err)
			return 2
		}
		defer f.Close()
		out = f
	}
	if *jsonOut {
		if err := report.WriteJSON(out, res); err != nil {
			fmt.Fprintf(stderr, "report: %v\n", err)
			return 2
		}
	} else {
		report.WriteText(out, res)
	}
	return 0
}
