package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "rec709shoulder: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runAll(nil)
	}
	switch args[0] {
	case "all":
		return runAll(args[1:])
	case "demo":
		return runDemo(args[1:])
	case "process":
		return runProcess(args[1:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		fs := flag.NewFlagSet("root", flag.ContinueOnError)
		printUsage(fs.Output())
		return fmt.Errorf("unknown command %q", args[0])
	}
}
