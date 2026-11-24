package main

import "testing"

func TestPreSwappedDisplayDimensionsWithMatrixIsDoubleAxisSwap(t *testing.T) {
	o := Observation{
		CodedWidth:               180,
		CodedHeight:              320,
		PixelWidth:               180,
		PixelHeight:              320,
		PixelOrient:              OrientationPortrait,
		HasTrackDisplay:          true,
		TrackDisplayWidth:        320,
		TrackDisplayHeight:       180,
		TrackDisplayOrient:       OrientationLandscape,
		DisplayClaimOrient:       OrientationLandscape,
		MatrixTurnsCW:            1,
		MatrixActive:             true,
		MatrixSwapsAxes:          true,
		MatrixOutputWidth:        320,
		MatrixOutputHeight:       180,
		MatrixOutputOrient:       OrientationLandscape,
		TrackMatchesCoded:        false,
		TrackMatchesMatrixOutput: true,
		TrackImpliedSAR:          256.0 / 81.0,
	}
	combo := displayDimensionsCombo(o)
	if combo.Status != "double_axis_swap_display_dimensions" || combo.Severity != "high" {
		t.Fatalf("status=%q severity=%q analysis=%s", combo.Status, combo.Severity, combo.Analysis)
	}
}

func TestCodedTrackDimensionsWithMatrixAreNormal(t *testing.T) {
	o := Observation{
		HasTrackDisplay:          true,
		MatrixSwapsAxes:          true,
		TrackMatchesCoded:        true,
		TrackMatchesMatrixOutput: false,
	}
	combo := displayDimensionsCombo(o)
	if combo.Status != "normal_pre_matrix_track_dimensions" || combo.Severity != "low" {
		t.Fatalf("status=%q severity=%q", combo.Status, combo.Severity)
	}
}
