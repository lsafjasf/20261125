package main

import (
	"os"

	"avsyncdiag/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
