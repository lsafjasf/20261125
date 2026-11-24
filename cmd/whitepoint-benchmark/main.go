package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"aces-whitepoint/coloradapt"
)

type methodReport struct {
	Method           string                    `json:"method"`
	Summary          coloradapt.Summary        `json:"summary"`
	Groups           []coloradapt.GroupSummary `json:"groups"`
	Illuminants      []coloradapt.GroupSummary `json:"illuminants"`
	WorstSamples     []coloradapt.Result       `json:"worst_samples"`
	Results          []coloradapt.Result       `json:"results"`
	AdaptationMatrix coloradapt.Mat3           `json:"adaptation_matrix"`
	AdaptedWhite     coloradapt.Vec3           `json:"adapted_white"`
	NormalizedWhite  coloradapt.Vec3           `json:"normalized_white"`
}

func main() {
	asJSON := flag.Bool("json", false, "emit JSON instead of a text report")
	includeAllResults := flag.Bool("all-results", false, "include every sample in JSON output")
	flag.Parse()

	samples := coloradapt.BuildSamples()
	methods := []coloradapt.Method{
		coloradapt.MethodKeepXYZ,
		coloradapt.MethodBradford,
		coloradapt.MethodVonKries,
	}

	reports := make([]methodReport, 0, len(methods))
	for _, method := range methods {
		results := coloradapt.Evaluate(method, samples)
		groups := make([]coloradapt.GroupSummary, 0)
		for _, key := range coloradapt.GroupKeys(results) {
			groups = append(groups, coloradapt.SummarizeGroup(results, key))
		}
		illuminants := make([]coloradapt.GroupSummary, 0)
		for _, key := range coloradapt.IlluminantKeys(results) {
			illuminants = append(illuminants, coloradapt.SummarizeIlluminant(results, key))
		}

		adaptedWhite := coloradapt.D50XYZFromAP0(coloradapt.AP0SampleFromXYZ(coloradapt.ACESWhite), method)
		reports = append(reports, methodReport{
			Method:           string(method),
			Summary:          coloradapt.Summarize(results),
			Groups:           groups,
			Illuminants:      illuminants,
			WorstSamples:     worstSamples(results, 8),
			Results:          optionalResults(results, *includeAllResults),
			AdaptationMatrix: coloradapt.AdaptationMatrix(method, coloradapt.ACESWhite, coloradapt.D50White),
			AdaptedWhite:     adaptedWhite,
			NormalizedWhite:  coloradapt.NormalizeWhite(adaptedWhite),
		})
	}

	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(reports); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	printText(reports, len(samples))
}

func optionalResults(results []coloradapt.Result, include bool) []coloradapt.Result {
	if include {
		return results
	}
	return nil
}

func worstSamples(results []coloradapt.Result, count int) []coloradapt.Result {
	ordered := append([]coloradapt.Result(nil), results...)
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if ordered[j].PerceptualDeltaE > ordered[i].PerceptualDeltaE {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	if len(ordered) < count {
		count = len(ordered)
	}
	return ordered[:count]
}

func printText(reports []methodReport, sampleCount int) {
	fmt.Printf("ACES AP0 D60 -> internal D50 XYZ benchmark (%d samples)\n", sampleCount)
	fmt.Println("Perceptual ΔE2000 is relative to Bradford D50 output; round trip uses exact linear math without clipping.")
	fmt.Println()

	for _, report := range reports {
		summary := report.Summary
		fmt.Printf("method=%s\n", report.Method)
		fmt.Printf("  normalized white = (%.9f, %.9f, %.9f), white-point ΔE2000=%.6f\n",
			report.NormalizedWhite[0], report.NormalizedWhite[1], report.NormalizedWhite[2], summary.WhitePointDeltaE)
		fmt.Printf("  round trip: mean=%.3e max=%.3e ΔE2000, max |XYZ|=%.3e, max relative AP0=%.3e\n",
			summary.RoundTripMean, summary.RoundTripMax, summary.MaxXYZResidual, summary.MaxAP0Residual)
		fmt.Printf("  vs Bradford: mean=%.4f p95=%.4f max=%.4f ΔE2000; outside encoding cube after round trip=%d\n",
			summary.PerceptualMean, summary.PerceptualP95, summary.PerceptualMax, summary.OutsideUnitCubeCount)
		fmt.Printf("  neutral mean=%.4f, skin mean=%.4f, AP0 gamut boundary max=%.4f, encoding cube max=%.4f\n",
			summary.NeutralPerceptualMean, summary.SkinPerceptualMean, summary.BoundaryPerceptualMax, summary.CubePerceptualMax)
		fmt.Println("  by group:")
		for _, group := range report.Groups {
			fmt.Printf("    %-17s n=%3d mean=%8.4f p95=%8.4f max=%8.4f\n",
				group.Key, group.Count, group.Mean, group.P95, group.Max)
		}
		fmt.Println("  by illuminant:")
		for _, illuminant := range report.Illuminants {
			fmt.Printf("    %-17s n=%3d mean=%8.4f p95=%8.4f max=%8.4f\n",
				illuminant.Key, illuminant.Count, illuminant.Mean, illuminant.P95, illuminant.Max)
		}
		fmt.Println("  worst samples:")
		for _, result := range report.WorstSamples {
			fmt.Printf("    %8.4f  %s\n", result.PerceptualDeltaE, result.Sample.ID)
		}
		fmt.Println()
	}
}
