package lutdiff

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// RegionVerdict is the machine-readable conclusion for one input region.
type RegionVerdict struct {
	Label        string  `json:"label"`
	Count        int     `json:"count"`
	MeanDeltaE   float64 `json:"mean_delta_e76"`
	MaxDeltaE    float64 `json:"max_delta_e76"`
	MaxCodeDelta float64 `json:"max_code_delta_8bit"`
	Commutable   bool    `json:"commutable"`
}

// Summary is the machine-readable top-level result.
type Summary struct {
	LUTGridSize        int             `json:"lut_grid_size"`
	SweepGridSize      int             `json:"sweep_grid_size"`
	GrayAxisMaxCode    float64         `json:"gray_axis_max_code_delta"`
	GlobalMaxDeltaE    float64         `json:"global_max_delta_e76"`
	GlobalMaxCodeDelta float64         `json:"global_max_code_delta_8bit"`
	Regions            []RegionVerdict `json:"regions"`
	Averaging          AveragingResult `json:"averaging"`
}

// AveragingResult documents why averaging cannot mask the disagreement.
type AveragingResult struct {
	ReferenceOrder       string  `json:"reference_order"`
	Identity             string  `json:"identity"`
	PointsOver1CodeVsAB  int     `json:"points_avg_error_over_1_code"`
	MaxAvgRefCodeError   float64 `json:"max_avg_reference_code_error"`
	AverageIsValidResult bool    `json:"average_is_valid_pipeline_result"`
}

// BuildSummary derives the machine-readable verdict. A region is flagged
// commutable when its worst sampled disagreement is at most 3 8-bit code
// values; the exact gray axis is reported separately via GrayAxisMaxCode.
func BuildSummary(d *ReportData) Summary {
	order := []struct {
		bit  Region
		name string
	}{
		{RegionNeutralBit, "neutral"},
		{RegionSaturatedBit, "high_saturation"},
		{RegionBoundaryBit, "near_boundary"},
	}
	regions := make([]RegionVerdict, 0, len(order))
	for _, item := range order {
		st := d.Cube.Regions[item.bit]
		regions = append(regions, RegionVerdict{
			Label:        item.name,
			Count:        st.Count,
			MeanDeltaE:   st.MeanDeltaE,
			MaxDeltaE:    st.MaxDeltaE,
			MaxCodeDelta: st.MaxCodeDelta,
			Commutable:   st.MaxCodeDelta <= 3.0,
		})
	}
	return Summary{
		LUTGridSize:        d.GridSize,
		SweepGridSize:      d.Cube.Grid,
		GrayAxisMaxCode:    d.Cube.GrayAxisMaxCode,
		GlobalMaxDeltaE:    d.Cube.All.MaxDeltaE,
		GlobalMaxCodeDelta: d.Cube.All.MaxCodeDelta,
		Regions:            regions,
		Averaging: AveragingResult{
			ReferenceOrder:       "A_then_B",
			Identity:             "|avg(AB,BA)-AB| = |AB-BA|/2",
			PointsOver1CodeVsAB:  d.Cube.All.AvgRefOver1,
			MaxAvgRefCodeError:   d.Cube.All.MaxAvgRefCode,
			AverageIsValidResult: false,
		},
	}
}

// WriteSummaryJSON writes summary.json.
func WriteSummaryJSON(dir string, s Summary) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "summary.json"), data, 0o644)
}
