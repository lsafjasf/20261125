package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	outDir := flag.String("out", "artifacts/hlg-pipeline", "directory for SVG charts, CSV data and JSON report")
	peaksText := flag.String("peaks", "1000,2000,4000", "comma-separated display peak luminances in nits (three recommended)")
	gammaLawText := flag.String("gamma-law", string(GammaAuto), "gamma law: auto, standard, extended")
	manualGamma := flag.Float64("gamma", 0, "manual system gamma; 0 derives gamma from each peak")
	ootfText := flag.String("ootf", string(OOTFBT2100), "OOTF: bt2100, scene-power, linear-scene")
	blackNits := flag.Float64("black-nit", 0, "display black luminance in nits")
	referenceNits := flag.Float64("reference-nits", 1000, "common SDR preview white/reference luminance in nits")
	wrongAlpha := flag.Float64("wrong-alpha", 1000, "wrong example: OOTF alpha left at this nit value")
	wrongGamma := flag.Float64("wrong-gamma", 1.2, "wrong example: fixed system gamma")
	flag.Parse()

	peaks, err := ParsePeaks(*peaksText)
	if err != nil {
		log.Fatal(err)
	}

	gammaLaw := GammaLaw(*gammaLawText)
	ootfMode := OOTFMode(*ootfText)
	configs := make([]Config, 0, len(peaks))
	for _, peak := range peaks {
		cfg := DefaultConfig(peak)
		cfg.GammaLaw = gammaLaw
		cfg.ManualGamma = *manualGamma
		cfg.OOTF = ootfMode
		cfg.BlackNits = *blackNits
		cfg.ReferenceNits = *referenceNits
		if err := cfg.Validate(); err != nil {
			log.Fatal(err)
		}
		configs = append(configs, cfg)
	}

	wrongPeak := peaks[len(peaks)-1]
	wrong := DefaultConfig(wrongPeak)
	wrong.Label = "错误标定"
	if *wrongGamma > 0 {
		wrong.GammaLaw = GammaManual
		wrong.ManualGamma = *wrongGamma
	} else {
		wrong.GammaLaw = GammaAuto
	}
	wrong.AlphaNits = *wrongAlpha
	wrong.BlackNits = *blackNits
	wrong.ReferenceNits = *referenceNits
	wrong.OOTF = OOTFBT2100
	if err := wrong.Validate(); err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	files := struct {
		grayscale string
		color     string
		compare   string
		curveCSV  string
		highCSV   string
		report    string
	}{
		grayscale: filepath.Join(*outDir, "grayscale_curves.svg"),
		color:     filepath.Join(*outDir, "color_highlights.svg"),
		compare:   filepath.Join(*outDir, "peak_comparison.svg"),
		curveCSV:  filepath.Join(*outDir, "curve_data.csv"),
		highCSV:   filepath.Join(*outDir, "highlights.csv"),
		report:    filepath.Join(*outDir, "report.json"),
	}

	must(WriteGrayscaleChart(files.grayscale, configs, wrong))
	must(WriteColorChart(files.color, configs))
	must(WritePeakComparisonChart(files.compare, configs))
	must(WriteCurveCSV(files.curveCSV, configs, wrong))
	must(WriteHighlightsCSV(files.highCSV, configs))

	report := Report{
		Schema:           "hlg-pipeline/v1",
		Description:      "Adjustable HLG inverse OETF, system gamma, display peak and OOTF laboratory.",
		GammaFormula:     "standard: gamma=1.2+0.42*log10(Lw/1000); extended: gamma=1.2*1.111^log2(Lw/1000)",
		OOTFFormula:      "RD=alpha*Ys^(gamma-1)*Rs, likewise G/B; Ys=0.2627R+0.6780G+0.0593B",
		PeakConfigs:      configs,
		WrongCalibration: wrong,
		Checkpoints:      BuildCheckpoints(configs, wrong),
		OutputFiles: []string{
			files.grayscale, files.color, files.compare,
			files.curveCSV, files.highCSV, files.report,
		},
	}
	must(WriteReportJSON(files.report, report))

	fmt.Println("HLG pipeline outputs written to", *outDir)
	for _, cfg := range configs {
		p := cfg.DecodeGray(0.75)
		fmt.Printf("  %-8s Lw=%6.0f nit gamma=%.4f alpha=%6.0f nit | E'=0.75 -> %7.1f nit | E'=1 -> %7.1f nit\n",
			cfg.Label, cfg.PeakNits, cfg.Gamma(), cfg.Alpha(), p.RawLumaNits, cfg.DecodeGray(1).RawLumaNits)
	}
	bad75 := wrong.DecodeGray(0.75)
	fmt.Printf("  wrong    Lw=%6.0f nit gamma=%.4f alpha=%6.0f nit | E'=0.75 -> %7.1f nit | E'=1 -> %7.1f nit\n",
		wrong.PeakNits, wrong.Gamma(), wrong.Alpha(), bad75.RawLumaNits, wrong.DecodeGray(1).RawLumaNits)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
