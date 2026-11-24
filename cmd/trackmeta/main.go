// trackmeta audits per-track colour metadata of MP4/MOV files without
// relying on any external probing tool.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"trackmeta/mp4"
	"trackmeta/report"
)

func main() {
	strict := flag.Bool("strict", false, "exit non-zero when inconsistencies are found")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: trackmeta [-strict] <file.mp4|file.mov>")
		os.Exit(2)
	}
	path := flag.Arg(0)
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	tracks, err := mp4.ParseFile(f, st.Size())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	rep := report.Build(path, tracks)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if *strict {
		for _, iss := range rep.Inconsistencies {
			if iss.Severity == "error" {
				os.Exit(1)
			}
		}
	}
}
