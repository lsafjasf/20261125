package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"icccmp/icc"
)

func main() {
	jsonOut := flag.Bool("json", false, "emit the comparison report as JSON")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [--json] <profile-v2.icc> <profile-v4.icc>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}

	left, err := icc.ParseFile(flag.Arg(0))
	if err != nil {
		fatal("cannot read %s: %v", flag.Arg(0), err)
	}
	right, err := icc.ParseFile(flag.Arg(1))
	if err != nil {
		fatal("cannot read %s: %v", flag.Arg(1), err)
	}

	report := icc.Compare(left, right)
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fatal("json encode failed: %v", err)
		}
		return
	}
	printText(report)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "icccmp: "+format+"\n", args...)
	os.Exit(1)
}
