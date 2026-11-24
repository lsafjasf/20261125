package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"ffv1compare/internal/fixture"
)

func main() {
	src := flag.String("a", "testdata/machine-a.mkv", "known-good FFV1/Matroska capture")
	out := flag.String("out", "testdata", "output directory")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	jobs := []struct {
		name string
		fn   func() error
	}{
		{"machine-a-copy.mkv", func() error {
			return fixture.Copy(*src, filepath.Join(*out, "machine-a-copy.mkv"))
		}},
		{"machine-b-reorder.mkv", func() error {
			return fixture.ReorderSlices(*src, filepath.Join(*out, "machine-b-reorder.mkv"), 1, 2)
		}},
		{"machine-b-shift.mkv", func() error {
			return fixture.ShiftTimecodes(*src, filepath.Join(*out, "machine-b-shift.mkv"), 500)
		}},
	}
	for _, job := range jobs {
		if err := job.fn(); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", job.name, err)
			os.Exit(2)
		}
		fmt.Println("wrote", job.name)
	}
	fmt.Println("note: machine-b-slices.mkv and machine-b-444.mkv are checked-in re-encodes used as encoding-setting fixtures")
}
