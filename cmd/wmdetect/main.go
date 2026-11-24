// wmdetect audits a WAV dialogue recording for repeated short watermark
// fragments, distinguishing true repeats from natural echo and codec
// ringing. Every conclusion carries sample ranges, bootstrap confidence
// intervals and a measured counterexample. Standard library only.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"watermarkaudit/internal/detect"
	"watermarkaudit/internal/dsp"
	"watermarkaudit/internal/wavio"
)

func main() {
	in := flag.String("in", "", "input WAV file (required)")
	jsonOut := flag.String("json", "", "write machine-readable report here")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: wmdetect -in recording.wav [-json report.json]")
		os.Exit(2)
	}
	w, err := wavio.ReadFile(*in)
	if err != nil {
		fail(err)
	}
	cfg := detect.DefaultConfig(w.SampleRate)
	rep := detect.Analyze(w, dsp.DefaultBands(), cfg)
	rep.File = *in

	fmt.Print(renderText(rep))
	if *jsonOut != "" {
		j, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(*jsonOut, j, 0o644); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "wmdetect:", err)
	os.Exit(1)
}
