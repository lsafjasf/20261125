package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rec2020-to-rec709-shoulder/internal/media"
	"rec2020-to-rec709-shoulder/internal/pipeline"
	"rec2020-to-rec709-shoulder/internal/report"
	"rec2020-to-rec709-shoulder/internal/synth"
)

var variantDirs = map[string]string{
	pipeline.VariantHard:     "hard_clip",
	pipeline.VariantFixed:    "fixed_shoulder",
	pipeline.VariantAdaptive: "adaptive_shoulder",
}

func runAll(args []string) error {
	o, fs, err := bindFlags("all", args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	_ = fs
	return execute(o, true)
}

func runProcess(args []string) error {
	o, fs, err := bindFlags("process", args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	_ = fs
	if strings.TrimSpace(o.Input) == "" {
		return fmt.Errorf("process requires -input FRAME_DIR")
	}
	return execute(o, false)
}

func runDemo(args []string) error {
	o, fs, err := bindFlags("demo", args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	_ = fs
	sourceDir := filepath.Join(o.Out, "source_rec2020_pfm")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return err
	}
	for frame := 0; frame < o.Frames; frame++ {
		im := synth.GenerateFrame(o.Width, o.Height, frame, o.Frames)
		path := filepath.Join(sourceDir, fmt.Sprintf("frame_%04d.pfm", frame))
		if err := media.WritePFM(path, im); err != nil {
			return err
		}
	}
	fmt.Printf("wrote %d Rec.2020 linear PFM frames to %s\n", o.Frames, sourceDir)
	return nil
}

type frameJob struct {
	index int
	path  string
}

func execute(o *options, generated bool) error {
	if o.Frames <= 0 {
		o.Frames = 12
	}
	if o.ContactFrame < 0 || o.ContactFrame >= o.Frames {
		o.ContactFrame = o.Frames - 1
	}
	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return err
	}

	jobs, err := buildJobs(o, generated)
	if err != nil {
		return err
	}
	stats, err := analyzeSequence(o, jobs, generated)
	if err != nil {
		return err
	}
	decisions := pipeline.BuildDecisions(stats, pipeline.AdaptiveOptions{Window: o.Window, MaxSlew: o.MaxSlew})
	if err := writeDecisions(o.Out, decisions); err != nil {
		return err
	}

	metrics, contact, err := renderSequence(o, jobs, generated, decisions)
	if err != nil {
		return err
	}
	rep := buildReport(o, generated, jobs, decisions, metrics, contact)
	if err := report.WriteFiles(o.Out, rep); err != nil {
		return err
	}
	if err := writeVisuals(o.Out, contact); err != nil {
		return err
	}
	printSummary(o.Out, rep.Summaries, decisions)
	return nil
}

func buildJobs(o *options, generated bool) ([]frameJob, error) {
	if generated {
		jobs := make([]frameJob, o.Frames)
		for i := range jobs {
			jobs[i] = frameJob{index: i}
		}
		return jobs, nil
	}
	paths, err := media.ListFrames(o.Input)
	if err != nil {
		return nil, err
	}
	jobs := make([]frameJob, len(paths))
	for i, path := range paths {
		jobs[i] = frameJob{index: i, path: path}
	}
	o.Frames = len(jobs)
	if o.ContactFrame >= o.Frames {
		o.ContactFrame = o.Frames - 1
	}
	return jobs, nil
}

func readJob(o *options, job frameJob, generated bool) (*pipeline.FloatImage, error) {
	var im *pipeline.FloatImage
	var err error
	if generated {
		im = synth.GenerateFrame(o.Width, o.Height, job.index, o.Frames)
	} else {
		im, err = media.ReadFrame(job.path, o.Transfer, o.SignalRange)
		if err != nil {
			return nil, err
		}
	}
	gain := o.Exposure
	if gain == 0 {
		if o.Transfer == "pq" {
			gain = 100
		} else {
			gain = 1
		}
	}
	if gain != 1 {
		for i := range im.Pix {
			im.Pix[i] *= gain
		}
	}
	return im, nil
}

func analyzeSequence(o *options, jobs []frameJob, generated bool) ([]pipeline.FrameStats, error) {
	stats := make([]pipeline.FrameStats, 0, len(jobs))
	for _, job := range jobs {
		src, err := readJob(o, job, generated)
		if err != nil {
			return nil, fmt.Errorf("analyze frame %d: %w", job.index, err)
		}
		stats = append(stats, pipeline.AnalyzeFrame(src, job.index))
	}
	return stats, nil
}

func writeDecisions(out string, decisions []pipeline.Decision) error {
	data, err := json.MarshalIndent(decisions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "adaptive_decisions.json"), data, 0o644)
}
