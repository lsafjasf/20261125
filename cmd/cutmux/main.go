package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cutmux/internal/audit"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cutmux:", err)
		os.Exit(2)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("cutmux", flag.ContinueOnError)
	a := fs.String("a", "", "container A (MP4/MOV or Matroska)")
	b := fs.String("b", "", "container B (MP4/MOV or Matroska)")
	labelA := fs.String("label-a", "A", "label for container A")
	labelB := fs.String("label-b", "B", "label for container B")
	framesA := fs.String("frames-a", "", "optional per-frame blackness CSV for A")
	framesB := fs.String("frames-b", "", "optional per-frame blackness CSV for B")
	outDir := fs.String("out", "audit-out", "output directory for report.json and report.md")
	nowText := fs.String("now", "", "fixed RFC3339 time for reproducible reports")
	tolerance := fs.Float64("tolerance-ms", 5, "timeline tolerance in milliseconds")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if *a == "" && len(rest) > 0 {
		*a = rest[0]
		rest = rest[1:]
	}
	if *b == "" && len(rest) > 0 {
		*b = rest[0]
		rest = rest[1:]
	}
	if *a == "" || *b == "" || len(rest) > 0 {
		return errors.New("usage: cutmux -a <file> -b <file> [-frames-a csv -frames-b csv] [-out dir]")
	}
	now := time.Now()
	if *nowText != "" {
		parsed, err := time.Parse(time.RFC3339, *nowText)
		if err != nil {
			return fmt.Errorf("invalid -now: %w", err)
		}
		now = parsed
	}
	rep, err := audit.Run(filepath.Clean(*a), filepath.Clean(*b), audit.Options{
		LabelA: *labelA, LabelB: *labelB, FramesA: *framesA, FramesB: *framesB,
		Now: now, ToleranceMS: *tolerance,
	})
	if err != nil {
		return err
	}
	if err := audit.WriteOutputs(rep, filepath.Clean(*outDir)); err != nil {
		return err
	}
	printReport(rep)
	if rep.Verdict.RealLoss || rep.Verdict.NeedsReview {
		os.Exit(1)
	}
	return nil
}

func printReport(rep *audit.Report) {
	fmt.Printf("cutmux %s  A=%s  B=%s\n", rep.Version, rep.LabelA, rep.LabelB)
	fmt.Printf("containers: %s / %s\n", rep.ContainerA.Format, rep.ContainerB.Format)
	fmt.Printf("video: fps A=%s B=%s; frames A=%d/%d B=%d/%d; class=%s\n",
		rep.Video.FPSA, rep.Video.FPSB,
		rep.Video.RawFramesA, rep.Video.PresentedA,
		rep.Video.RawFramesB, rep.Video.PresentedB,
		rep.Video.Classification)
	if len(rep.Video.OnlyA) > 0 || len(rep.Video.OnlyB) > 0 {
		fmt.Printf("video payload: common=%d onlyA=%d onlyB=%d\n",
			rep.Video.Identity.Common, rep.Video.Identity.OnlyA, rep.Video.Identity.OnlyB)
	}
	fmt.Printf("audio: packets A=%d B=%d; samples A=%d B=%d (%+d); presented B-A=%+.3f ms; head trim A=%.3f B=%.3f; class=%s\n",
		rep.Audio.PacketsA, rep.Audio.PacketsB,
		rep.Audio.SampleFramesA, rep.Audio.SampleFramesB, rep.Audio.SampleFrameDelta,
		rep.Audio.DurationDeltaMs,
		rep.Audio.HeadTrimA, rep.Audio.HeadTrimB, rep.Audio.Classification)
	fmt.Printf("subtitle: A=%.3f ms B=%.3f ms; class=%s\n",
		rep.Subtitle.Timeline.A.StartMs, rep.Subtitle.Timeline.B.StartMs,
		rep.Subtitle.Classification)
	fmt.Printf("duration: declared A=%.3f B=%.3f; presented end A=%.3f B=%.3f; A-B=%+.3f ms\n",
		rep.TotalDuration.A.DeclaredMs, rep.TotalDuration.B.DeclaredMs,
		rep.TotalDuration.A.MeasuredMs, rep.TotalDuration.B.MeasuredMs,
		rep.TotalDuration.Delta)
	fmt.Printf("black: A=%s B=%s; class=%s\n",
		rep.BlackFrames.A.Source, rep.BlackFrames.B.Source, rep.BlackFrames.Classification)
	fmt.Printf("\nverdict: %s\n%s\n\n", rep.Verdict.Status, rep.Verdict.Summary)
	for _, f := range rep.Findings {
		fmt.Printf("[%s] %s: %s\n", f.Severity, f.Code, f.Message)
	}
	fmt.Printf("\nnext: %s\n", rep.NextCheck)
	fmt.Println("outputs: report.json report.md")
}
