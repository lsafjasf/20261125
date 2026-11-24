package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"mediaclock"
)

func main() {
	pretty := flag.Bool("pretty", true, "pretty-print JSON report")
	flag.Parse()

	report, err := mediaclock.RunThreeHourAcceptance()
	encoder := json.NewEncoder(os.Stdout)
	if *pretty {
		encoder.SetIndent("", "  ")
	}
	if encodeErr := encoder.Encode(report); encodeErr != nil {
		fmt.Fprintln(os.Stderr, encodeErr)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr,
		"PASS: %d frames over %s; presented=%d dropped=%d max_drift=%s global_seeks=%d\n",
		report.FrameCount,
		report.TimelineDuration,
		report.PresentedFrames,
		report.DroppedFrames,
		report.MaxPresentedDrift,
		report.AutomaticGlobalSeeks+uint64(report.GlobalSeekCalls),
	)
}
