package cutluma

import (
	"math"
	"sort"
)

// SegmentRepair is the correction recipe for one segment.
type SegmentRepair struct {
	Segment int    `json:"segment"`
	Mode    string `json:"mode"` // params_reconvert | quantile_lut | identity

	// Recipe-level correction hints (for a color operator).
	UnifiedParams ConvertParams `json:"unifiedParams"`
	BlackPointIn  float64       `json:"blackPointIn"`
	WhitePointIn  float64       `json:"whitePointIn"`
	ExposureEVIn  float64       `json:"exposureEvIn"`
	PowerIn       float64       `json:"powerIn"`
	CorrectionEV  float64       `json:"correctionEv"`

	// Empirical parametric map, used when no declared recipe exists.
	GammaRel float64 `json:"gammaRel,omitempty"`

	LUTSize int       `json:"lutSize"`
	LUT     []float64 `json:"lut"`
}

// SeamTrim is a short additive ramp that cancels residual seam mismatch.
type SeamTrim struct {
	CutIndex       int     `json:"cutIndex"`
	Frame          int     `json:"frame"`
	ResidualBefore float64 `json:"residualBefore"`
	ResidualAfter  float64 `json:"residualAfter"`
	LeftOffset     float64 `json:"leftOffset"`
	RightOffset    float64 `json:"rightOffset"`
	RampFrames     int     `json:"rampFrames"`
}

// CutVerification compares boundary gaps before and after repair.
type CutVerification struct {
	CutIndex     int     `json:"cutIndex"`
	Kind         string  `json:"kind"`
	GapBefore    float64 `json:"gapBefore"`
	GapAfter     float64 `json:"gapAfter"`
	AbsBefore    float64 `json:"absBefore"`
	AbsAfter     float64 `json:"absAfter"`
	ReductionPct float64 `json:"reductionPct"`
	SlopeBefore  float64 `json:"slopeBefore"`
	SlopeAfter   float64 `json:"slopeAfter"`
	SlopeKeptPct float64 `json:"slopeKeptPct"`
}

// RepairPlan is the complete portable repair output.
type RepairPlan struct {
	ReferenceSegment int               `json:"referenceSegment"`
	TargetParams     ConvertParams     `json:"targetParams"`
	LUTSize          int               `json:"lutSize"`
	Segments         []SegmentRepair   `json:"segments"`
	Trims            []SeamTrim        `json:"trims"`
	Verification     []CutVerification `json:"verification"`
}

// BuildRepairPlan creates one correction LUT per segment mapping it to the
// unified target recipe, then derives closed-loop seam trims.
func BuildRepairPlan(job *Job, rep *Report, lutSize int) (*RepairPlan, error) {
	if lutSize < 2 {
		lutSize = 257
	}
	target := rep.TargetParams
	plan := &RepairPlan{ReferenceSegment: 0, TargetParams: target, LUTSize: lutSize}

	refQuantiles := rep.Segments[0].Quantiles
	for _, sr := range rep.Segments {
		corr := SegmentRepair{
			Segment: sr.Index, UnifiedParams: target, LUTSize: lutSize,
			BlackPointIn: sr.BlackPoint, WhitePointIn: sr.WhitePoint,
			GammaRel: sr.GammaRel,
		}
		if sr.Declared != nil {
			lut, err := ReconversionMap(*sr.Declared, target, lutSize)
			if err != nil {
				return nil, err
			}
			corr.Mode = "params_reconvert"
			corr.LUT = lut
			corr.ExposureEVIn = sr.Declared.ExposureEV
			corr.PowerIn = sr.Declared.TransferPower
			corr.CorrectionEV = target.ExposureEV - sr.Declared.ExposureEV
		} else {
			corr.Mode = "quantile_lut"
			corr.LUT = quantileLUT(sr.Quantiles, refQuantiles, lutSize)
		}
		if sr.Index == 0 {
			corr.Mode = "identity_reference"
			corr.LUT = identityLUT(lutSize)
		}
		plan.Segments = append(plan.Segments, corr)
	}

	plan.Trims = deriveTrims(job, rep, plan)
	plan.Verification = VerifyRepair(job, rep, plan)
	return plan, nil
}

func identityLUT(n int) []float64 {
	lut := make([]float64, n)
	for i := range lut {
		lut[i] = float64(i) / float64(n-1)
	}
	return lut
}

// quantileLUT builds a monotone piecewise-linear map from one segment's
// typical quantiles onto the reference segment's quantiles.
func quantileLUT(fromQ, toQ []float64, size int) []float64 {
	type anchor struct{ x, y float64 }
	anchors := []anchor{{0, 0}}
	n := minInt(len(fromQ), len(toQ))
	lastX := 0.0
	for i := 0; i < n; i++ {
		x, y := clamp01(fromQ[i]), clamp01(toQ[i])
		if x <= lastX+1e-6 {
			continue
		}
		anchors = append(anchors, anchor{x, y})
		lastX = x
	}
	anchors = append(anchors, anchor{1, 1})
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].x < anchors[j].x })
	dedup := anchors[:0]
	for _, an := range anchors {
		if len(dedup) > 0 && an.x <= dedup[len(dedup)-1].x+1e-9 {
			if an.y > dedup[len(dedup)-1].y {
				dedup[len(dedup)-1].y = an.y
			}
			continue
		}
		dedup = append(dedup, an)
	}
	anchors = dedup

	lut := make([]float64, size)
	a := 0
	for i := 0; i < size; i++ {
		v := float64(i) / float64(size-1)
		for a+1 < len(anchors) && v > anchors[a+1].x {
			a++
		}
		if a+1 >= len(anchors) {
			lut[i] = anchors[len(anchors)-1].y
			continue
		}
		x0, y0 := anchors[a].x, anchors[a].y
		x1, y1 := anchors[a+1].x, anchors[a+1].y
		if x1-x0 < 1e-9 {
			lut[i] = y0
			continue
		}
		f := clamp01((v - x0) / (x1 - x0))
		lut[i] = clamp01(y0 + (y1-y0)*f)
	}
	return lut
}

func repairForSegment(plan *RepairPlan, seg int) *SegmentRepair {
	for i := range plan.Segments {
		if plan.Segments[i].Segment == seg {
			return &plan.Segments[i]
		}
	}
	return nil
}

// ApplyLUTs maps the median-luma curve through per-segment correction LUTs.
func ApplyLUTs(job *Job, plan *RepairPlan, qi int) []float64 {
	curve := quantileColumn(job, qi, false)
	out := make([]float64, len(curve))
	for t, v := range curve {
		sr := repairForSegment(plan, job.SegmentOf(t))
		if sr == nil || len(sr.LUT) == 0 {
			out[t] = v
			continue
		}
		out[t] = ApplyLUT(sr.LUT, v)
	}
	return out
}

// ApplyTrims adds the smooth seam-ramp offsets described by the plan.
func ApplyTrims(curve []float64, job *Job, plan *RepairPlan) []float64 {
	out := append([]float64(nil), curve...)
	for _, tr := range plan.Trims {
		c := tr.Frame
		r := tr.RampFrames
		if r <= 0 || (math.Abs(tr.LeftOffset) < 1e-6 && math.Abs(tr.RightOffset) < 1e-6) {
			continue
		}
		for k := 1; k <= r; k++ {
			w := 1 - float64(k-1)/float64(r) // full at the frame nearest cut
			t := c - k
			if t >= 0 && job.SegmentOf(t) == job.SegmentOf(c-1) {
				out[t] = clamp01(out[t] + tr.LeftOffset*w)
			}
			t = c + k - 1
			if t < len(out) && job.SegmentOf(t) == job.SegmentOf(c) {
				out[t] = clamp01(out[t] + tr.RightOffset*w)
			}
		}
	}
	return out
}

// CorrectedCurve returns LUT-corrected curve with trims applied.
func CorrectedCurve(job *Job, plan *RepairPlan) []float64 {
	qi := job.QIndex(0.50)
	if qi < 0 {
		qi = len(job.QuantileLevels) / 2
	}
	return ApplyTrims(ApplyLUTs(job, plan, qi), job, plan)
}

func deriveTrims(job *Job, rep *Report, plan *RepairPlan) []SeamTrim {
	qi := job.QIndex(0.50)
	if qi < 0 {
		qi = len(job.QuantileLevels) / 2
	}
	pre := ApplyLUTs(job, plan, qi)
	trims := make([]SeamTrim, 0, len(job.Cuts))
	for i, c := range job.Cuts {
		d := rep.Cuts[i]
		leftIdx, rightIdx := cutWindow(job, i, rep.Config)
		bf := measureBoundary(pre, leftIdx, rightIdx, c, rep.Config.EdgeFrames, rep.SigmaLuma)
		tr := SeamTrim{CutIndex: i, Frame: c, ResidualBefore: bf.fitGap}
		active := (d.Kind == KindConversion || d.Kind == KindMixed) && math.Abs(bf.fitGap) > 0.0035
		if active {
			ramp := d.BlendFrames + 2
			if ramp < 6 {
				ramp = 6
			}
			tr.RampFrames = ramp
			tr.LeftOffset = bf.fitGap / 2
			tr.RightOffset = -bf.fitGap / 2
			after := append([]float64(nil), pre...)
			trims2 := []SeamTrim{tr}
			after = ApplyTrims(after, job, &RepairPlan{Trims: trims2})
			af := measureBoundary(after, leftIdx, rightIdx, c, rep.Config.EdgeFrames, rep.SigmaLuma)
			tr.ResidualAfter = af.fitGap
		} else {
			tr.ResidualAfter = bf.fitGap
		}
		trims = append(trims, tr)
	}
	return trims
}

// VerifyRepair measures gap and trend preservation before/after correction.
func VerifyRepair(job *Job, rep *Report, plan *RepairPlan) []CutVerification {
	final := CorrectedCurve(job, plan)
	qi := job.QIndex(0.50)
	if qi < 0 {
		qi = len(job.QuantileLevels) / 2
	}
	raw := quantileColumn(job, qi, false)
	out := make([]CutVerification, 0, len(job.Cuts))
	for i, c := range job.Cuts {
		leftIdx, rightIdx := cutWindow(job, i, rep.Config)
		before := measureBoundary(raw, leftIdx, rightIdx, c, rep.Config.EdgeFrames, rep.SigmaLuma)
		after := measureBoundary(final, leftIdx, rightIdx, c, rep.Config.EdgeFrames, rep.SigmaLuma)
		v := CutVerification{
			CutIndex: i, Kind: rep.Cuts[i].Kind,
			GapBefore: before.fitGap, GapAfter: after.fitGap,
			AbsBefore: math.Abs(before.fitGap), AbsAfter: math.Abs(after.fitGap),
			SlopeBefore: before.trendChange, SlopeAfter: after.trendChange,
		}
		if v.AbsBefore > 1e-9 {
			v.ReductionPct = 100 * (1 - v.AbsAfter/v.AbsBefore)
		}
		if before.trendChange > 1e-9 {
			v.SlopeKeptPct = 100 * after.trendChange / before.trendChange
		}
		out = append(out, v)
	}
	return out
}
