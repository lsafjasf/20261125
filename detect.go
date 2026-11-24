package cutluma

import (
	"math"
)

// SourceEvidence repeats the boundary analysis on pre-conversion luma.
type SourceEvidence struct {
	Available   bool    `json:"available"`
	EdgeJump    float64 `json:"edgeJump"`
	FitGap      float64 `json:"fitGap"`
	ZScore      float64 `json:"zScore"`
	Significant bool    `json:"significant"`
}

// CutDiagnosis is the forensic result for one splice.
type CutDiagnosis struct {
	CutIndex     int `json:"cutIndex"`
	Frame        int `json:"frame"`
	LeftSegment  int `json:"leftSegment"`
	RightSegment int `json:"rightSegment"`

	// Boundary level measurements in normalized output luma.
	EdgeJump    float64 `json:"edgeJump"`
	FitGap      float64 `json:"fitGap"`
	ZScore      float64 `json:"zScore"`
	OutlierZ    float64 `json:"outlierZ"`
	BoundaryInc float64 `json:"boundaryIncrement"`
	NoiseSE     float64 `json:"noiseSe"`
	WindowSwing float64 `json:"windowSwing"`

	// Robust trend evidence on each side.
	LeftSlope     float64 `json:"leftSlope"`
	RightSlope    float64 `json:"rightSlope"`
	TrendChange   float64 `json:"trendChange"`
	PreCutDrift   float64 `json:"preCutDrift"`
	PostCutDrift  float64 `json:"postCutDrift"`
	NaturalMotion bool    `json:"naturalMotion"`

	// Parameter / conversion evidence.
	ParamMismatch   bool    `json:"paramMismatch"`
	MismatchScore   float64 `json:"mismatchScore"`
	PredictedStep   float64 `json:"predictedStep"`
	PredictionError float64 `json:"predictionError"`
	PredictionOK    bool    `json:"predictionOk"`

	Source SourceEvidence `json:"source"`

	Kind        string   `json:"kind"`
	Confidence  float64  `json:"confidence"`
	ReasonCodes []string `json:"reasonCodes"`

	// RepairHint is the recommended cross-boundary blend in frames; zero
	// means a static per-segment LUT is enough / no seam should be touched.
	BlendFrames int `json:"blendFrames"`
}

// boundaryFeatures holds raw robust measurements for one luma curve.
type boundaryFeatures struct {
	edgeJump    float64
	fitGap      float64
	zScore      float64
	outlierZ    float64
	boundaryInc float64
	se          float64
	windowSwing float64
	leftSlope   float64
	rightSlope  float64
	trendChange float64
}

func cutWindow(job *Job, cut int, cfg Config) (leftIdx, rightIdx []int) {
	c := job.Cuts[cut]
	leftSeg := job.SegmentOf(c - 1)
	rightSeg := job.SegmentOf(c)
	leftStart := c - cfg.Window
	rightEnd := c + cfg.Window
	for _, s := range job.Segments {
		if s.Index == leftSeg {
			leftStart = maxInt(leftStart, s.Start)
		}
		if s.Index == rightSeg {
			rightEnd = minInt(rightEnd, s.End)
		}
	}
	for t := leftStart; t < c; t++ {
		leftIdx = append(leftIdx, t)
	}
	for t := c; t < rightEnd; t++ {
		rightIdx = append(rightIdx, t)
	}
	return leftIdx, rightIdx
}

// contextDrift compares robust edge levels with levels one longer context
// span away, capturing light changes that started well before the cut.
func contextDrift(curve []float64, job *Job, cutFrame int, cfg Config) (pre, post float64) {
	e := cfg.EdgeFrames
	ctx := cfg.ContextFrames
	// Pre-cut: near level vs far level, constrained to the left segment.
	leftSeg := job.SegmentOf(cutFrame - 1)
	leftStart := 0
	for _, s := range job.Segments {
		if s.Index == leftSeg {
			leftStart = s.Start
		}
	}
	nearL := robustRangeMedian(curve, cutFrame-e, cutFrame)
	farLEnd := cutFrame - ctx + e
	if farLEnd > cutFrame-e {
		farLEnd = cutFrame - e
	}
	farLStart := maxInt(leftStart, cutFrame-ctx)
	if farLEnd > farLStart {
		farL := robustRangeMedian(curve, farLStart, farLEnd)
		pre = nearL - farL
	}
	// Post-cut: near level vs far level, constrained to the right segment.
	rightSeg := job.SegmentOf(cutFrame)
	rightEnd := len(curve)
	for _, s := range job.Segments {
		if s.Index == rightSeg {
			rightEnd = s.End
		}
	}
	nearR := robustRangeMedian(curve, cutFrame, cutFrame+e)
	farRStart := cutFrame + ctx - e
	farREnd := minInt(rightEnd, cutFrame+ctx)
	if farRStart < farREnd {
		farR := robustRangeMedian(curve, farRStart, farREnd)
		post = farR - nearR
	}
	return pre, post
}

// robustRangeMedian returns the median of curve[start:end] with bounds.
func robustRangeMedian(curve []float64, start, end int) float64 {
	start = maxInt(0, start)
	end = minInt(len(curve), end)
	if end <= start {
		return 0
	}
	return Median(curve[start:end])
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// measureBoundary computes trend-adjusted discontinuity features.
func measureBoundary(curve []float64, leftIdx, rightIdx []int, cutFrame, edge int, sigma float64) boundaryFeatures {
	var bf boundaryFeatures
	if len(leftIdx) < 2 || len(rightIdx) < 2 {
		return bf
	}
	xL := make([]float64, len(leftIdx))
	yL := make([]float64, len(leftIdx))
	for i, t := range leftIdx {
		xL[i] = float64(t - cutFrame)
		yL[i] = curve[t]
	}
	xR := make([]float64, len(rightIdx))
	yR := make([]float64, len(rightIdx))
	for i, t := range rightIdx {
		xR[i] = float64(t - cutFrame)
		yR[i] = curve[t]
	}
	lineL := TheilSen(xL, yL)
	lineR := TheilSen(xR, yR)
	bf.leftSlope = lineL.Slope
	bf.rightSlope = lineR.Slope

	wL := len(leftIdx)
	wR := len(rightIdx)
	bf.trendChange = math.Max(math.Abs(lineL.Slope)*float64(wL), math.Abs(lineR.Slope)*float64(wR))

	e := minInt(edge, minInt(wL, wR))
	edgeL := Median(yL[wL-e:])
	edgeR := Median(yR[:e])
	bf.edgeJump = edgeR - edgeL
	bf.fitGap = lineR.At(0) - lineL.At(0)
	bf.windowSwing = Median(yR) - Median(yL)

	resid := make([]float64, 0, wL+wR)
	for i := range xL {
		resid = append(resid, yL[i]-lineL.At(xL[i]))
	}
	for i := range xR {
		resid = append(resid, yR[i]-lineR.At(xR[i]))
	}
	localSigma := RobustSigma(resid)
	noise := math.Max(sigma, localSigma)
	// Approximate SE for a difference of two medians of e samples.
	bf.se = noise * math.Sqrt(math.Pi/float64(e))
	if bf.se < 1e-6 {
		bf.se = 1e-6
	}
	bf.zScore = math.Abs(bf.edgeJump) / bf.se

	// Boundary-increment outlier test. A true splice step makes the single
	// adjacent increment at the cut a robust outlier versus all within-side
	// increments; a smooth light ramp has boundary increments in family.
	var incDiffs []float64
	collect := func(idx []int) {
		for i := 1; i < len(idx); i++ {
			if idx[i] == idx[i-1]+1 {
				incDiffs = append(incDiffs, curve[idx[i]]-curve[idx[i-1]])
			}
		}
	}
	collect(leftIdx)
	collect(rightIdx)
	if cutFrame > 0 && cutFrame < len(curve) {
		bf.boundaryInc = curve[cutFrame] - curve[cutFrame-1]
	}
	centerInc := Median(incDiffs)
	incSE := math.Max(RobustSigma(incDiffs), noise*math.Sqrt2)
	bf.outlierZ = math.Abs(bf.boundaryInc-centerInc) / incSE
	return bf
}

func diagnoseCut(job *Job, cutIndex, cutFrame int, cfg Config, sigma float64) CutDiagnosis {
	leftSeg := job.SegmentOf(cutFrame - 1)
	rightSeg := job.SegmentOf(cutFrame)
	d := CutDiagnosis{
		CutIndex: cutIndex, Frame: cutFrame,
		LeftSegment: leftSeg, RightSegment: rightSeg,
	}
	medQI := job.QIndex(0.50)
	if medQI < 0 {
		medQI = len(job.QuantileLevels) / 2
	}
	outCurve := quantileColumn(job, medQI, false)
	leftIdx, rightIdx := cutWindow(job, cutIndex, cfg)
	bf := measureBoundary(outCurve, leftIdx, rightIdx, cutFrame, cfg.EdgeFrames, sigma)

	d.EdgeJump = bf.edgeJump
	d.FitGap = bf.fitGap
	d.ZScore = bf.zScore
	d.OutlierZ = bf.outlierZ
	d.BoundaryInc = bf.boundaryInc
	d.NoiseSE = bf.se
	d.WindowSwing = bf.windowSwing
	d.LeftSlope = bf.leftSlope
	d.RightSlope = bf.rightSlope
	d.TrendChange = bf.trendChange
	d.PreCutDrift, d.PostCutDrift = contextDrift(outCurve, job, cutFrame, cfg)
	d.NaturalMotion = bf.trendChange >= cfg.NaturalRamp ||
		math.Abs(d.PreCutDrift) >= cfg.NaturalDrift ||
		math.Abs(d.PostCutDrift) >= cfg.NaturalDrift

	significant := math.Abs(bf.fitGap) >= cfg.MinStep &&
		math.Abs(bf.edgeJump) >= cfg.MinStep*0.8 &&
		bf.zScore >= cfg.ZMin &&
		bf.outlierZ >= cfg.ZOutlier &&
		math.Abs(bf.boundaryInc) >= cfg.MinStep*0.6 &&
		sameSignish(bf.fitGap, bf.edgeJump)

	// Declared parameter mismatch + predicted step at a common source anchor.
	pL := findParams(job, leftSeg)
	pR := findParams(job, rightSeg)
	if pL != nil && pR != nil {
		pd := DiffParams(*pR, *pL)
		d.MismatchScore = mismatchScore(pd, cfg.Tolerance)
		d.ParamMismatch = d.MismatchScore > 1
		anchor := sourceAnchor(job, medQI, leftIdx, rightIdx, pL)
		d.PredictedStep = pR.Apply(anchor) - pL.Apply(anchor)
		d.PredictionError = math.Abs(bf.fitGap - d.PredictedStep)
		tol := math.Max(0.022, 0.35*math.Abs(bf.fitGap))
		d.PredictionOK = math.Abs(d.PredictedStep) >= cfg.MinStep*0.6 && d.PredictionError <= tol
	}

	// Optional source-domain evidence.
	if len(job.Frames) > 0 && len(job.Frames[0].Src) > 0 {
		srcCurve := quantileColumn(job, medQI, true)
		srcSigma := noiseWithinSegments(job, srcCurve, cfg.SigmaFloor)
		sf := measureBoundary(srcCurve, leftIdx, rightIdx, cutFrame, cfg.EdgeFrames, srcSigma)
		se := SourceEvidence{
			Available: true, EdgeJump: sf.edgeJump, FitGap: sf.fitGap, ZScore: sf.zScore,
			Significant: math.Abs(sf.fitGap) >= cfg.MinStep && sf.zScore >= cfg.ZMin*0.8,
		}
		d.Source = se
	}

	d.Kind, d.Confidence, d.ReasonCodes = classify(d, significant, cfg)
	switch d.Kind {
	case KindConversion:
		d.BlendFrames = 4
	case KindMixed:
		d.BlendFrames = 6
	default:
		d.BlendFrames = 0
	}
	return d
}

func sameSignish(a, b float64) bool {
	if math.Abs(a) < 1e-9 || math.Abs(b) < 1e-9 {
		return true
	}
	return math.Signbit(a) == math.Signbit(b)
}

func findParams(job *Job, seg int) *ConvertParams {
	for i := range job.Segments {
		if job.Segments[i].Index == seg {
			return job.Segments[i].Params
		}
	}
	return nil
}

// sourceAnchor estimates the common source-light level at the cut, using
// source measurements when present and inverse conversion otherwise.
func sourceAnchor(job *Job, medQI int, leftIdx, rightIdx []int, pL *ConvertParams) float64 {
	if len(job.Frames) > 0 && len(job.Frames[0].Src) > medQI {
		var vals []float64
		for _, t := range leftIdx[maxInt(0, len(leftIdx)-4):] {
			vals = append(vals, job.Frames[t].Src[medQI])
		}
		for _, t := range rightIdx[:minInt(4, len(rightIdx))] {
			vals = append(vals, job.Frames[t].Src[medQI])
		}
		if len(vals) > 0 {
			return Median(vals)
		}
	}
	if pL != nil && len(leftIdx) > 0 {
		var outs []float64
		for _, t := range leftIdx[maxInt(0, len(leftIdx)-4):] {
			outs = append(outs, job.Frames[t].Out[medQI])
		}
		return pL.Inverse(Median(outs))
	}
	return 0.4
}

func mismatchScore(d ParamDiff, tol ParamTolerance) float64 {
	return math.Max(
		math.Abs(d.BlackDelta)/tol.Black,
		math.Max(math.Abs(d.WhiteDelta)/tol.White,
			math.Max(math.Abs(d.ExposureEVDelta)/tol.ExposureEV,
				math.Abs(d.PowerDelta)/tol.Power)))
}

// classify maps evidence to a cause without ever treating a smooth trend as
// a conversion seam: the gate is the trend-adjusted boundary gap.
func classify(d CutDiagnosis, significant bool, cfg Config) (string, float64, []string) {
	reasons := []string{}
	if !significant {
		if d.NaturalMotion {
			reasons = append(reasons, "smooth_trend_across_cut", "boundary_gap_below_threshold")
			return KindNatural, 0.85, reasons
		}
		reasons = append(reasons, "boundary_gap_below_threshold", "no_significant_trend")
		return KindConsistent, 0.7, reasons
	}

	reasons = append(reasons, "boundary_discontinuity_significant")
	srcStepExplains := d.Source.Available && d.Source.Significant &&
		math.Abs(d.Source.FitGap) >= 0.55*math.Abs(d.FitGap) &&
		sameSignish(d.Source.FitGap, d.FitGap)

	switch {
	case srcStepExplains && !d.ParamMismatch:
		reasons = append(reasons, "source_domain_step_present", "params_consistent")
		return KindPreexisting, 0.92, reasons
	case d.ParamMismatch && d.PredictionOK && (!d.Source.Available || !d.Source.Significant):
		reasons = append(reasons, "declared_param_mismatch", "step_matches_param_prediction")
		if d.Source.Available {
			reasons = append(reasons, "source_domain_continuous")
		}
		if d.NaturalMotion {
			reasons = append(reasons, "underlying_natural_trend_preserved")
			return KindMixed, 0.9, reasons
		}
		return KindConversion, 0.96, reasons
	case d.Source.Available && !d.Source.Significant:
		reasons = append(reasons, "source_domain_continuous", "output_only_discontinuity")
		if d.ParamMismatch {
			reasons = append(reasons, "declared_param_mismatch")
		}
		if d.NaturalMotion {
			return KindMixed, 0.8, append(reasons, "underlying_natural_trend_preserved")
		}
		return KindConversion, 0.85, reasons
	case d.ParamMismatch:
		reasons = append(reasons, "declared_param_mismatch", "prediction_weak")
		if d.NaturalMotion {
			return KindMixed, 0.7, reasons
		}
		return KindConversion, 0.72, reasons
	default:
		reasons = append(reasons, "cause_not_separable")
		if d.Source.Available && d.Source.Significant {
			return KindPreexisting, 0.6, append(reasons, "source_domain_step_present")
		}
		return KindAmbiguous, 0.35, reasons
	}
}
