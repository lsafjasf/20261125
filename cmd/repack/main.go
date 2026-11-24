package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"repack"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "repack":
		runRepack(os.Args[2:])
	case "verify":
		runVerify(os.Args[2:])
	case "gen-testdata":
		runGenTestData(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runRepack(args []string) {
	fs := flag.NewFlagSet("repack", flag.ContinueOnError)
	inputs := multiFlag{}
	fs.Var(&inputs, "in", "input pack path (repeatable); relative paths stay inside -base")
	out := fs.String("out", "", "output pack path")
	base := fs.String("base", "", "base directory for relative paths (defaults to cwd)")
	failAt := fs.String("fail-at", "", "inject failure: after_write|before_rename|after_rename|verify_output")
	corrupt := fs.Bool("corrupt-temp", false, "flip a byte in the temporary output before verification")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if len(inputs) == 0 || *out == "" {
		fmt.Fprintln(os.Stderr, "repack requires at least one -in and one -out")
		fs.PrintDefaults()
		os.Exit(2)
	}
	baseDir := *base
	if baseDir != "" {
		abs, err := filepath.Abs(baseDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve -base: %v\n", err)
			os.Exit(2)
		}
		baseDir = abs
	}
	opts := repack.Options{
		Inputs:      inputs,
		Output:      *out,
		Base:        baseDir,
		FailAt:      repack.FailStage(*failAt),
		CorruptTemp: *corrupt,
	}
	if err := repack.Repack(opts); err != nil {
		fmt.Fprintf(os.Stderr, "repack failed (rolled back): %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("committed %s from %d input(s)\n", *out, len(inputs))
}

func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	path := fs.String("pack", "", "pack file to verify")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *path == "" {
		fmt.Fprintln(os.Stderr, "verify requires -pack")
		os.Exit(2)
	}
	boxes, err := repack.ReadPack(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%s OK: %d boxes\n", *path, len(boxes))
}

func runGenTestData(args []string) {
	fs := flag.NewFlagSet("gen-testdata", flag.ContinueOnError)
	dir := fs.String("dir", "", "directory to write sample language packs into")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "gen-testdata requires -dir")
		os.Exit(2)
	}
	if err := repack.GenerateSamplePacks(*dir); err != nil {
		fmt.Fprintf(os.Stderr, "gen-testdata failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("sample packs written under %s\n", *dir)
}

type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint([]string(*m)) }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `repack - transactional multi-language media repackager

Usage:
  repack repack -in a.mcpack -in b.mcpack -out merged.mcpack [-base DIR]
                 [-fail-at STAGE] [-corrupt-temp]
  repack verify -pack merged.mcpack
  repack gen-testdata -dir ./sample

Inputs are opened read-only. The output is written via a same-directory
temp file, verified, atomically renamed into place, and rolled back on any
failure. Relative -in/-out paths are confined to -base (defaults to cwd).
`)
}
