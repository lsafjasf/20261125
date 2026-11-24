package main

import "testing"

func comboByID(combos []ComboCheck, id string) ComboCheck {
	for _, combo := range combos {
		if combo.ID == id {
			return combo
		}
	}
	return ComboCheck{}
}

func intPtr(v int) *int { return &v }

func TestReportedLandscapeClaimPortraitPixelsSameDirectionDouble(t *testing.T) {
	o := Observation{
		DisplayClaimOrient: OrientationLandscape,
		PixelOrient:        OrientationPortrait,
		MatrixTurnsCW:      1,
		MatrixActive:       true,
		MatrixSwapsAxes:    true,
		ResidualTurnsCW:    intPtr(2),
		Pixel: PixelEvidence{
			CorrectionTurnsCW: intPtr(3),
			Source:            "reference-image-mae",
		},
	}
	combos := DiagnoseCombos(o)

	c1 := comboByID(combos, "C1-landscape-claim-portrait-pixels-quarter-matrix")
	if c1.Status != "confirmed_double_or_conflicting_rotation" || c1.Severity != "high" {
		t.Fatalf("C1 status=%q severity=%q", c1.Status, c1.Severity)
	}
	c2 := comboByID(combos, "C2-baked-pixels-plus-active-display-matrix")
	if c2.Status != "confirmed_same_direction_double_rotation" {
		t.Fatalf("C2 status=%q analysis=%s", c2.Status, c2.Analysis)
	}
	if len(HighRiskCombos(combos)) < 2 {
		t.Fatalf("high risk combos=%d, want at least 2", len(HighRiskCombos(combos)))
	}
}

func TestNormalMetadataRotationIsNotDouble(t *testing.T) {
	o := Observation{
		DisplayClaimOrient: OrientationPortrait,
		PixelOrient:        OrientationLandscape,
		MatrixTurnsCW:      1,
		MatrixActive:       true,
		MatrixSwapsAxes:    true,
		ResidualTurnsCW:    intPtr(0),
		Pixel:              PixelEvidence{CorrectionTurnsCW: intPtr(1), Source: "explicit-cli"},
	}
	combos := DiagnoseCombos(o)
	c2 := comboByID(combos, "C2-baked-pixels-plus-active-display-matrix")
	if c2.Status != "safe_single_matrix_rotation" || c2.Severity == "high" {
		t.Fatalf("normal metadata rotation misclassified: %+v", c2)
	}
}

func TestStaleMatrixAfterUprightBakeIsHighRisk(t *testing.T) {
	o := Observation{
		DisplayClaimOrient: OrientationLandscape,
		PixelOrient:        OrientationLandscape,
		MatrixTurnsCW:      1,
		MatrixActive:       true,
		MatrixSwapsAxes:    true,
		ResidualTurnsCW:    intPtr(3),
		Pixel:              PixelEvidence{CorrectionTurnsCW: intPtr(0), Source: "reference-image-mae"},
	}
	combos := DiagnoseCombos(o)
	c2 := comboByID(combos, "C2-baked-pixels-plus-active-display-matrix")
	if c2.Status != "confirmed_baked_pixels_plus_matrix" || c2.Severity != "high" {
		t.Fatalf("stale matrix status=%q severity=%q", c2.Status, c2.Severity)
	}
}

func TestUnknownSemanticsRequiresVisualCheck(t *testing.T) {
	o := Observation{
		DisplayClaimOrient: OrientationLandscape,
		PixelOrient:        OrientationLandscape,
		TrackDisplayOrient: OrientationLandscape,
		HasTrackDisplay:    true,
		MatrixTurnsCW:      1,
		MatrixActive:       true,
		MatrixSwapsAxes:    true,
	}
	combos := DiagnoseCombos(o)
	c2 := comboByID(combos, "C2-baked-pixels-plus-active-display-matrix")
	if c2.Status != "orientation_unverified" {
		t.Fatalf("status=%q, want orientation_unverified", c2.Status)
	}
}
