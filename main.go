package main

import (
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("srt-ratconvert", flag.ContinueOnError)
	inputPath := flags.String("input", "-", "input SRT file, or - for stdin")
	outputPath := flags.String("output", "-", "output SRT file, or - for stdout")
	sourceRateText := flags.String("source-rate", "", "source frame rate, e.g. 24000/1001 or 23.976")
	rateText := flags.String("rates", "", "comma-separated target rates, e.g. 25,24000/1001")
	reportPath := flags.String("report", "", "write diagnostic report to this file instead of stderr")
	rounding := flags.String("rounding", "nearest", "millisecond quantization: nearest or floor")

	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *sourceRateText == "" || *rateText == "" {
		return fmt.Errorf("-source-rate and -rates are required")
	}

	sourceRate, err := parseRate(*sourceRateText)
	if err != nil {
		return err
	}
	rateValues, err := parseRateList(*rateText)
	if err != nil {
		return err
	}

	input, err := readInput(*inputPath)
	if err != nil {
		return err
	}
	cues, err := parseSRT(input)
	if err != nil {
		return err
	}

	converted, report, err := convertCues(cues, sourceRate, rateValues, *rounding)
	if err != nil {
		return err
	}
	rendered, err := formatSRT(converted)
	if err != nil {
		return err
	}
	if err := writeOutput(*outputPath, rendered); err != nil {
		return err
	}

	reportWriter, closeReport, err := openReport(*reportPath)
	if err != nil {
		return err
	}
	defer closeReport()
	if err := writeReport(reportWriter, report); err != nil {
		return err
	}
	if hasError(report.Issues) {
		return fmt.Errorf("validation found one or more errors")
	}
	return nil
}

func parseRateList(value string) ([]*big.Rat, error) {
	parts := splitCSV(value)
	rates := make([]*big.Rat, 0, len(parts))
	for i, part := range parts {
		rate, err := parseRate(part)
		if err != nil {
			return nil, fmt.Errorf("rate %d: %w", i+1, err)
		}
		rates = append(rates, rate)
	}
	if len(rates) == 0 {
		return nil, fmt.Errorf("at least one target rate is required")
	}
	return rates, nil
}

func splitCSV(value string) []string {
	var values []string
	start := 0
	for i, r := range value {
		if r == ',' {
			values = append(values, value[start:i])
			start = i + 1
		}
	}
	values = append(values, value[start:])

	filtered := values[:0]
	for _, item := range values {
		if trimmed := trimSpace(item); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}

func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func writeOutput(path, content string) error {
	if path == "-" {
		_, err := os.Stdout.WriteString(content)
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func openReport(path string) (*os.File, func() error, error) {
	if path == "" {
		return os.Stderr, func() error { return nil }, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}

func trimSpace(value string) string {
	start := 0
	end := len(value)
	for start < end && isSpace(value[start]) {
		start++
	}
	for end > start && isSpace(value[end-1]) {
		end--
	}
	return value[start:end]
}

func isSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}
