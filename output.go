package main

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

type Report struct {
	Schema           string       `json:"schema"`
	Description      string       `json:"description"`
	GammaFormula     string       `json:"gamma_formula"`
	OOTFFormula      string       `json:"ootf_formula"`
	PeakConfigs      []Config     `json:"peak_configs"`
	WrongCalibration Config       `json:"wrong_calibration_example"`
	Checkpoints      []Checkpoint `json:"checkpoints"`
	OutputFiles      []string     `json:"output_files"`
}

type Checkpoint struct {
	ConfigLabel string  `json:"config"`
	Code        float64 `json:"code"`
	Gamma       float64 `json:"gamma"`
	AlphaNits   float64 `json:"alpha_nits"`
	RawYLuma    float64 `json:"raw_y_luma_nits"`
	DisplayY    float64 `json:"display_y_nits"`
	OverPeak    bool    `json:"over_peak"`
}

func WriteCurveCSV(path string, configs []Config, wrong Config) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w := csv.NewWriter(file)
	defer w.Flush()

	header := []string{
		"config", "kind", "peak_nits", "alpha_nits", "gamma", "ootf", "code",
		"scene_r", "scene_g", "scene_b", "scene_luma",
		"raw_r_nit", "raw_g_nit", "raw_b_nit", "raw_y_nit",
		"display_y_nit", "over_peak",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, item := range configs {
		if err := writeCurveRows(w, item, "reference"); err != nil {
			return err
		}
	}
	if err := writeCurveRows(w, wrong, "wrong_calibration"); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func writeCurveRows(w *csv.Writer, cfg Config, kind string) error {
	for _, code := range GrayCodes(101) {
		s := cfg.DecodeGray(code)
		record := []string{
			cfg.Label, kind, f(cfg.PeakNits), f(cfg.Alpha()), f(cfg.Gamma()), string(cfg.OOTF), f(code),
			f(s.Scene.R), f(s.Scene.G), f(s.Scene.B), f(s.SceneLuma),
			f(s.RawNits.R), f(s.RawNits.G), f(s.RawNits.B), f(s.RawLumaNits),
			f(s.DisplayLuma), strconv.FormatBool(s.OverPeak),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return nil
}

func WriteHighlightsCSV(path string, configs []Config) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w := csv.NewWriter(file)
	defer w.Flush()

	header := []string{
		"level", "color", "config", "peak_nits", "gamma",
		"code_r", "code_g", "code_b",
		"raw_r_nit", "raw_g_nit", "raw_b_nit", "raw_y_nit",
		"display_r_nit", "display_g_nit", "display_b_nit", "display_y_nit",
		"over_peak", "channel_clipped",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, level := range HighlightLevels() {
		for _, patch := range HighlightPatches(level) {
			for _, cfg := range configs {
				s := cfg.Decode(patch.Code)
				record := []string{
					f(level), patch.Name, cfg.Label, f(cfg.PeakNits), f(cfg.Gamma()),
					f(patch.Code.R), f(patch.Code.G), f(patch.Code.B),
					f(s.RawNits.R), f(s.RawNits.G), f(s.RawNits.B), f(s.RawLumaNits),
					f(s.DisplayNits.R), f(s.DisplayNits.G), f(s.DisplayNits.B), f(s.DisplayLuma),
					strconv.FormatBool(s.OverPeak), strconv.FormatBool(s.ClippedChannel),
				}
				if err := w.Write(record); err != nil {
					return err
				}
			}
		}
	}
	w.Flush()
	return w.Error()
}

func WriteReportJSON(path string, report Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func BuildCheckpoints(configs []Config, wrong Config) []Checkpoint {
	out := make([]Checkpoint, 0, len(configs)*3+3)
	codes := []float64{0.5, 0.75, 1}
	for _, cfg := range configs {
		for _, code := range codes {
			s := cfg.DecodeGray(code)
			out = append(out, Checkpoint{
				ConfigLabel: cfg.Label, Code: code, Gamma: cfg.Gamma(),
				AlphaNits: cfg.Alpha(), RawYLuma: round(s.RawLumaNits, 3),
				DisplayY: round(s.DisplayLuma, 3), OverPeak: s.OverPeak,
			})
		}
	}
	for _, code := range codes {
		s := wrong.DecodeGray(code)
		out = append(out, Checkpoint{
			ConfigLabel: wrong.Label, Code: code, Gamma: wrong.Gamma(),
			AlphaNits: wrong.Alpha(), RawYLuma: round(s.RawLumaNits, 3),
			DisplayY: round(s.DisplayLuma, 3), OverPeak: s.OverPeak,
		})
	}
	return out
}

func f(v float64) string {
	return strconv.FormatFloat(round(v, 6), 'f', -1, 64)
}

func round(v float64, digits int) float64 {
	pow := math.Pow10(digits)
	return math.Round(v*pow) / pow
}
