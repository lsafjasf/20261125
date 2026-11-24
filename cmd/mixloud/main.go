package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	mixloud "mixloud"
)

type commonFlags struct {
	input             string
	dialogue          string
	music             string
	sfx               string
	outDir            string
	hopMS             int
	silenceDB         float64
	dialogueThreshold float64
	adJumpLU          float64
	peakLimitDB       float64
	duckDB            float64
	writeWAV          bool
	jsonWindows       bool
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "demo":
			os.Exit(runDemo(os.Args[2:]))
		case "analyze":
			os.Exit(runAnalyze(os.Args[2:]))
		case "-h", "--help", "help":
			usage()
			return
		}
	}
	os.Exit(runAnalyze(os.Args[1:]))
}

func usage() {
	fmt.Fprint(os.Stderr, `mixloud — pure-Go 400 ms mix loudness checker

Usage:
  mixloud analyze [flags] <mix.wav>
  mixloud demo [--out-dir reports/demo] [--write-wav]

Examples:
  mixloud analyze final.wav --out-dir report
  mixloud analyze final.wav --dialogue d.wav --music m.wav --sfx fx.wav
  mixloud demo --write-wav

Flags:
`)
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	bindFlags(fs)
	fs.PrintDefaults()
}

func bindFlags(fs *flag.FlagSet) *commonFlags {
	c := &commonFlags{}
	fs.StringVar(&c.input, "input", "", "mixed WAV file")
	fs.StringVar(&c.dialogue, "dialogue", "", "optional dialogue stem WAV")
	fs.StringVar(&c.music, "music", "", "optional music stem WAV")
	fs.StringVar(&c.sfx, "sfx", "", "optional sound-effects stem WAV")
	fs.StringVar(&c.outDir, "out-dir", "mixloud-report", "directory for report.json/report.md/windows.csv")
	fs.IntVar(&c.hopMS, "hop-ms", 100, "window hop in milliseconds (400 ms window; 100 ms gives 75% overlap)")
	fs.Float64Var(&c.silenceDB, "silence-db", -70, "silence/near-silence RMS threshold in dBFS")
	fs.Float64Var(&c.dialogueThreshold, "dialogue-threshold", 0.55, "dialogue classification threshold")
	fs.Float64Var(&c.adJumpLU, "ad-jump-lu", 8, "loudness jump threshold for ad intrusion")
	fs.Float64Var(&c.peakLimitDB, "peak-limit-db", -1, "peak limiter ceiling in dBFS")
	fs.Float64Var(&c.duckDB, "dialogue-duck-db", -6, "music/SFX attenuation during dialogue")
	fs.BoolVar(&c.writeWAV, "write-wav", false, "write three processed 32-bit float WAV files")
	fs.BoolVar(&c.jsonWindows, "json-windows", true, "include every window in report.json")
	return c
}

func runAnalyze(args []string) int {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	cfg := bindFlags(fs)
	positionals, err := parseInterspersed(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if cfg.input == "" && len(positionals) > 0 {
		cfg.input = positionals[0]
	}
	if cfg.input == "" {
		fmt.Fprintln(os.Stderr, "error: missing input WAV; use `mixloud analyze <file.wav>` or `mixloud demo`")
		fs.Usage()
		return 2
	}

	mix, err := mixloud.ReadWAVFile(cfg.input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		return 1
	}
	stems, err := loadStems(cfg, mix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stems: %v\n", err)
		return 1
	}

	opts := optionsFromFlags(cfg)
	comparison, err := mixloud.Compare(mix, stems, cfg.input, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "analyze: %v\n", err)
		return 1
	}
	if err := writeOutputs(cfg.outDir, comparison, cfg.writeWAV); err != nil {
		fmt.Fprintf(os.Stderr, "write outputs: %v\n", err)
		return 1
	}
	printSummary(comparison, cfg.outDir)
	return 0
}

func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		positionals = append(positionals, rest[0])
		args = rest[1:]
	}
	return positionals, nil
}

func runDemo(args []string) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	outDir := fs.String("out-dir", "reports/demo", "output directory")
	writeWAV := fs.Bool("write-wav", false, "write processed WAV files")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	mix, stems := mixloud.DemoAudio()
	opts := mixloud.DefaultOptions()
	comparison, err := mixloud.Compare(mix, stems, "<built-in synthetic demo: dialogue+music+sfx+silence+ad+phase>", opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "demo analyze: %v\n", err)
		return 1
	}
	if err := writeOutputs(*outDir, comparison, *writeWAV); err != nil {
		fmt.Fprintf(os.Stderr, "write outputs: %v\n", err)
		return 1
	}
	printSummary(comparison, *outDir)
	return 0
}

func optionsFromFlags(c *commonFlags) mixloud.Options {
	o := mixloud.DefaultOptions()
	o.HopMS = c.hopMS
	o.SilenceDBFS = c.silenceDB
	o.DialogueThreshold = c.dialogueThreshold
	o.AdJumpLU = c.adJumpLU
	o.PeakLimitDBFS = c.peakLimitDB
	o.DialogueDuckDB = c.duckDB
	o.IncludeWindows = c.jsonWindows
	return o
}

func loadStems(c *commonFlags, ref *mixloud.Audio) (mixloud.StemSet, error) {
	var stems mixloud.StemSet
	load := func(path string) (*mixloud.Audio, error) {
		if path == "" {
			return nil, nil
		}
		a, err := mixloud.ReadWAVFile(path)
		if err != nil {
			return nil, err
		}
		if a.SampleRate != ref.SampleRate {
			return nil, fmt.Errorf("%s sample rate %d != mix %d", path, a.SampleRate, ref.SampleRate)
		}
		return fitToReference(a, ref.Frames(), ref.NumChannels()), nil
	}
	var err error
	if stems.Dialogue, err = load(c.dialogue); err != nil {
		return stems, err
	}
	if stems.Music, err = load(c.music); err != nil {
		return stems, err
	}
	stems.SFX, err = load(c.sfx)
	return stems, err
}

func fitToReference(a *mixloud.Audio, frames, channels int) *mixloud.Audio {
	out, _ := mixloud.NewAudio(a.SampleRate, channels, frames)
	for ch := 0; ch < channels; ch++ {
		sourceCh := ch
		if sourceCh >= a.NumChannels() {
			sourceCh = a.NumChannels() - 1
		}
		for i := 0; i < frames && i < a.Frames(); i++ {
			out.Channels[ch][i] = a.Channels[sourceCh][i]
		}
	}
	return out
}

func writeOutputs(dir string, c *mixloud.Comparison, writeWAV bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := mixloud.WriteJSON(filepath.Join(dir, "report.json"), &c.Report); err != nil {
		return err
	}
	if err := mixloud.WriteMarkdown(filepath.Join(dir, "report.md"), &c.Report); err != nil {
		return err
	}
	if err := mixloud.WriteWindowCSV(filepath.Join(dir, "windows.csv"), &c.Report); err != nil {
		return err
	}
	if writeWAV {
		for _, item := range c.Audio {
			path := filepath.Join(dir, item.ID+".wav")
			if err := mixloud.WriteWAVFile(path, item.Audio, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func printSummary(c *mixloud.Comparison, dir string) {
	fmt.Printf("报告已写入 %s/report.md、report.json、windows.csv\n", dir)
	for _, sr := range c.Report.Schemes {
		m := sr.Metrics
		integrated := "−∞"
		if m.IntegratedLUFS != nil {
			integrated = fmt.Sprintf("%.1f LUFS", *m.IntegratedLUFS)
		}
		peak := "−∞"
		if m.MaxPeakDBFS != nil {
			peak = fmt.Sprintf("%.2f dBFS", *m.MaxPeakDBFS)
		}
		fmt.Printf("%-18s I=%-10s max400=%6.1f LUFS peak=%s clip=%d dialogue=%.1f%% silence=%d ad=%d phase=%d burst=%d\n",
			m.Label, integrated, ptrOrNegInf(m.MaxWindowLUFS), peak, m.ClippedSamples,
			100*m.DialogueRatioTotal, len(sr.SilenceSegments), len(sr.AdIntrusions),
			len(sr.PhaseInversions), len(sr.BurstEvents))
	}
}

func ptrOrNegInf(p *float64) float64 {
	if p == nil {
		return -120
	}
	return *p
}
