package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vqaudit:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("vqaudit", flag.ContinueOnError)
	refDir := fs.String("ref", "", "reference frame directory")
	dirA := fs.String("a", "", "parameter set A coded frame directory")
	dirB := fs.String("b", "", "parameter set B coded frame directory")
	labelA := fs.String("label-a", "A", "label for set A")
	labelB := fs.String("label-b", "B", "label for set B")
	outDir := fs.String("out", "audit-out", "report output directory")
	topN := fs.Int("top", 10, "number of inspection tiles to rank")
	runTime := fs.String("now", "", "fixed UTC timestamp (RFC3339) for reproducible run ids")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *refDir == "" || *dirA == "" || *dirB == "" {
		return errors.New("usage: vqaudit -ref <dir> -a <dir> -b <dir> [-label-a ... -label-b ... -out ... -top ...]")
	}

	now := time.Now()
	if *runTime != "" {
		parsed, err := time.Parse(time.RFC3339, *runTime)
		if err != nil {
			return fmt.Errorf("invalid -now: %w", err)
		}
		now = parsed
	}

	cfg := buildConfig(filepath.Clean(*refDir),
		setConfig{Label: *labelA, Dir: filepath.Clean(*dirA)},
		setConfig{Label: *labelB, Dir: filepath.Clean(*dirB)}, *topN)

	refPaths, err := listFrames(cfg.ReferenceDir)
	if err != nil {
		return err
	}
	aPaths, err := listFrames(cfg.SetA.Dir)
	if err != nil {
		return err
	}
	bPaths, err := listFrames(cfg.SetB.Dir)
	if err != nil {
		return err
	}
	n := len(refPaths)
	if len(aPaths) != n || len(bPaths) != n {
		return fmt.Errorf("frame count mismatch: ref=%d A=%d B=%d (pairing uses natural-sorted filename order)",
			n, len(aPaths), len(bPaths))
	}
	if n == 0 {
		return errors.New("no supported frames (.png/.jpg) found")
	}

	frameOrder := make([]string, n)
	inputPaths := make([]string, 0, 3*n)
	perSet := map[string][]*frameStats{
		cfg.SetA.Label: make([]*frameStats, 0, n),
		cfg.SetB.Label: make([]*frameStats, 0, n),
	}
	comparisons := make([]frameComparison, 0, n)

	for i := 0; i < n; i++ {
		ref, err := loadFrame(refPaths[i])
		if err != nil {
			return err
		}
		frameOrder[i] = ref.Name
		inputPaths = append(inputPaths, refPaths[i])

		codedA, err := loadFrame(aPaths[i])
		if err != nil {
			return err
		}
		codedB, err := loadFrame(bPaths[i])
		if err != nil {
			return err
		}
		inputPaths = append(inputPaths, aPaths[i], bPaths[i])

		statA := analyzeFrame(ref, codedA, cfg.SetA.Label)
		statB := analyzeFrame(ref, codedB, cfg.SetB.Label)
		perSet[cfg.SetA.Label] = append(perSet[cfg.SetA.Label], statA)
		perSet[cfg.SetB.Label] = append(perSet[cfg.SetB.Label], statB)

		comparisons = append(comparisons, frameComparison{
			Index: i, Frame: ref.Name,
			PSNRA: statA.PSNRY, PSNRB: statB.PSNRY,
			PSNRDeltaAB: statB.PSNRY - statA.PSNRY,
			BandStepsA:  statA.BandSteps, BandStepsB: statB.BandSteps,
			BlockScoreA: statA.BlockScore, BlockScoreB: statB.BlockScore,
			RMSECbA: statA.RMSECb, RMSECrA: statA.RMSECr,
			RMSECbB: statB.RMSECb, RMSECrB: statB.RMSECr,
			TilesA: statA.TileCounts, TilesB: statB.TileCounts,
		})
	}

	runID := newRunID(now)
	rep := &report{
		RunID: runID, CreatedUTC: now.UTC().Format(time.RFC3339), Config: cfg,
		SummaryA:   buildSummary(cfg.SetA.Label, perSet[cfg.SetA.Label]),
		SummaryB:   buildSummary(cfg.SetB.Label, perSet[cfg.SetB.Label]),
		Frames:     comparisons,
		Inspection: rankInspection(perSet, cfg),
	}
	rep.NextCheck = buildNextCheck(rep.Inspection)

	man, err := buildManifest(cfg, runID, now, inputPaths, frameOrder)
	if err != nil {
		return err
	}
	man.Outputs = []string{"report.json", "inspection.csv", "manifest.json"}
	if err := writeOutputs(filepath.Clean(*outDir), rep, man); err != nil {
		return err
	}

	printConsole(rep)
	return nil
}

func printConsole(rep *report) {
	fmt.Printf("run: %s\n", rep.RunID)
	for _, s := range []setSummary{rep.SummaryA, rep.SummaryB} {
		fmt.Printf("[%s] frames=%d meanPSNRY=%.2f rmseCb=%.2f rmseCr=%.2f blockScore=%.2f bandSteps=%d tiles(coding=%d conversion=%d mixed=%d)\n",
			s.Label, s.Frames, s.MeanPSNRY, s.MeanRMSECb, s.MeanRMSECr,
			s.MeanBlockScore, s.TotalBandSteps,
			s.TilesCoding, s.TilesConversion, s.TilesMixed)
	}
	fmt.Println("frame, psnrA, psnrB, dPSNR(B-A), bandA, bandB, blockA, blockB, tilesA(cod/conv/mix), tilesB(cod/conv/mix)")
	for _, f := range rep.Frames {
		fmt.Printf("#%03d %s  %.2f  %.2f  %+.2f  %d  %d  %.2f  %.2f  %d/%d/%d  %d/%d/%d\n",
			f.Index, f.Frame, f.PSNRA, f.PSNRB, f.PSNRDeltaAB,
			f.BandStepsA, f.BandStepsB, f.BlockScoreA, f.BlockScoreB,
			f.TilesA[classCoding], f.TilesA[classConversion], f.TilesA[classMixed],
			f.TilesB[classCoding], f.TilesB[classConversion], f.TilesB[classMixed])
	}
	fmt.Println("\ninspection ranking:")
	for _, it := range rep.Inspection {
		fmt.Printf("#%d [%s] frame=%s tile(%d,%d) x[%d:%d]y[%d:%d] class=%s severity=%.3f block=%.2f band=%.4f chroma=%.2f\n",
			it.Rank, it.Set, it.Frame, it.TileCol, it.TileRow,
			it.X0, it.X1, it.Y0, it.Y1, it.Class, it.Severity,
			it.BlockScore, it.BandScore, it.ChromaScore)
	}
	fmt.Println("\n" + rep.NextCheck)
	fmt.Println("outputs: report.json inspection.csv manifest.json")
}
