package cutluma

import (
	"math"
)

// Config tunes cut detection. Zero values fall back to DefaultConfig.
type Config struct {
	// Window is the number of frames inspected on each side of a cut.
	Window int `json:"window"`
	// EdgeFrames is the robust level sample count nearest the cut.
	EdgeFrames int `json:"edgeFrames"`
	// MinStep is the smallest normalized-luma discontinuity treated as a seam.
	MinStep float64 `json:"minStep"`
	// ZMin is the robust significance threshold for a discontinuity.
	ZMin float64 `json:"zMin"`
	// ZOutlier is the threshold for the boundary increment being an outlier.
	ZOutlier float64 `json:"zOutlier"`
	// NaturalRamp is the minimum per-window trend change indicating light motion.
	NaturalRamp float64 `json:"naturalRamp"`
	// ContextFrames is the longer look-back span used for pre-cut drift.
	ContextFrames int `json:"contextFrames"`
	// NaturalDrift is the minimum long-context level drift indicating light motion.
	NaturalDrift float64 `json:"naturalDrift"`
	// SigmaFloor models code-level noise when robust scale collapses.
	SigmaFloor float64 `json:"sigmaFloor"`
	// Tolerance defines declared-parameter consistency limits.
	Tolerance ParamTolerance `json:"tolerance"`
}

// DefaultConfig returns detection defaults calibrated for 8-bit-equivalent
// luma curves sampled every frame.
func DefaultConfig() Config {
	return Config{
		Window:        24,
		EdgeFrames:    5,
		MinStep:       0.010,
		ZMin:          6.0,
		ZOutlier:      6.0,
		NaturalRamp:   0.014,
		ContextFrames: 48,
		NaturalDrift:  0.018,
		SigmaFloor:    0.0012,
		Tolerance:     DefaultTolerance(),
	}
}

func (c Config) withDefaults() Config {
	d := DefaultConfig()
	if c.Window <= 0 {
		c.Window = d.Window
	}
	if c.EdgeFrames <= 0 {
		c.EdgeFrames = d.EdgeFrames
	}
	if c.MinStep <= 0 {
		c.MinStep = d.MinStep
	}
	if c.ZMin <= 0 {
		c.ZMin = d.ZMin
	}
	if c.ZOutlier <= 0 {
		c.ZOutlier = d.ZOutlier
	}
	if c.NaturalRamp <= 0 {
		c.NaturalRamp = d.NaturalRamp
	}
	if c.ContextFrames <= 0 {
		c.ContextFrames = d.ContextFrames
	}
	if c.NaturalDrift <= 0 {
		c.NaturalDrift = d.NaturalDrift
	}
	if c.SigmaFloor <= 0 {
		c.SigmaFloor = d.SigmaFloor
	}
	if c.Tolerance == (ParamTolerance{}) {
		c.Tolerance = d.Tolerance
	}
	return c
}

// SegmentReport collects declared and measured conversion descriptors.
type SegmentReport struct {
	Index int `json:"index"`
	Start int `json:"start"`
	End   int `json:"end"`

	// Measured effective points/exposure/transfer from output quantiles.
	BlackPoint         float64 `json:"blackPoint"`
	WhitePoint         float64 `json:"whitePoint"`
	MedianLuma         float64 `json:"medianLuma"`
	ExposureStopsVsRef float64 `json:"exposureStopsVsRef"`
	GammaRel           float64 `json:"gammaRel"`

	// Robust across-frame median of every distribution quantile.
	Quantiles []float64 `json:"quantiles"`

	Declared           *ConvertParams `json:"declared,omitempty"`
	DeclaredAvailable  bool           `json:"declaredAvailable"`
	DeltaVsTarget      *ParamDiff     `json:"deltaVsTarget,omitempty"`
	ParamsConsistent   bool           `json:"paramsConsistent"`
	InconsistentFields []string       `json:"inconsistentFields,omitempty"`
}

// Report is the full consistency + cut forensics result.
type Report struct {
	FrameCount   int             `json:"frameCount"`
	SegmentCount int             `json:"segmentCount"`
	CutCount     int             `json:"cutCount"`
	Config       Config          `json:"config"`
	SigmaLuma    float64         `json:"sigmaLuma"`
	TargetParams ConvertParams   `json:"targetParams"`
	Segments     []SegmentReport `json:"segments"`
	Cuts         []CutDiagnosis  `json:"cuts"`
}

// quantileColumn extracts one quantile column across all frames.
func quantileColumn(job *Job, qi int, src bool) []float64 {
	out := make([]float64, len(job.Frames))
	for i, f := range job.Frames {
		row := f.Out
		if src {
			row = f.Src
		}
		if qi >= 0 && qi < len(row) {
			out[i] = row[qi]
		}
	}
	return out
}

// segmentQuantiles computes the robust typical distribution of a segment:
// for every quantile level, the median across its frames.
func segmentQuantiles(job *Job, seg SegmentJob) []float64 {
	nq := len(job.QuantileLevels)
	cols := make([][]float64, nq)
	for q := range cols {
		cols[q] = make([]float64, 0, seg.End-seg.Start)
	}
	for t := seg.Start; t < seg.End && t < len(job.Frames); t++ {
		row := job.Frames[t].Out
		for q := 0; q < nq && q < len(row); q++ {
			cols[q] = append(cols[q], row[q])
		}
	}
	out := make([]float64, nq)
	for q := range cols {
		out[q] = Median(cols[q])
	}
	return out
}

// estimateGammaRel fits log(Qref) = c + gamma*log(Qseg) over midtone anchors.
func estimateGammaRel(qSeg, qRef, levels []float64) float64 {
	xs := make([]float64, 0, len(levels))
	ys := make([]float64, 0, len(levels))
	for i, q := range levels {
		if q < 0.10 || q > 0.90 || i >= len(qSeg) || i >= len(qRef) {
			continue
		}
		a, b := qSeg[i], qRef[i]
		if a < 0.02 || b < 0.02 || a > 0.999 || b > 0.999 {
			continue
		}
		xs = append(xs, math.Log(a))
		ys = append(ys, math.Log(b))
	}
	if len(xs) < 3 {
		return 1
	}
	m := TheilSen(xs, ys)
	if m.Slope <= 0.2 || m.Slope > 5 || math.IsNaN(m.Slope) {
		return 1
	}
	return m.Slope
}

// noiseWithinSegments estimates per-frame luma noise from adjacent-frame
// differences that never cross a cut.
func noiseWithinSegments(job *Job, curve []float64, floor float64) float64 {
	var diffs []float64
	for t := 1; t < len(curve); t++ {
		if job.SegmentOf(t) != job.SegmentOf(t-1) {
			continue
		}
		diffs = append(diffs, curve[t]-curve[t-1])
	}
	sigma := RobustSigma(diffs) / math.Sqrt2
	return math.Max(sigma, floor)
}

// Analyze measures every segment and diagnoses every cut.
func Analyze(job *Job, cfg Config, target *ConvertParams) (*Report, error) {
	cfg = cfg.withDefaults()
	if target == nil {
		if len(job.Segments) > 0 && job.Segments[0].Params != nil {
			target = job.Segments[0].Params
		} else {
			d := DefaultParams()
			target = &d
		}
	}

	segReps := make([]SegmentReport, 0, len(job.Segments))
	for _, seg := range job.Segments {
		q := segmentQuantiles(job, seg)
		rep := SegmentReport{
			Index: seg.Index, Start: seg.Start, End: seg.End, Quantiles: q,
			Declared: seg.Params, DeclaredAvailable: seg.Params != nil,
		}
		if qi := job.QIndex(0.005); qi >= 0 && qi < len(q) {
			rep.BlackPoint = q[qi]
		}
		if qi := job.QIndex(0.995); qi >= 0 && qi < len(q) {
			rep.WhitePoint = q[qi]
		}
		if qi := job.QIndex(0.50); qi >= 0 && qi < len(q) {
			rep.MedianLuma = q[qi]
		}
		if seg.Params != nil {
			d := DiffParams(*seg.Params, *target)
			rep.DeltaVsTarget = &d
			rep.ParamsConsistent = d.Consistent(cfg.Tolerance)
			rep.InconsistentFields = inconsistentFields(d, cfg.Tolerance)
		} else {
			rep.ParamsConsistent = true
		}
		segReps = append(segReps, rep)
	}

	refMedian := segReps[0].MedianLuma
	for i := range segReps {
		if refMedian > 1e-4 && segReps[i].MedianLuma > 1e-4 {
			segReps[i].ExposureStopsVsRef = math.Log2(segReps[i].MedianLuma / refMedian)
		}
		segReps[i].GammaRel = estimateGammaRel(segReps[i].Quantiles, segReps[0].Quantiles, job.QuantileLevels)
	}

	medQI := job.QIndex(0.50)
	if medQI < 0 {
		medQI = len(job.QuantileLevels) / 2
	}
	lumaCurve := quantileColumn(job, medQI, false)
	sigma := noiseWithinSegments(job, lumaCurve, cfg.SigmaFloor)

	cuts := make([]CutDiagnosis, 0, len(job.Cuts))
	for i, c := range job.Cuts {
		cuts = append(cuts, diagnoseCut(job, i, c, cfg, sigma))
	}

	return &Report{
		FrameCount:   job.FrameCount,
		SegmentCount: len(job.Segments),
		CutCount:     len(job.Cuts),
		Config:       cfg,
		SigmaLuma:    sigma,
		TargetParams: *target,
		Segments:     segReps,
		Cuts:         cuts,
	}, nil
}

func inconsistentFields(d ParamDiff, tol ParamTolerance) []string {
	var fields []string
	if math.Abs(d.BlackDelta) > tol.Black {
		fields = append(fields, "black")
	}
	if math.Abs(d.WhiteDelta) > tol.White {
		fields = append(fields, "white")
	}
	if math.Abs(d.ExposureEVDelta) > tol.ExposureEV {
		fields = append(fields, "exposureEV")
	}
	if math.Abs(d.PowerDelta) > tol.Power {
		fields = append(fields, "transferPower")
	}
	return fields
}
