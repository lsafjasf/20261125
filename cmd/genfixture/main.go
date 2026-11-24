// genfixture writes a synthetic multi-track MP4 mixing HDR and SDR
// material, for exercising trackmeta end to end.
package main

import (
	"fmt"
	"os"

	"trackmeta/testutil"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: genfixture <out.mp4>")
		os.Exit(2)
	}
	if err := os.WriteFile(os.Args[1], testutil.BuildMixedFile(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
