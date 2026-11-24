package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aes3check/aes3"
)

// ExitError carries a desired process exit status.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Main runs the CLI and terminates the process according to the result.
func Main() {
	if err := Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "aes3check: %v\n", err)
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(2)
	}
}

// Run executes the command-line interface.
func Run(args []string) error {
	fs := flag.NewFlagSet("aes3check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	input := fs.String("input", "-", "input edge file (.json, .jsonl, .csv), or - for stdin")
	format := fs.String("format", "auto", "input format: auto, json, jsonl, csv")
	output := fs.String("output", "", "output report file; empty means stdout")
	text := fs.Bool("text", false, "print human-readable text instead of JSON")
	pretty := fs.Bool("pretty", true, "indent JSON output")
	halfUnit := fs.Float64("half-unit", 0, "fixed half-bit/UI2 time in input units; zero recovers it")
	tolerance := fs.Float64("tolerance", 0.35, "edge jitter tolerance as a fraction of half-bit time")
	minScore := fs.Float64("min-score", 1, "minimum structural candidate score from 0 to 1")
	requireParity := fs.Bool("require-parity", false, "reject subframes with parity errors")
	scale := fs.Float64("time-scale", 1, "multiply input timestamps by this before decoding")
	strict := fs.Bool("strict", false, "exit with status 1 when lock intervals or untrusted subframes exist")
	if err := fs.Parse(args); err != nil {
		return err
	}

	edges, err := loadEdges(*input, *format)
	if err != nil {
		return err
	}
	if *scale != 1 {
		for i := range edges {
			edges[i].Time *= *scale
		}
	}

	report, err := aes3.Decode(edges, aes3.Options{
		HalfUnit:         *halfUnit,
		JitterTolerance:  *tolerance,
		MinSubframeScore: *minScore,
		RequireParity:    *requireParity,
	})
	if err != nil {
		return err
	}

	var out []byte
	if *text {
		out = []byte(aes3.FormatText(report))
	} else {
		if *pretty {
			out, err = json.MarshalIndent(report, "", "  ")
		} else {
			out, err = json.Marshal(report)
		}
		if err != nil {
			return err
		}
		out = append(out, '\n')
	}

	writer := io.Writer(os.Stdout)
	if *output != "" {
		file, err := os.Create(*output)
		if err != nil {
			return err
		}
		writer = file
		defer file.Close()
	}
	if _, err := writer.Write(out); err != nil {
		return err
	}
	if *output != "" {
		if closer, ok := writer.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				return err
			}
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *output)
	}

	if *strict && (len(report.LockIntervals) > 0 || report.Summary.Untrusted > 0) {
		return &ExitError{Code: 1, Err: errors.New("decoded report contains lock intervals or untrusted subframes")}
	}
	return nil
}

func loadEdges(path, format string) ([]aes3.Edge, error) {
	var reader io.ReadCloser
	if path == "-" {
		reader = io.NopCloser(os.Stdin)
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		reader = file
	}
	defer reader.Close()

	resolved := strings.ToLower(format)
	if resolved == "auto" {
		resolved = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		if path == "-" || resolved == "" {
			resolved = "json"
		}
		if resolved == "txt" || resolved == "text" {
			resolved = "csv"
		}
	}

	switch resolved {
	case "json":
		return aes3.ParseEdgesJSON(reader)
	case "jsonl", "ndjson":
		return aes3.ParseEdgesJSONLines(reader)
	case "csv":
		return aes3.ParseEdgesCSV(reader)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}
