package main

import (
	"flag"
	"fmt"
	"log"

	cutluma "cutluma"
)

func main() {
	inPath := flag.String("in", "", "path to a job JSON file; when empty the built-in deterministic ten-segment scenario is used")
	outDir := flag.String("out", "artifacts", "directory for JSON/CSV/SVG/Markdown outputs")
	lutSize := flag.Int("lut", 257, "repair LUT size")
	flag.Parse()

	var job *cutluma.Job
	var truth []cutluma.CutTruth

	if *inPath != "" {
		j, err := cutluma.LoadJobJSON(*inPath)
		if err != nil {
			log.Fatalf("load job: %v", err)
		}
		job = j
	} else {
		sc := cutluma.BuildScenario()
		job = sc.Job
		truth = sc.Truth
	}

	rep, err := cutluma.Analyze(job, cutluma.DefaultConfig(), nil)
	if err != nil {
		log.Fatalf("analyze: %v", err)
	}
	plan, err := cutluma.BuildRepairPlan(job, rep, *lutSize)
	if err != nil {
		log.Fatalf("repair: %v", err)
	}
	sum, err := cutluma.WriteArtifacts(*outDir, job, rep, plan, truth)
	if err != nil {
		log.Fatalf("write artifacts: %v", err)
	}

	fmt.Printf("cutluma: %d frames / %d segments / %d cuts, sigma=%.5f\n",
		sum.FrameCount, sum.SegmentCount, sum.CutCount, sum.SigmaLuma)
	for _, c := range rep.Cuts {
		fmt.Printf("  cut %d @%4d  %-20s gap=%+.4f z=%5.1f conf=%.2f\n",
			c.CutIndex, c.Frame, c.Kind, c.FitGap, c.ZScore, c.Confidence)
	}
	fmt.Printf("conversion max gap %.4f -> %.5f, mean reduction %.1f%%\n",
		sum.MaxConversionGapBefore, sum.MaxConversionGapAfter, sum.MeanConversionReductionPct)
	if truth != nil {
		fmt.Printf("synthetic truth matched: %v\n", sum.AllTruthMatched)
	}
	fmt.Printf("artifacts written to %s\n", *outDir)
}
