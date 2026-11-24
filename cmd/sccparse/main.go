package main

import (
	"fmt"
	"os"

	"sccparse/internal/scc"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: sccparse <file.scc>\n")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	frames, errs := scc.Parse(f)
	fmt.Print(scc.Report(frames, errs))
}
