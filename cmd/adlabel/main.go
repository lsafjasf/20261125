// Command adlabel repairs batched language and duration labels of an ad media
// library. It only ever rewrites the two allowed sidecar fields, keeps the
// original media hash stable, and applies each correction group atomically.
package main

import (
	"flag"
	"fmt"
	"os"

	"adlabel/internal/fixture"
	"adlabel/internal/repair"
	"adlabel/internal/report"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "sample":
		err = runSample(os.Args[2:])
	case "chain":
		err = runChain(os.Args[2:])
	case "repair":
		err = runRepair(os.Args[2:], false)
	case "dry-run":
		err = runRepair(os.Args[2:], true)
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "adlabel:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `adlabel - batch ad label repair (pure Go)

usage:
  adlabel sample -out DIR
      Generate a deterministic demo library plus correction batches that
      cover same-name multi-language items, read-only sidecars and failures.

  adlabel chain -library DIR -corrections FILE
      Print the field authority chain verdict for every correction without
      writing anything (always read-only).

  adlabel dry-run -library DIR -corrections FILE [-all-or-nothing]
      Validate and stage every group; report what would change.

  adlabel repair -library DIR -corrections FILE [-all-or-nothing] \
      [-report-json FILE]
      Apply corrections. Each group commits both fields or rolls back. With
      -all-or-nothing the whole batch shares one transaction.
`)
}

func runSample(args []string) error {
	fs := flag.NewFlagSet("sample", flag.ContinueOnError)
	out := fs.String("out", "demo", "output directory for the fixture library")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return fixture.Generate(*out)
}

func runChain(args []string) error {
	libraryDir, correctionsPath, _, allOrNothing, err := parseRepairFlags("chain", args)
	if err != nil {
		return err
	}
	lib, corrections, err := loadInputs(libraryDir, correctionsPath)
	if err != nil {
		return err
	}
	eng := repair.NewEngine(lib, repairDefaultOptions(), allOrNothing, true)
	r, err := eng.Run(corrections)
	if err != nil {
		return err
	}
	report.WriteText(os.Stdout, r)
	return nil
}

func runRepair(args []string, dryRun bool) error {
	libraryDir, correctionsPath, reportJSON, allOrNothing, err := parseRepairFlags("repair", args)
	if err != nil {
		return err
	}
	lib, corrections, err := loadInputs(libraryDir, correctionsPath)
	if err != nil {
		return err
	}
	eng := repair.NewEngine(lib, repairDefaultOptions(), allOrNothing, dryRun)
	r, err := eng.Run(corrections)
	if err != nil {
		return err
	}
	report.WriteText(os.Stdout, r)
	if reportJSON != "" {
		f, cerr := os.Create(reportJSON)
		if cerr != nil {
			return cerr
		}
		defer f.Close()
		if werr := report.WriteJSON(f, r); werr != nil {
			return werr
		}
	}
	if code := report.ExitCode(r); code != 0 {
		os.Exit(code)
	}
	return nil
}

func parseRepairFlags(name string, args []string) (library, corrections, reportJSON string, allOrNothing bool, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&library, "library", "", "library root directory")
	fs.StringVar(&corrections, "corrections", "", "correction batch JSON file")
	fs.StringVar(&reportJSON, "report-json", "", "optional path for the JSON report")
	fs.BoolVar(&allOrNothing, "all-or-nothing", false, "make the whole batch one transaction")
	if err = fs.Parse(args); err != nil {
		return
	}
	if library == "" || corrections == "" {
		err = fmt.Errorf("%s requires -library and -corrections", name)
	}
	return
}
