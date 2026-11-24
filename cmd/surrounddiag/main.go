package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"surrounddiag/internal/analyze"
	"surrounddiag/internal/fixture"
	"surrounddiag/internal/preview"
	"surrounddiag/internal/report"
	"surrounddiag/internal/wav"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "analyze":
		err = analyzeCommand(os.Args[2:])
	case "audition":
		err = auditionCommand(os.Args[2:])
	case "delay":
		err = delayCommand(os.Args[2:])
	case "fixture":
		err = fixtureCommand(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func analyzeCommand(args []string) error {
	flags := flag.NewFlagSet("analyze", flag.ContinueOnError)
	filePath := flags.String("file", "", "5.1 export WAV")
	referencePath := flags.String("reference", "", "optional same-rate reference WAV")
	jsonPath := flags.String("json", "", "JSON report path")
	markdownPath := flags.String("markdown", "", "Markdown report path; '-' writes stdout")
	order := flags.String("order", "", "override channel order, e.g. FL,FR,FC,LFE,BL,BR")
	window := flags.Float64("window", 1.0, "analysis window seconds")
	hop := flags.Float64("hop", 1.0, "window hop seconds")
	maxDelay := flags.Float64("max-delay-ms", 20, "sample correlation search radius in milliseconds")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("--file is required")
	}
	export, err := readWithOrder(*filePath, *order)
	if err != nil {
		return err
	}
	config := analyze.Config{WindowSeconds: *window, HopSeconds: *hop, MaxDelayMS: *maxDelay}
	var result *analyze.Report
	if *referencePath != "" {
		reference, err := readWithOrder(*referencePath, "")
		if err != nil {
			return fmt.Errorf("reference: %w", err)
		}
		result, err = analyze.AnalyzeReference(export, reference, config)
	} else {
		result, err = analyze.Analyze(export, config)
	}
	if err != nil {
		return err
	}
	if *jsonPath != "" {
		if err := report.WriteJSON(*jsonPath, result); err != nil {
			return err
		}
	}
	if *markdownPath != "" {
		if err := report.WriteMarkdown(*markdownPath, result); err != nil {
			return err
		}
	} else if *jsonPath == "" {
		if err := report.WriteMarkdown("-", result); err != nil {
			return err
		}
	}
	return nil
}

func auditionCommand(args []string) error {
	flags := flag.NewFlagSet("audition", flag.ContinueOnError)
	filePath := flags.String("file", "", "5.1 export WAV")
	outDir := flags.String("outdir", "previews", "per-channel WAV output directory")
	order := flags.String("order", "", "override channel order")
	start := flags.Float64("start", 0, "range start seconds")
	end := flags.Float64("end", 0, "range end seconds; 0 means end of file")
	player := flags.String("play", "", "optional player executable; receives each mono WAV as argv[1]")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("--file is required")
	}
	file, err := readWithOrder(*filePath, *order)
	if err != nil {
		return err
	}
	manifest, err := preview.Export(file, *outDir, *start, *end)
	if err != nil {
		return err
	}
	for _, item := range manifest.Items {
		fmt.Printf("%-4s %8.1f dBFS  %s\n", item.Channel, item.RMSDB, item.File)
	}
	fmt.Println("manifest:", *outDir+"/manifest.json")
	return preview.Play(manifest, *player)
}

func delayCommand(args []string) error {
	flags := flag.NewFlagSet("delay", flag.ContinueOnError)
	filePath := flags.String("file", "", "5.1 export WAV")
	referencePath := flags.String("reference", "", "optional same-rate reference WAV")
	order := flags.String("order", "", "override channel order")
	maxDelay := flags.Float64("max-delay-ms", 20, "search radius in milliseconds")
	jsonPath := flags.String("json", "", "JSON output path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("--file is required")
	}
	file, err := readWithOrder(*filePath, *order)
	if err != nil {
		return err
	}
	config := analyze.Config{MaxDelayMS: *maxDelay}
	var result *analyze.Report
	if *referencePath != "" {
		reference, err := readWithOrder(*referencePath, "")
		if err != nil {
			return fmt.Errorf("reference: %w", err)
		}
		result, err = analyze.AnalyzeReference(file, reference, config)
	} else {
		result, err = analyze.Analyze(file, config)
	}
	if err != nil {
		return err
	}
	if *jsonPath != "" {
		payload := any(result.Pairs)
		if *referencePath != "" {
			payload = result.ChannelMap
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(*jsonPath, append(data, '\n'), 0o644)
	}
	if *referencePath != "" {
		fmt.Println("output/ref     status      lag(samples)   lag(ms)       r      env-r")
		for _, mapping := range result.ChannelMap {
			if mapping.ReferenceIndex < 0 {
				fmt.Printf("%-4s/%-4s %-10s %12s %10s %8s %8s\n",
					mapping.OutputChannel, "—", mapping.Status, "—", "—", "—", "—")
				continue
			}
			lagMS := float64(mapping.LagSamples) * 1000 / float64(result.SampleRate)
			fmt.Printf("%-4s/%-4s %-10s %+12d %+10.3f %+8.3f %+8.3f\n",
				mapping.OutputChannel, mapping.ReferenceChannel, mapping.Status,
				mapping.LagSamples, lagMS, mapping.Correlation, mapping.EnvelopeCorr)
		}
		return nil
	}
	fmt.Println("A/B                         lag(samples)   lag(ms)       r      env-r  mirror")
	for _, pair := range result.Pairs {
		fmt.Printf("%-4s/%-4s %+14d %+10.3f %+8.3f %+8.3f  %v\n",
			pair.AChannel, pair.BChannel, pair.MedianLag, pair.LagMS,
			pair.Correlation, pair.EnvelopeCorr, pair.Inverted)
	}
	return nil
}

func fixtureCommand(args []string) error {
	flags := flag.NewFlagSet("fixture", flag.ContinueOnError)
	outDir := flags.String("outdir", "testdata/demo", "fixture output directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	truth, err := fixture.Generate(*outDir)
	if err != nil {
		return err
	}
	fmt.Printf("master: %s\nexport: %s\ntruth:  %s/truth.json\n", truth.Source, truth.Export, *outDir)
	return nil
}

func readWithOrder(path, order string) (*wav.File, error) {
	file, err := wav.Read(path)
	if err != nil {
		return nil, err
	}
	if order != "" {
		layout, err := wav.ParseLayout(order, file.Channels)
		if err != nil {
			return nil, err
		}
		file.Layout = layout
		file.LayoutSource = "CLI override"
	}
	return file, nil
}

func usage() {
	fmt.Fprint(os.Stderr, `surrounddiag - 5.1 export phase/channel/LFE forensic tool

Commands:
  analyze   phase, mapping, center and low-frequency evidence report
  audition  extract each channel to a mono 24-bit WAV for listening
  delay     report sample-level inter-channel lags
  fixture   generate reproducible 5.1 source/export truth set

Examples:
  surrounddiag analyze --file export51.wav --json report.json --markdown report.md
  surrounddiag analyze --file export51.wav --reference master51.wav --markdown report.md
  surrounddiag audition --file export51.wav --outdir previews --play ffplay
`)
}
