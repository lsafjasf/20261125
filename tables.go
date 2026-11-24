package cutluma

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteSegmentCSV exports the per-segment parameter consistency table.
func WriteSegmentCSV(path string, rep *Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{
		"segment", "start", "end", "black_measured", "white_measured", "median_luma",
		"exposure_stops_vs_ref", "gamma_rel",
		"decl_black", "decl_white", "decl_exposure_ev", "decl_power",
		"delta_black", "delta_white", "delta_ev", "delta_power",
		"consistent", "inconsistent_fields",
	})
	for _, s := range rep.Segments {
		row := []string{
			itoa(s.Index), itoa(s.Start), itoa(s.End),
			f6(s.BlackPoint), f6(s.WhitePoint), f6(s.MedianLuma),
			f6(s.ExposureStopsVsRef), f6(s.GammaRel),
		}
		if s.Declared != nil {
			row = append(row, f6(s.Declared.Black), f6(s.Declared.White),
				f6(s.Declared.ExposureEV), f6(s.Declared.TransferPower))
		} else {
			row = append(row, "", "", "", "")
		}
		if s.DeltaVsTarget != nil {
			d := s.DeltaVsTarget
			row = append(row, f6(d.BlackDelta), f6(d.WhiteDelta),
				f6(d.ExposureEVDelta), f6(d.PowerDelta))
		} else {
			row = append(row, "", "", "", "")
		}
		row = append(row, boolCN(s.ParamsConsistent), strings.Join(s.InconsistentFields, "|"))
		_ = w.Write(row)
	}
	w.Flush()
	return w.Error()
}

// WriteCutCSV exports the per-cut forensic table.
func WriteCutCSV(path string, rep *Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{
		"cut", "frame", "left_seg", "right_seg", "kind", "confidence",
		"edge_jump", "fit_gap", "z_score", "outlier_z", "boundary_inc", "noise_se",
		"trend_change", "pre_cut_drift", "post_cut_drift", "natural_motion",
		"param_mismatch", "mismatch_score", "predicted_step", "prediction_ok",
		"source_available", "source_gap", "source_significant", "blend_frames", "reason_codes",
	})
	for _, c := range rep.Cuts {
		_ = w.Write([]string{
			itoa(c.CutIndex), itoa(c.Frame), itoa(c.LeftSegment), itoa(c.RightSegment),
			c.Kind, f6(c.Confidence),
			f6(c.EdgeJump), f6(c.FitGap), f6(c.ZScore), f6(c.OutlierZ), f6(c.BoundaryInc), f6(c.NoiseSE),
			f6(c.TrendChange), f6(c.PreCutDrift), f6(c.PostCutDrift), boolCN(c.NaturalMotion),
			boolCN(c.ParamMismatch), f6(c.MismatchScore), f6(c.PredictedStep), boolCN(c.PredictionOK),
			boolCN(c.Source.Available), f6(c.Source.FitGap), boolCN(c.Source.Significant),
			itoa(c.BlendFrames), strings.Join(c.ReasonCodes, "|"),
		})
	}
	w.Flush()
	return w.Error()
}

func f6(v float64) string  { return fmt.Sprintf("%.6f", v) }
func itoa(v int) string    { return fmt.Sprintf("%d", v) }
func boolCN(b bool) string { return fmt.Sprintf("%t", b) }
