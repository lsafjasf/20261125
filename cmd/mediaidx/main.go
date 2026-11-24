package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"mediaidx"
)

const usage = `usage:
  mediaidx append  -index PATH [-data PATH] -kind frame|sample [-start N|auto] [-count N] FILE...
  mediaidx verify  -index PATH [-data PATH] [-json]
  mediaidx inspect -index PATH [-data PATH] [-json]
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var code int
	switch os.Args[1] {
	case "append":
		code = runAppend(os.Args[2:])
	case "verify":
		code = runVerify(os.Args[2:])
	case "inspect":
		code = runInspect(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		code = 2
	}
	os.Exit(code)
}

func runAppend(args []string) int {
	flags := flag.NewFlagSet("append", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	indexPath := flags.String("index", "", "index file")
	dataPath := flags.String("data", "", "media data file (default INDEX.data)")
	kindName := flags.String("kind", "", "frame or sample")
	startText := flags.String("start", "auto", "first unit, or auto to resume")
	count := flags.Uint64("count", 1, "samples covered by each file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *indexPath == "" || *kindName == "" || flags.NArg() == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	kind, err := parseKind(*kindName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if kind == mediaidx.Frame && *count != 1 {
		fmt.Fprintln(os.Stderr, "frame count must be 1")
		return 2
	}
	if *count == 0 {
		fmt.Fprintln(os.Stderr, "count must be greater than zero")
		return 2
	}
	var start uint64
	explicitStart := *startText != "auto"
	if explicitStart {
		start, err = strconv.ParseUint(*startText, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid start: %v\n", err)
			return 2
		}
	}

	index, err := mediaidx.OpenWithOptions(*indexPath, mediaidx.Options{DataPath: *dataPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer index.Close()
	tx, err := index.Begin()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, name := range flags.Args() {
		data, err := readChunk(name)
		if err != nil {
			_ = tx.Abort()
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		switch {
		case kind == mediaidx.Frame && explicitStart:
			_, err = tx.AppendFrameAt(start, data)
			start++
		case kind == mediaidx.Frame:
			_, err = tx.AppendFrame(data)
		case explicitStart:
			_, err = tx.AppendSamplesAt(start, *count, data)
			start += *count
		default:
			_, err = tx.AppendSamples(*count, data)
		}
		if err != nil {
			_ = tx.Abort()
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	info, err := tx.Commit()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printJSON(info)
	return 0
}

func readChunk(name string) ([]byte, error) {
	if name == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return data, nil
}

func parseKind(value string) (mediaidx.Kind, error) {
	switch strings.ToLower(value) {
	case "frame", "video":
		return mediaidx.Frame, nil
	case "sample", "audio":
		return mediaidx.Sample, nil
	default:
		return 0, fmt.Errorf("unknown kind %q", value)
	}
}

func printJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func runVerify(args []string) int {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	indexPath := flags.String("index", "", "index file")
	dataPath := flags.String("data", "", "media data file (default INDEX.data)")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *indexPath == "" || flags.NArg() != 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	index, err := mediaidx.OpenWithOptions(*indexPath, mediaidx.Options{DataPath: *dataPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer index.Close()
	report, err := index.Verify()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *asJSON {
		printJSON(report)
	} else {
		fmt.Printf(
			"complete=%t commit=%d leaves=%d verified=%d data=%d/%d root=%s\n",
			report.Complete,
			report.Commit.Number,
			report.LeafCount,
			report.VerifiedLeaves,
			report.ActualData,
			report.CommittedData,
			report.Commit.Root,
		)
		for _, issue := range report.Issues {
			fmt.Printf(
				"issue=%s seq=%d locator=%s bytes=[%d:%d] available=%d missing_bytes=%d extra_bytes=%d\n",
				issue.Code,
				issue.LeafSeq,
				issue.Locator,
				issue.DataOffset,
				issue.DataOffset+issue.DataSize,
				issue.Available,
				issue.MissingBytes,
				issue.ExtraBytes,
			)
		}
	}
	if !report.Complete {
		return 1
	}
	return 0
}

func runInspect(args []string) int {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	indexPath := flags.String("index", "", "index file")
	dataPath := flags.String("data", "", "media data file (default INDEX.data)")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *indexPath == "" || flags.NArg() != 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	index, err := mediaidx.OpenWithOptions(*indexPath, mediaidx.Options{DataPath: *dataPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer index.Close()
	stats, err := index.Stats()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *asJSON {
		printJSON(stats)
	} else {
		fmt.Printf(
			"commit=%d leaves=%d data=%d root=%s gap=%t damaged=%t uncommitted=%t\n",
			stats.Commit.Number,
			stats.LeafCount,
			stats.DataSize,
			stats.Commit.Root,
			stats.HasGap,
			stats.Damaged,
			stats.Uncommitted,
		)
	}
	return 0
}
