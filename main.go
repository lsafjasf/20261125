package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "manifest", "hash":
		return runManifest(args[1:], stdout, stderr)
	case "diff", "compare":
		return runDiff(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	}

	if len(args) == 1 {
		return runManifest(args, stdout, stderr)
	}
	if len(args) == 2 && !looksLikeFlag(args[0]) && !looksLikeFlag(args[1]) {
		return runDiff(args, stdout, stderr)
	}

	fmt.Fprintf(stderr, "unknown command or invalid arguments: %v\n", args)
	printUsage(stderr)
	return 2
}

func looksLikeFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `usage:
  mediahash manifest [-json] [-block-size bytes] [-audio-window samples] FILE
  mediahash diff [-json] [-strict-container] [-block-size bytes] [-audio-window samples] ORIGINAL COPY
  mediahash verify [-json] MANIFEST.json FILE

aliases: hash=manifest, compare=diff`)
}

func optionsFromFlags(fs *flag.FlagSet) Options {
	blockSize := fs.Int("block-size", DefaultOptions().BlockSize, "fixed container block hash size in bytes")
	audioWindow := fs.Int("audio-window", DefaultOptions().AudioIntervalSamples, "audio samples/frames per interval hash")
	return Options{BlockSize: *blockSize, AudioIntervalSamples: *audioWindow}
}

func runManifest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("manifest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "write JSON manifest")
	outPath := fs.String("out", "", "write manifest JSON to file instead of stdout")
	opts := optionsFromFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "manifest requires exactly one FILE")
		return 2
	}

	archive, err := OpenArchive(fs.Arg(0), opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	defer archive.Close()

	if *outPath != "" {
		data, err := json.MarshalIndent(archive.Manifest, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		if err := os.WriteFile(*outPath, append(data, '\n'), 0o644); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	}
	if *asJSON {
		return writeJSON(stdout, archive.Manifest, stderr)
	}
	printManifestText(stdout, archive.Manifest)
	return 0
}

func runDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "write JSON report")
	strictContainer := fs.Bool("strict-container", false, "fail when container/box layout differs even if media hashes match")
	opts := optionsFromFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "diff requires ORIGINAL and COPY")
		return 2
	}

	report, err := CompareFiles(fs.Arg(0), fs.Arg(1), opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if *asJSON {
		if code := writeJSON(stdout, report, stderr); code != 0 {
			return code
		}
	} else {
		printReportText(stdout, report)
	}

	if !report.MediaIdentical {
		return 1
	}
	if *strictContainer && !report.ContainerIdentical {
		return 1
	}
	return 0
}

func runVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "write JSON result")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "verify requires MANIFEST.json and FILE")
		return 2
	}

	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	var expected Manifest
	if err := json.Unmarshal(data, &expected); err != nil {
		fmt.Fprintf(stderr, "error: invalid manifest: %v\n", err)
		return 2
	}
	actual, err := BuildManifestFromPath(fs.Arg(1), expected.Options)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	result := map[string]any{
		"schema":           SchemaVersion,
		"file":             fs.Arg(1),
		"manifest_sha256":  expected.SHA256,
		"actual_sha256":    actual.SHA256,
		"whole_file_match": expected.SHA256 == actual.SHA256,
		"sample_count":     len(actual.Samples),
		"interval_count":   len(actual.AudioIntervals),
	}
	if *asJSON {
		return writeJSON(stdout, result, stderr)
	}
	fmt.Fprintf(stdout, "whole-file match: %v\n", result["whole_file_match"])
	if result["whole_file_match"] != true {
		return 1
	}
	return 0
}

func BuildManifestFromPath(path string, opts Options) (*Manifest, error) {
	archive, err := OpenArchive(path, opts)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	return archive.Manifest, nil
}

func writeJSON(w io.Writer, value any, stderr io.Writer) int {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "error writing json: %v\n", err)
		return 2
	}
	return 0
}
