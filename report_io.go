package mixloud

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"strconv"
)

func WriteJSON(path string, report *Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func WriteWindowCSV(path string, report *Report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	header := []string{
		"scheme", "window_index", "start_sec", "end_sec", "loudness_lufs",
		"rms_dbfs", "peak_dbfs", "correlation", "mono_compatibility_db",
		"center_share", "dialogue_score", "dialogue", "silence",
		"phase_inverted", "clipped_samples", "near_clip_samples",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, sr := range report.Schemes {
		if err := writeSchemeWindowRows(w, sr); err != nil {
			return err
		}
	}
	for _, sr := range report.Stems {
		if err := writeSchemeWindowRows(w, sr); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func writeSchemeWindowRows(w *csv.Writer, sr SchemeReport) error {
	for _, win := range sr.Windows {
		row := []string{
			sr.Metrics.Scheme,
			strconv.Itoa(win.Index),
			f2(win.StartSec),
			f2(win.EndSec),
			ptrF(win.LoudnessLUFS),
			ptrF(win.RMSDBFS),
			ptrF(win.PeakDBFS),
			ptrF(win.Correlation),
			ptrF(win.MonoCompatibilityDB),
			ptrF(win.CenterShare),
			f3(win.DialogueScore),
			strconv.FormatBool(win.Dialogue),
			strconv.FormatBool(win.Silence),
			strconv.FormatBool(win.PhaseInverted),
			strconv.FormatInt(win.ClippedSamples, 10),
			strconv.FormatInt(win.NearClipSamples, 10),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func f2(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
func f3(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }

func ptrF(v *float64) string {
	if v == nil || !finite(*v) {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}
