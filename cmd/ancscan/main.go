// Command ancscan parses SMPTE ANC/VANC packets from captured video lines.
package main

import (
	"os"

	"ancscan/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
