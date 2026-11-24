package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"editlist-audit/internal/audit"
)

func main() {
	fs := flag.NewFlagSet("editlist-audit", flag.ExitOnError)
	markdownPath := fs.String("markdown", "", "write Markdown report to this file ('-' means stdout)")
	jsonPath := fs.String("json", "", "write machine-readable JSON report to this file ('-' means stdout)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: %s [options] <movie-file-or-directory>...\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(fs.Output(), "Read-only MOV/MP4 edit-list auditor. Defaults to testdata/cases when no input is supplied.")
		fs.PrintDefaults()
	}
	_ = fs.Parse(reorderInterspersedFlags(os.Args[1:]))

	report, err := audit.AuditPaths(fs.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit inputs: %v\n", err)
		os.Exit(1)
	}

	if *jsonPath != "" {
		if err := writeOutput(*jsonPath, renderJSON(report)); err != nil {
			fmt.Fprintf(os.Stderr, "write JSON report: %v\n", err)
			os.Exit(1)
		}
	}

	markdown := audit.RenderMarkdown(report)
	if *markdownPath == "" && *jsonPath == "" {
		fmt.Print(markdown)
		return
	}
	if *markdownPath != "" {
		if err := writeOutput(*markdownPath, []byte(markdown)); err != nil {
			fmt.Fprintf(os.Stderr, "write Markdown report: %v\n", err)
			os.Exit(1)
		}
	}
}

func reorderInterspersedFlags(args []string) []string {
	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			takesValue := (name == "markdown" || name == "json") && !strings.Contains(arg, "=")
			if takesValue && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flags, positional...)
}

func renderJSON(report audit.Report) []byte {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "encode JSON: %v\n", err)
		os.Exit(1)
	}
	return buf.Bytes()
}

func writeOutput(path string, data []byte) error {
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
