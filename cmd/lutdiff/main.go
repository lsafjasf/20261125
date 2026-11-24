// Command lutdiff builds two 3D LUTs, applies them in both orders to a
// procedural test chart, and writes difference maps, sample CSV and a
// Chinese explanation report into the artifacts directory.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	lutdiff "lutdiff"
)

const (
	lutGridSize  = 33
	sweepGrid    = 41
	amplifyGain  = 5.0
	artifactsDir = "artifacts"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return err
	}

	pipeline := lutdiff.NewPipeline(lutGridSize)

	sampleResults := lutdiff.AnalyzeSamples(pipeline)
	if err := lutdiff.WriteSamplesCSV(artifactsDir, sampleResults); err != nil {
		return fmt.Errorf("write samples.csv: %w", err)
	}

	cube := lutdiff.SweepCube(pipeline, sweepGrid)

	src := lutdiff.SourceChart()
	renders := lutdiff.RenderOrders(pipeline, src)

	imageMaxDist, imageMaxCode := lutdiff.DiffStats(renders.AB, renders.BA)
	heatScale := cube.All.MaxDist
	if heatScale <= 0 {
		heatScale = imageMaxDist
	}

	files := map[string]func() error{
		"source_chart.png": func() error {
			return lutdiff.SavePNG(src.RGBA(), path("source_chart.png"))
		},
		"order_A_then_B.png": func() error {
			return lutdiff.SavePNG(renders.AB.RGBA(), path("order_A_then_B.png"))
		},
		"order_B_then_A.png": func() error {
			return lutdiff.SavePNG(renders.BA.RGBA(), path("order_B_then_A.png"))
		},
		"average_AB_BA.png": func() error {
			return lutdiff.SavePNG(renders.Avg.RGBA(), path("average_AB_BA.png"))
		},
		"diff_map.png": func() error {
			h := lutdiff.DiffHeatmap(renders.AB, renders.BA, heatScale)
			return lutdiff.SavePNG(lutdiff.HeatmapWithLegend(h, heatScale), path("diff_map.png"))
		},
		"diff.png": func() error {
			h := lutdiff.DiffHeatmap(renders.AB, renders.BA, heatScale)
			return lutdiff.SavePNG(lutdiff.HeatmapWithLegend(h, heatScale), path("diff.png"))
		},
		fmt.Sprintf("diff_amplified_x%.0f.png", amplifyGain): func() error {
			g := lutdiff.AmplifiedDiff(renders.AB, renders.BA, amplifyGain)
			return lutdiff.SavePNG(g, path(fmt.Sprintf("diff_amplified_x%.0f.png", amplifyGain)))
		},
		"average_error_vs_AB.png": func() error {
			h := lutdiff.AverageReferenceHeatmap(renders, heatScale)
			return lutdiff.SavePNG(lutdiff.HeatmapWithLegend(h, heatScale), path("average_error_vs_AB.png"))
		},
	}
	for name, write := range files {
		if err := write(); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		fmt.Println("wrote", path(name))
	}

	data := &lutdiff.ReportData{
		GridSize: lutGridSize,
		Samples:  sampleResults,
		Cube:     cube,
		Art: lutdiff.ArtifactStats{
			HeatScale:    heatScale,
			Gain:         amplifyGain,
			ImageMaxDist: imageMaxDist,
			ImageMaxCode: imageMaxCode,
		},
	}
	if err := lutdiff.WriteReport(artifactsDir, data); err != nil {
		return fmt.Errorf("write REPORT.md: %w", err)
	}
	fmt.Println("wrote", path("REPORT.md"))

	summary := lutdiff.BuildSummary(data)
	if err := lutdiff.WriteSummaryJSON(artifactsDir, summary); err != nil {
		return fmt.Errorf("write summary.json: %w", err)
	}
	fmt.Println("wrote", path("summary.json"))

	fmt.Printf("cube max DeltaE=%.2f maxCode=%.1f; chart maxCode=%.1f; avg-ref points >1 code: %d/%d\n",
		cube.All.MaxDeltaE, cube.All.MaxCodeDelta, imageMaxCode,
		cube.All.AvgRefOver1, cube.All.Count)
	return nil
}

func path(name string) string {
	return filepath.Join(artifactsDir, name)
}
