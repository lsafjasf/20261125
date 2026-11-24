// Package cli implements the ancscan command line interface.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"ancscan/anc"
)

// Run executes the CLI and returns the process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}
	switch args[0] {
	case "scan":
		return runScan(args[1:], stdin, stdout, stderr)
	case "gen-scenario":
		return runGen(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `ancscan - SMPTE ANC/VANC line scanner

Usage:
  ancscan scan [file] [flags]
  ancscan gen-scenario [--out path]

Commands:
  scan            parse a JSON/JSONL document or hex-lines text
  gen-scenario    print a rich example input document

Scan flags:
  --format            json (default) or text
  --input-format      auto/json (default), jsonl, hexlines, or raw
  --encoding          fallback encoding: u16be (default), u16le, anc8, words, v210
  --continuation-lines N   following lines used for truncation reassembly (default 1)
  --line-bytes N      raw input: fixed line width in bytes
  --out               write output to file instead of stdout
  --compact           compact JSON output

Use "-" as file to read stdin.`)
}

type scanFlags struct {
	format       string
	inputFormat  string
	encoding     string
	out          string
	continuation int
	compact      bool
	lineBytes    int
}

func runScan(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f scanFlags
	fs.StringVar(&f.format, "format", "json", "output format: json|text")
	fs.StringVar(&f.inputFormat, "input-format", "auto", "input format: auto|json|hexlines")
	fs.StringVar(&f.encoding, "encoding", "", "fallback line encoding")
	fs.StringVar(&f.out, "out", "", "output file path")
	fs.IntVar(&f.continuation, "continuation-lines", 1, "following lines to consult for continuation")
	fs.BoolVar(&f.compact, "compact", false, "compact JSON")
	fs.IntVar(&f.lineBytes, "line-bytes", 0, "raw input: fixed video-line width in bytes (0 = whole file one line)")

	flagArgs, positionals := splitInterspersedArgs(args, map[string]bool{
		"format": true, "input-format": true, "encoding": true,
		"out": true, "continuation-lines": true, "line-bytes": true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	var in io.Reader = stdin
	inputName := "-"
	if len(positionals) > 0 {
		inputName = positionals[0]
		if inputName != "-" {
			file, err := os.Open(inputName)
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return 1
			}
			defer file.Close()
			in = file
		}
	}

	opts := &anc.Options{
		Encoding:          anc.Encoding(f.encoding),
		ContinuationLines: f.continuation,
	}

	var rep *anc.Report
	switch f.inputFormat {
	case "json", "jsonl", "auto", "":
		var err error
		rep, err = anc.ParseAndScan(in, opts)
		if err != nil {
			fmt.Fprintf(stderr, "scan error: %v\n", err)
			return 1
		}
	case "hexlines":
		lines, err := anc.ParseHexLines(in, anc.Encoding(firstNonEmpty(f.encoding, "anc8")))
		if err != nil {
			fmt.Fprintf(stderr, "parse error: %v\n", err)
			return 1
		}
		rep, err = anc.Scan(lines, opts)
		if err != nil {
			fmt.Fprintf(stderr, "scan error: %v\n", err)
			return 1
		}
	case "raw", "binary", "bytes":
		data, err := io.ReadAll(in)
		if err != nil {
			fmt.Fprintf(stderr, "read error: %v\n", err)
			return 1
		}
		enc := anc.Encoding(firstNonEmpty(f.encoding, "anc8"))
		var lines []anc.Line
		if f.lineBytes > 0 {
			for start, n := 0, 1; start < len(data); start, n = start+f.lineBytes, n+1 {
				end := start + f.lineBytes
				if end > len(data) {
					end = len(data)
				}
				lines = append(lines, anc.Line{Number: n, Bytes: data[start:end], Encoding: enc})
			}
		} else {
			lines = []anc.Line{{Number: 1, Bytes: data, Encoding: enc}}
		}
		rep, err = anc.Scan(lines, opts)
		if err != nil {
			fmt.Fprintf(stderr, "scan error: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "unsupported --input-format %q\n", f.inputFormat)
		return 2
	}

	var out io.Writer = stdout
	if f.out != "" {
		file, err := os.Create(f.out)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		defer file.Close()
		out = file
	}

	switch f.format {
	case "json":
		enc := json.NewEncoder(out)
		if !f.compact {
			enc.SetIndent("", "  ")
		}
		enc.SetEscapeHTML(false)
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(stderr, "encode error: %v\n", err)
			return 1
		}
	case "text":
		renderText(out, rep)
	default:
		fmt.Fprintf(stderr, "unsupported --format %q\n", f.format)
		return 2
	}
	return 0
}

// splitInterspersedArgs moves flags (and their values for valueFlags) into
// flagArgs while collecting plain positional arguments separately, so that
// `scan file.json --format text` works as well as `scan --format text file`.
func splitInterspersedArgs(args []string, valueFlags map[string]bool) (flagArgs, positionals []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' {
			flagArgs = append(flagArgs, a)
			name := strings.TrimLeft(a, "-")
			hasEq := strings.Contains(name, "=")
			name = strings.SplitN(name, "=", 2)[0]
			if valueFlags[name] && !hasEq && i+1 < len(args) {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, a)
	}
	return flagArgs, positionals
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
