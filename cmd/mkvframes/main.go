package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	matroska "mkvparse"
)

func main() {
	var opts matroska.ScanOptions
	var pretty bool
	flag.Int64Var(&opts.TimestampScale, "scale", 0, "override Matroska timestamp scale in nanoseconds")
	flag.Int64Var(&opts.DefaultFrameDuration, "frame-duration", 0, "override per-frame duration in nanoseconds")
	flag.BoolVar(&opts.ReportUnknown, "unknown", false, "report skipped unknown elements")
	flag.BoolVar(&pretty, "pretty", true, "indent JSON output")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [options] file.mkv\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	result, err := matroska.ScanFile(flag.Arg(0), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkvframes: %v\n", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "mkvframes: %v\n", err)
		os.Exit(1)
	}
}
