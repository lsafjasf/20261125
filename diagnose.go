package main

import "fmt"

type PixelEvidence struct {
	CorrectionTurnsCW *int      `json:"correctionTurnsClockwise,omitempty"`
	CorrectionDegCW   *int      `json:"correctionDegreesClockwise,omitempty"`
	Source            string    `json:"source"`
	ReferencePath     string    `json:"referencePath,omitempty"`
	CandidateMAE      []float64 `json:"candidateMAE,omitempty"`
	Note              string    `json:"note,omitempty"`
}

type Observation struct {
	InputPath string `json:"inputPath"`

	StreamIndex int    `json:"streamIndex"`
	Codec       string `json:"codec"`

	CodedWidth  int `json:"codedWidth"`
	CodedHeight int `json:"codedHeight"`

	PixelWidth  int         `json:"pixelWidth"`
	PixelHeight int         `json:"pixelHeight"`
	PixelOrient Orientation `json:"pixelOrientation"`

	SAR       string      `json:"sampleAspectRatio,omitempty"`
	SARNum    int         `json:"sarNumerator,omitempty"`
	SARDen    int         `json:"sarDenominator,omitempty"`
	DAR       string      `json:"displayAspectRatio,omitempty"`
	DAROrient Orientation `json:"darOrientation,omitempty"`

	TrackDisplayWidth  float64     `json:"trackDisplayWidth,omitempty"`
	TrackDisplayHeight float64     `json:"trackDisplayHeight,omitempty"`
	TrackDisplayOrient Orientation `json:"trackDisplayOrientation,omitempty"`
	HasTrackDisplay    bool        `json:"hasTrackDisplay"`
	TrackImpliedSAR    float64     `json:"trackImpliedSar,omitempty"`
	DisplayClaimWidth  float64     `json:"displayClaimWidth,omitempty"`
	DisplayClaimHeight float64     `json:"displayClaimHeight,omitempty"`
	DisplayClaimOrient Orientation `json:"displayClaimOrientation"`
	DisplayClaimSource string      `json:"displayClaimSource"`

	MP4Tracks       []TrackDisplayInfo `json:"mp4Tracks,omitempty"`
	MP4Matrix       *MatrixInfo        `json:"mp4Matrix,omitempty"`
	FFProbeMatrix   *MatrixInfo        `json:"ffprobeMatrix,omitempty"`
	LegacyRotateTag *int               `json:"legacyRotateTagDegrees,omitempty"`

	MatrixTurnsCW   int    `json:"matrixTurnsClockwise"`
	MatrixDegCW     int    `json:"matrixDegreesClockwise"`
	MatrixSwapsAxes bool   `json:"matrixSwapsAxes"`
	MatrixActive    bool   `json:"matrixActive"`
	MatrixSource    string `json:"matrixSource,omitempty"`

	MatrixOutputWidth  int         `json:"matrixOutputWidth"`
	MatrixOutputHeight int         `json:"matrixOutputHeight"`
	MatrixOutputOrient Orientation `json:"matrixOutputOrientation"`

	TrackMatchesCoded        bool `json:"trackDisplayMatchesCoded"`
	TrackMatchesMatrixOutput bool `json:"trackDisplayMatchesMatrixOutput"`

	Pixel PixelEvidence `json:"pixelEvidence"`

	ResidualTurnsCW *int `json:"compliantPlayerResidualTurnsCW,omitempty"`
}

type ComboCheck struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Scope       string         `json:"scope"`
	Triggered   bool           `json:"triggered"`
	Status      string         `json:"status"`
	Severity    string         `json:"severity"`
	Conditions  map[string]any `json:"conditions"`
	Analysis    string         `json:"analysis"`
	Consequence string         `json:"consequence,omitempty"`
	Fix         string         `json:"fix"`
}

func buildObservation(input string, probed *ProbedVideo, tracks []TrackDisplayInfo, rawWidth, rawHeight int, evidence PixelEvidence) Observation {
	o := Observation{
		InputPath:       input,
		StreamIndex:     probed.StreamIndex,
		Codec:           probed.CodecName,
		CodedWidth:      probed.Width,
		CodedHeight:     probed.Height,
		PixelWidth:      rawWidth,
		PixelHeight:     rawHeight,
		PixelOrient:     orientationOf(rawWidth, rawHeight),
		SAR:             probed.SARString,
		DAR:             probed.DARString,
		MP4Tracks:       tracks,
		Pixel:           evidence,
		LegacyRotateTag: probed.LegacyRotateTag,
	}
	if o.PixelWidth == 0 {
		o.PixelWidth = probed.Width
		o.PixelHeight = probed.Height
		o.PixelOrient = orientationOf(o.PixelWidth, o.PixelHeight)
	}
	if n, d, ok := parseRatio(probed.SARString); ok {
		o.SARNum, o.SARDen = n, d
	}
	if n, d, ok := parseRatio(probed.DARString); ok {
		o.DAROrient = orientationOf(n, d)
	}

	var videoTrack *TrackDisplayInfo
	for i := range tracks {
		if tracks[i].HandlerType == "vide" || videoTrack == nil {
			videoTrack = &tracks[i]
			if tracks[i].HandlerType == "vide" {
				break
			}
		}
	}
	if videoTrack != nil && videoTrack.DisplayWidth > 0 && videoTrack.DisplayHeight > 0 {
		o.HasTrackDisplay = true
		o.TrackDisplayWidth = videoTrack.DisplayWidth
		o.TrackDisplayHeight = videoTrack.DisplayHeight
		o.TrackDisplayOrient = orientationOfF(videoTrack.DisplayWidth, videoTrack.DisplayHeight)
		matrix := matrixFromRaw(videoTrack.Matrix, "mp4-tkhd")
		o.MP4Matrix = &matrix
		o.DisplayClaimWidth = videoTrack.DisplayWidth
		o.DisplayClaimHeight = videoTrack.DisplayHeight
		o.DisplayClaimOrient = o.TrackDisplayOrient
		o.DisplayClaimSource = "mp4-tkhd-pre-matrix-width-height"
	} else if o.DAROrient != "" {
		o.DisplayClaimOrient = o.DAROrient
		o.DisplayClaimSource = "ffprobe-display-aspect-ratio"
	} else {
		o.DisplayClaimOrient = o.PixelOrient
		o.DisplayClaimSource = "unavailable-assumed-pixel-grid"
	}

	o.FFProbeMatrix = probed.FFProbeMatrix

	turns := 0
	source := "identity-default"
	nonIdentityMatrix := false
	if o.MP4Matrix != nil && (!o.MP4Matrix.LinearIdentity || o.MP4Matrix.Reflection) {
		turns = o.MP4Matrix.TurnsClockwise
		source = "mp4-tkhd-matrix"
		nonIdentityMatrix = true
	} else if probed.FFProbeMatrix != nil && (!probed.FFProbeMatrix.LinearIdentity || probed.FFProbeMatrix.Reflection) {
		turns = probed.FFProbeMatrix.TurnsClockwise
		source = "ffprobe-display-matrix"
		nonIdentityMatrix = true
	} else if probed.FFProbeRotation != nil {
		turns = turnsFromFFprobeRotation(*probed.FFProbeRotation)
		source = "ffprobe-rotation"
		nonIdentityMatrix = turns != 0
	} else if probed.LegacyRotateTag != nil {
		turns = turnsFromLegacyRotateTag(*probed.LegacyRotateTag)
		source = "legacy-rotate-tag"
		nonIdentityMatrix = turns != 0
	}
	o.MatrixTurnsCW = turns
	o.MatrixDegCW = turns * 90
	o.MatrixSwapsAxes = turns == 1 || turns == 3
	o.MatrixActive = nonIdentityMatrix
	o.MatrixSource = source
	if o.MP4Matrix != nil {
		o.MP4Matrix.InterpretationNote = "active interpretation source priority: " + source
	}

	o.MatrixOutputWidth, o.MatrixOutputHeight = rotateDims(o.PixelWidth, o.PixelHeight, turns)
	o.MatrixOutputOrient = orientationOf(o.MatrixOutputWidth, o.MatrixOutputHeight)
	if o.HasTrackDisplay {
		tol := 1.5
		o.TrackMatchesCoded = approxEqualIntFloat(o.CodedWidth, o.TrackDisplayWidth, tol) &&
			approxEqualIntFloat(o.CodedHeight, o.TrackDisplayHeight, tol)
		o.TrackMatchesMatrixOutput = approxEqualIntFloat(o.MatrixOutputWidth, o.TrackDisplayWidth, tol) &&
			approxEqualIntFloat(o.MatrixOutputHeight, o.TrackDisplayHeight, tol)
		if o.CodedWidth > 0 && o.CodedHeight > 0 && o.TrackDisplayHeight > 0 {
			o.TrackImpliedSAR = (o.TrackDisplayWidth / o.TrackDisplayHeight) /
				(float64(o.CodedWidth) / float64(o.CodedHeight))
		}
	}

	if o.Pixel.CorrectionTurnsCW != nil {
		c := normTurnsCW(*o.Pixel.CorrectionTurnsCW)
		o.Pixel.CorrectionTurnsCW = &c
		deg := c * 90
		o.Pixel.CorrectionDegCW = &deg
		residual := normTurnsCW(c - o.MatrixTurnsCW)
		o.ResidualTurnsCW = &residual
	}
	return o
}

func DiagnoseCombos(o Observation) []ComboCheck {
	combos := []ComboCheck{
		landscapeSignatureCombo(o),
		bakedMatrixCombo(o),
		autorotateManualCombo(o),
		duplicateMetadataCombo(o),
		displayDimensionsCombo(o),
	}
	return combos
}

func landscapeSignatureCombo(o Observation) ComboCheck {
	landscapeClaim := o.MatrixOutputOrient == OrientationLandscape ||
		o.TrackDisplayOrient == OrientationLandscape ||
		o.DisplayClaimOrient == OrientationLandscape
	triggered := landscapeClaim && o.PixelOrient == OrientationPortrait && o.MatrixSwapsAxes
	status := "absent"
	severity := "info"
	analysis := "Not the reported landscape-claim/portrait-pixel/quarter-matrix signature."
	if triggered {
		status = "needs_visual_check"
		severity = "medium"
		analysis = "Claimed display is landscape, raw no-autorotate pixels are portrait, and a 90/270 matrix swaps axes. Dimensions are consistent when the matrix is the only rotation. This signature alone does not prove baked pixels or double rotation."
		if o.ResidualTurnsCW != nil {
			if *o.ResidualTurnsCW == 0 {
				status = "consistent_single_rotation"
				severity = "low"
				analysis = "Reference/explicit orientation confirms the matrix is exactly the one correction needed; players honoring it are correct. Players ignoring the matrix show sideways pixels."
			} else {
				status = "confirmed_double_or_conflicting_rotation"
				severity = "high"
				analysis = fmt.Sprintf("Raw pixels need %d clockwise quarter-turn(s) to become upright, but the compliant matrix applies %d. The matrix-respecting image still needs %d quarter-turn(s); a bake plus matrix or manual filter is rotating the picture twice/in the wrong direction.",
					*o.Pixel.CorrectionTurnsCW, o.MatrixTurnsCW, *o.ResidualTurnsCW)
			}
		}
	}
	return ComboCheck{
		ID:        "C1-landscape-claim-portrait-pixels-quarter-matrix",
		Title:     "横屏声明 × 竖屏真实像素 × 90/270 矩阵",
		Scope:     "file",
		Triggered: triggered, Status: status, Severity: severity,
		Conditions: map[string]any{
			"displayClaimOrientation": o.DisplayClaimOrient,
			"pixelOrientation":        o.PixelOrient,
			"matrixTurnsClockwise":    o.MatrixTurnsCW,
			"matrixSwapsAxes":         o.MatrixSwapsAxes,
		},
		Analysis: analysis,
		Fix:      "Extract with -noautorotate, compare against an upright reference, then choose exactly one transform: keep the matrix or bake the correct rotation and strip the matrix.",
	}
}

func bakedMatrixCombo(o Observation) ComboCheck {
	known := o.Pixel.CorrectionTurnsCW != nil
	confirmed := known && o.MatrixActive && *o.Pixel.CorrectionTurnsCW != o.MatrixTurnsCW
	uprightBake := known && *o.Pixel.CorrectionTurnsCW == 0 && o.MatrixTurnsCW != 0
	needsVisual := !known && o.MatrixActive

	status := "absent"
	severity := "info"
	triggered := confirmed || needsVisual
	if confirmed {
		status = "confirmed_baked_pixels_plus_matrix"
		severity = "high"
	} else if needsVisual {
		status = "orientation_unverified"
		severity = "medium"
	} else if known && o.MatrixActive && *o.Pixel.CorrectionTurnsCW == o.MatrixTurnsCW {
		triggered = true
		status = "safe_single_matrix_rotation"
		severity = "low"
	}

	bakedTurns := 0
	sameDirectionDouble := false
	if known {
		bakedTurns = normTurnsCW(-*o.Pixel.CorrectionTurnsCW)
		sameDirectionDouble = o.MatrixTurnsCW != 0 && bakedTurns == o.MatrixTurnsCW
		if sameDirectionDouble {
			status = "confirmed_same_direction_double_rotation"
			severity = "high"
			confirmed = true
			triggered = true
		}
	}

	analysis := "No active matrix/baked-pixel conflict detected."
	consequence := ""
	if sameDirectionDouble {
		analysis = fmt.Sprintf("Pixels were already rotated %d quarter-turn(s) clockwise before encoding, and the display matrix requests the same %d turn(s). A matrix-respecting player applies both, totaling %d quarter-turn(s).",
			bakedTurns, o.MatrixTurnsCW, normTurnsCW(bakedTurns+o.MatrixTurnsCW))
		consequence = "For 90/270-degree cases the image is usually upside down (180 degrees total); players that ignore the matrix show the sideways baked frame instead."
	} else if uprightBake {
		analysis = "Raw pixels are already upright, but a non-identity matrix remains. Every compliant player rotates the already-baked image a second time."
		consequence = "Matrix-aware players show a sideways/upside-down image; matrix-ignoring players may look correct."
	} else if confirmed {
		analysis = "The semantic orientation measured from the raw pixels does not match the active display matrix; this is a bake/metadata rotation conflict."
	} else if needsVisual {
		analysis = "Geometry alone cannot distinguish the safe metadata-only rotation from already-baked pixels with a stale/second matrix; inspect 01/02 frames or supply -reference/-pixel-correction-cw."
	} else if status == "safe_single_matrix_rotation" {
		analysis = "The required raw-pixel correction equals the matrix turn; there is one metadata rotation and no second bake."
	}

	return ComboCheck{
		ID:        "C2-baked-pixels-plus-active-display-matrix",
		Title:     "已烘入像素的旋转 × 仍生效的显示矩阵",
		Scope:     "file",
		Triggered: triggered, Status: status, Severity: severity,
		Conditions: map[string]any{
			"matrixActive":               o.MatrixActive,
			"matrixTurnsClockwise":       o.MatrixTurnsCW,
			"pixelCorrectionKnown":       known,
			"pixelCorrectionTurnsCW":     o.Pixel.CorrectionTurnsCW,
			"inferredBakedTurnsCW":       bakedTurns,
			"sameDirectionDoubleTurn":    sameDirectionDouble,
			"displayMatchesPixelGrid":    o.HasTrackDisplay && o.TrackDisplayOrient == o.PixelOrient,
			"displayMatchesMatrixOutput": o.TrackMatchesMatrixOutput,
		},
		Analysis: analysis, Consequence: consequence,
		Fix: "If pixels are already correct, strip/identity the matrix. If pixels are baked sideways, either replace the matrix with the one true correction or bake that correction and write an identity matrix; never do both.",
	}
}

func autorotateManualCombo(o Observation) ComboCheck {
	triggered := o.MatrixActive
	status := "absent"
	severity := "info"
	analysis := "No non-identity matrix, so autorotate plus a manual rotation cannot double-apply matrix orientation."
	if triggered {
		status = "demonstrated_by_double_rotation_png"
		severity = "high"
		analysis = "FFmpeg-based tools and many players apply display-matrix autorotation by default. Running transpose/rotate again on that already-normalized frame applies the matrix a second time. The generated 03-double-rotated PNG reproduces this exact pipeline mistake."
	}
	return ComboCheck{
		ID:        "C3-default-autorotate-plus-manual-transpose",
		Title:     "默认自动旋转 × 再手动 transpose/rotate",
		Scope:     "pipeline",
		Triggered: triggered, Status: status, Severity: severity,
		Conditions: map[string]any{
			"matrixActive":              o.MatrixActive,
			"ffmpegAutorotateDefault":   true,
			"secondRotationFilter":      true,
			"doubleFrameArtifactPrefix": "03-double-rotated",
		},
		Analysis:    analysis,
		Consequence: "Two successive matrix-equivalent turns; for a 90-degree metadata value this commonly produces a 180-degree wrong frame.",
		Fix:         "Use either default autorotate with no rotation filter, or -noautorotate followed by exactly one explicitly chosen correction.",
	}
}

func duplicateMetadataCombo(o Observation) ComboCheck {
	tagTurns := -1
	hasTag := false
	if tag := o.LegacyRotateTag; tag != nil {
		tagTurns = turnsFromLegacyRotateTag(*tag)
		hasTag = true
	} else if o.FFProbeMatrix != nil && o.FFProbeMatrix.LegacyTagRotation != nil {
		tagTurns = turnsFromLegacyRotateTag(*o.FFProbeMatrix.LegacyTagRotation)
		hasTag = true
	}
	hasMatrix := o.MatrixSource == "mp4-tkhd-matrix" ||
		o.MatrixSource == "ffprobe-display-matrix" ||
		o.MatrixSource == "ffprobe-rotation"
	equivalent := hasTag && hasMatrix && tagTurns == o.MatrixTurnsCW
	conflict := hasTag && hasMatrix && tagTurns != o.MatrixTurnsCW

	status := "absent"
	severity := "info"
	triggered := hasTag
	analysis := "No legacy rotate tag found."
	if equivalent {
		status = "duplicate_same_orientation"
		severity = "medium"
		analysis = "A legacy rotate tag and display matrix describe the same orientation. FFmpeg normally merges them; a custom player that converts the tag to a filter and also applies the matrix will rotate twice."
	} else if conflict {
		status = "metadata_conflict"
		severity = "high"
		analysis = "The legacy rotate tag and active matrix imply different quarter-turns. Different player precedence rules can produce inconsistent orientations."
	} else if hasTag && !hasMatrix {
		status = "legacy_tag_only"
		severity = "low"
		analysis = "Only a legacy rotate tag is visible; older tools may treat it differently from a display matrix."
	}
	return ComboCheck{
		ID:        "C4-legacy-rotate-tag-plus-display-matrix",
		Title:     "旧 rotate 标签 × Display Matrix 被当成两次变换",
		Scope:     "file",
		Triggered: triggered, Status: status, Severity: severity,
		Conditions: map[string]any{
			"legacyTagPresent": hasTag,
			"legacyTagTurnsCW": tagTurns,
			"matrixTurnsCW":    o.MatrixTurnsCW,
			"equivalent":       equivalent,
			"conflict":         conflict,
		},
		Analysis: analysis,
		Fix:      "Normalize to one display matrix. Do not add a tag-derived filter when the demuxer/player already exposes/applies side-data rotation.",
	}
}

func displayDimensionsCombo(o Observation) ComboCheck {
	triggered := o.MatrixSwapsAxes && o.HasTrackDisplay
	status := "absent"
	severity := "info"
	analysis := "No quarter-turn matrix plus explicit tkhd display dimensions to cross-check."
	if triggered {
		switch {
		case o.TrackMatchesCoded:
			status = "normal_pre_matrix_track_dimensions"
			severity = "low"
			analysis = "tkhd width/height match the pre-matrix coded track rectangle. This is the usual square-pixel MP4/MOV arrangement; the matrix performs the axis swap."
		case o.TrackMatchesMatrixOutput:
			extremeSAR := o.TrackImpliedSAR > 1.2 || (o.TrackImpliedSAR > 0 && o.TrackImpliedSAR < 1.0/1.2)
			status = "double_axis_swap_display_dimensions"
			severity = "medium"
			if extremeSAR {
				severity = "high"
			}
			analysis = fmt.Sprintf("tkhd width/height already match the matrix-swapped dimensions while the same quarter-turn matrix remains active. FFmpeg derives an implied SAR of %.3f; SAR-aware rotation then reciprocates that SAR and can change the final display orientation/aspect. Dimension-only players, SAR-aware players, and transcode pipelines can therefore choose different swaps or scaling.", o.TrackImpliedSAR)
		default:
			status = "display_dimension_mismatch"
			severity = "medium"
			analysis = "tkhd display dimensions match neither the coded grid nor the matrix-swapped grid. Check crop, SAR/DAR, and authoring tools; do not infer rotation solely from width > height."
		}
	}
	return ComboCheck{
		ID:        "C5-display-width-height-vs-matrix-axis-swap",
		Title:     "显示宽高 × 矩阵换轴 × 真实像素宽高",
		Scope:     "file",
		Triggered: triggered, Status: status, Severity: severity,
		Conditions: map[string]any{
			"trackDisplay":             fmt.Sprintf("%.2fx%.2f", o.TrackDisplayWidth, o.TrackDisplayHeight),
			"coded":                    formatDimensions(o.CodedWidth, o.CodedHeight),
			"matrixOutput":             formatDimensions(o.MatrixOutputWidth, o.MatrixOutputHeight),
			"trackMatchesCoded":        o.TrackMatchesCoded,
			"trackMatchesMatrixOutput": o.TrackMatchesMatrixOutput,
			"trackImpliedSAR":          roundForReport(o.TrackImpliedSAR, 4),
		},
		Analysis:    analysis,
		Consequence: "One consumer may use tkhd as final display size, another applies the matrix and swaps again, while another honors the implied SAR; this produces inconsistent orientation or stretched frames.",
		Fix:         "Keep tkhd dimensions in the pre-matrix track coordinate system for square pixels, or explicitly author a valid anamorphic SAR. Do not pre-swap tkhd width/height and leave the rotation matrix active.",
	}
}

func HighRiskCombos(combos []ComboCheck) []ComboCheck {
	out := make([]ComboCheck, 0)
	for _, c := range combos {
		if c.Triggered && c.Severity == "high" {
			out = append(out, c)
		}
	}
	return out
}

func HighRiskFileCombos(combos []ComboCheck) []ComboCheck {
	out := make([]ComboCheck, 0)
	for _, c := range combos {
		if c.Scope == "file" && c.Triggered && c.Severity == "high" {
			out = append(out, c)
		}
	}
	return out
}
