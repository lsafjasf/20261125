// Command ancscan is the repository-root entry point; the same CLI lives in
// ./cmd/ancscan.
package main

import (
	"os"

	"ancscan/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
