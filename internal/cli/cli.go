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
	"time"

	"avsyncdiag/internal/sync"
)

// Run executes the command line and returns the process exit code.
func Run(arguments []string) int {
	if len(arguments) < 1 {
		usage(os.Stderr)
		return 2
	}

	var err error
	switch arguments[0] {
	case "analyze":
		err = runAnalyze(arguments[1:])
	case "demo":
		err = runDemo(arguments[1:])
	case "-h", "--help", "help":
		usage(os.Stdout)
	default:
		err = fmt.Errorf("unknown command %q", arguments[0])
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "avsync: %v\n", err)
		return 1
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `avsync - frame-correlated capture/PTS/edit/render/playback sync diagnostics

Usage:
  avsync analyze --input trace.jsonl [--positions-out positions.csv] [--json]
  avsync demo [--at 15m] [--fps 25] [--pattern local_frame_skip] [--json]

Input formats: JSONL, JSON envelope/array, or header-based CSV.
Use "avsync analyze --help" or "avsync demo --help" for flags.`)
}

type commonFlags struct {
	fps           float64
	toleranceMS   float64
	jumpMS        float64
	driftBudgetMS float64
	context       int
	asJSON        bool
	positionsOut  string
	frameStride   int64
}

func bindCommonFlags(fs *flag.FlagSet, c *commonFlags) {
	fs.Float64Var(&c.fps, "fps", 25, "video frame rate used to convert frames/time")
	fs.Float64Var(&c.toleranceMS, "tolerance-ms", 2, "position noise tolerance in milliseconds")
	fs.Float64Var(&c.jumpMS, "jump-ms", 20, "minimum persistent step treated as a jump")
	fs.Float64Var(&c.driftBudgetMS, "drift-budget-ms", 80, "drift magnitude considered audible/visible")
	fs.IntVar(&c.context, "context", 5, "neighbor frames included around the first anomaly")
	fs.Int64Var(&c.frameStride, "frame-stride", 0, "expected frame interval for sparse traces; 0 estimates it")
	fs.BoolVar(&c.asJSON, "json", false, "emit machine-readable JSON report")
	fs.StringVar(&c.positionsOut, "positions-out", "", "write complete frame-by-stage position CSV")
}

func (c commonFlags) options() sync.Options {
	return sync.Options{
		FPS:           c.fps,
		Tolerance:     time.Duration(c.toleranceMS * float64(time.Millisecond)),
		JumpThreshold: time.Duration(c.jumpMS * float64(time.Millisecond)),
		DriftBudget:   time.Duration(c.driftBudgetMS * float64(time.Millisecond)),
		Context:       c.context,
		FrameStride:   c.frameStride,
	}
}

func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	input := fs.String("input", "-", "trace file path or - for stdin")
	format := fs.String("format", "auto", "input format: auto, json, jsonl, csv")
	reportOut := fs.String("report-out", "", "write JSON report to this file")
	common := commonFlags{}
	bindCommonFlags(fs, &common)
	if err := fs.Parse(args); err != nil {
		return err
	}

	data, filename, err := readInput(*input)
	if err != nil {
		return err
	}
	records, err := parseTrace(data, filename, *format)
	if err != nil {
		return err
	}
	report, err := sync.Analyze(records, common.options())
	if err != nil {
		return err
	}
	if err := writePositions(common.positionsOut, records); err != nil {
		return err
	}
	if *reportOut != "" {
		if err := writeJSONFile(*reportOut, report); err != nil {
			return err
		}
	}
	return writeReport(os.Stdout, report, common.asJSON)
}

func runDemo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	at := fs.Duration("at", 15*time.Minute, "simulated time of the discontinuity")
	changeFrame := fs.Int64("change-frame", 0, "override first abnormal zero-based frame")
	totalFrames := fs.Int64("frames", 0, "total frames; default adds 10 seconds after --at")
	offsetMS := fs.Float64("offset-ms", 240, "persistent offset/drift endpoint in milliseconds")
	pattern := fs.String("pattern", sync.KindFrameSkip,
		"pattern: local_frame_skip, linear_drift, fixed_offset")
	traceOut := fs.String("trace-out", "", "write generated JSONL trace to this file")
	reportOut := fs.String("report-out", "", "write JSON report to this file")
	common := commonFlags{}
	bindCommonFlags(fs, &common)
	if err := fs.Parse(args); err != nil {
		return err
	}

	change := *changeFrame
	if change == 0 {
		change = int64(at.Seconds() * common.fps)
	}
	total := *totalFrames
	if total == 0 {
		total = change + int64(10*common.fps)
	}
	records := sync.DemoRecords(sync.DemoSpec{
		FPS:         common.fps,
		TotalFrames: total,
		ChangeFrame: change,
		Offset:      time.Duration(*offsetMS * float64(time.Millisecond)),
		Pattern:     *pattern,
	})
	if *traceOut != "" {
		if err := writeJSONL(*traceOut, records); err != nil {
			return err
		}
	}
	report, err := sync.Analyze(records, common.options())
	if err != nil {
		return err
	}
	if err := writePositions(common.positionsOut, records); err != nil {
		return err
	}
	if *reportOut != "" {
		if err := writeJSONFile(*reportOut, report); err != nil {
			return err
		}
	}
	return writeReport(os.Stdout, report, common.asJSON)
}

func readInput(path string) ([]byte, string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		return data, "stdin", err
	}
	data, err := os.ReadFile(path)
	return data, path, err
}

func parseTrace(data []byte, filename, format string) ([]sync.Record, error) {
	resolved := strings.ToLower(format)
	if resolved == "auto" {
		switch strings.ToLower(filepath.Ext(filename)) {
		case ".csv":
			resolved = "csv"
		default:
			resolved = "json"
		}
	}
	if resolved == "csv" {
		return sync.ParseCSV(strings.NewReader(string(data)))
	}
	return sync.ParseJSON(data)
}

func writePositions(path string, records []sync.Record) error {
	if path == "" {
		return nil
	}
	points, err := sync.Normalize(records)
	if err != nil {
		return err
	}
	tracks, err := sync.BuildTracks(points)
	if err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return sync.WritePositionCSV(file, sync.BuildPositionRows(tracks))
}

func writeJSONL(path string, records []sync.Record) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeReport(w io.Writer, report *sync.Report, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	text := renderText(report)
	_, err := io.WriteString(w, text)
	return err
}
