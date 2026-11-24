package sync

import (
	"math"
	"sort"
)

const (
	KindInSync       = "in_sync"
	KindFixedOffset  = "fixed_offset"
	KindLinearDrift  = "linear_drift"
	KindLocalJump    = "local_jump"
	KindFrameSkip    = "local_frame_skip"
	KindIrregular    = "irregular"
	KindInsufficient = "insufficient_data"
)

// SeriesPoint is one aligned offset observation at a frame.
type SeriesPoint struct {
	Frame int64 `json:"frame"`
	Y     int64 `json:"y_ns"`
}

// SeriesReport classifies one offset or timeline series.
type SeriesReport struct {
	Name       string `json:"name"`
	Stage      Stage  `json:"stage,omitempty"`
	Kind       string `json:"kind"`
	Confidence string `json:"confidence"`
	Samples    int    `json:"samples"`

	FirstFrame int64 `json:"first_frame"`
	LastFrame  int64 `json:"last_frame"`

	InitialMedianNS int64 `json:"initial_median_ns"`
	FinalMedianNS   int64 `json:"final_median_ns"`
	LastValueNS     int64 `json:"last_value_ns"`
	MedianNS        int64 `json:"median_ns"`
	MADNS           int64 `json:"mad_ns"`

	SlopeNSPerFrame float64 `json:"slope_ns_per_frame"`
	DriftNS         int64   `json:"drift_ns"`
	R2              float64 `json:"r2"`

	JumpFrame                *int64  `json:"jump_frame,omitempty"`
	JumpNS                   *int64  `json:"jump_ns,omitempty"`
	Persistent               bool    `json:"persistent"`
	EstimatedSkippedFrames   int64   `json:"estimated_skipped_frames,omitempty"`
	FrameGridResidualNS      int64   `json:"frame_grid_residual_ns,omitempty"`
	PostJumpSlopeNSPerFrame  float64 `json:"post_jump_slope_ns_per_frame,omitempty"`
	FirstChangeFrame         *int64  `json:"first_change_frame,omitempty"`
	FirstAbsoluteOffsetFrame *int64  `json:"first_absolute_offset_frame,omitempty"`
	EstimatedOnsetFrame      *int64  `json:"estimated_onset_frame,omitempty"`
}

// AnalyzeSeries identifies constant offsets, linear drift, and persistent jumps.
func AnalyzeSeries(name string, stage Stage, input []SeriesPoint, opts Options) SeriesReport {
	opts = opts.normalized()
	points := append([]SeriesPoint(nil), input...)
	sort.SliceStable(points, func(i, j int) bool {
		return points[i].Frame < points[j].Frame
	})

	report := SeriesReport{Name: name, Stage: stage, Kind: KindInsufficient}
	n := len(points)
	report.Samples = n
	if n == 0 {
		return report
	}

	frames := make([]float64, n)
	values := make([]int64, n)
	ys := make([]float64, n)
	for i, point := range points {
		frames[i] = float64(point.Frame)
		values[i] = point.Y
		ys[i] = float64(point.Y)
	}

	report.FirstFrame = points[0].Frame
	report.LastFrame = points[n-1].Frame
	report.MedianNS = medianInt64(values)
	report.MADNS = medianAbsDeviation(values, report.MedianNS)

	initialCount := minInt(n, 5)
	report.InitialMedianNS = medianInt64(values[:initialCount])
	tailCount := n / 10
	if tailCount < 3 {
		tailCount = minInt(n, 3)
	}
	if tailCount > 64 {
		tailCount = 64
	}
	report.FinalMedianNS = medianInt64(values[n-tailCount:])
	report.LastValueNS = values[n-1]

	toleranceNS := int64(opts.Tolerance)
	for _, point := range points {
		frame := point.Frame
		if report.FirstChangeFrame == nil &&
			absInt64(point.Y-report.InitialMedianNS) >= toleranceNS {
			report.FirstChangeFrame = &frame
		}
		if report.FirstAbsoluteOffsetFrame == nil && absInt64(point.Y) >= toleranceNS {
			report.FirstAbsoluteOffsetFrame = &frame
		}
		if report.FirstChangeFrame != nil && report.FirstAbsoluteOffsetFrame != nil {
			break
		}
	}

	slope, intercept, r2 := linearRegression(frames, ys)
	report.SlopeNSPerFrame = slope
	report.R2 = r2
	report.DriftNS = int64(math.Round(slope * float64(points[n-1].Frame-points[0].Frame)))

	if n < 2 {
		report.Confidence = "low"
		return report
	}

	frameSlopes := make([]float64, 0, n-1)
	for i := 1; i < n; i++ {
		frameDelta := points[i].Frame - points[i-1].Frame
		if frameDelta > 0 {
			frameSlopes = append(frameSlopes, float64(points[i].Y-points[i-1].Y)/float64(frameDelta))
		}
	}
	robustSlope := medianFloat64(frameSlopes)

	persist := opts.Context
	if persist < 2 {
		persist = 2
	}

	for i := 1; i < n; i++ {
		frameDelta := points[i].Frame - points[i-1].Frame
		if frameDelta <= 0 {
			continue
		}
		actualDelta := points[i].Y - points[i-1].Y
		expectedDelta := int64(math.Round(robustSlope * float64(frameDelta)))
		jump := actualDelta - expectedDelta
		if absInt64(jump) < int64(opts.JumpThreshold) {
			continue
		}

		beforeStart := maxInt(0, i-persist)
		before := append([]int64(nil), values[beforeStart:i]...)
		afterEnd := minInt(n, i+persist+1)
		after := append([]int64(nil), values[i:afterEnd]...)
		if len(before) == 0 || len(after) == 0 {
			continue
		}
		shift := medianInt64(after) - medianInt64(before)
		persistent := len(after) >= 2 &&
			absInt64(shift) >= int64(opts.JumpThreshold) &&
			((jump > 0 && shift > 0) || (jump < 0 && shift < 0))

		frame := points[i].Frame
		jumpValue := jump
		report.JumpFrame = &frame
		report.JumpNS = &jumpValue
		report.Persistent = persistent
		report.Kind = KindLocalJump
		report.Confidence = "medium"

		absJump := math.Abs(float64(jump))
		gridFrames := int64(math.Round(absJump / float64(opts.FrameDuration)))
		gridResidual := int64(math.Round(math.Abs(absJump -
			float64(gridFrames)*float64(opts.FrameDuration))))
		allowedGridError := int64(opts.Tolerance + opts.FrameDuration/10)
		if gridFrames >= 1 && gridResidual <= allowedGridError {
			report.Kind = KindFrameSkip
			report.EstimatedSkippedFrames = gridFrames
			report.FrameGridResidualNS = gridResidual
			if persistent {
				report.Confidence = "high"
			}
		}

		if i+2 < n {
			postX := make([]float64, 0)
			postY := make([]float64, 0)
			for j := i; j < n; j++ {
				postX = append(postX, float64(points[j].Frame))
				postY = append(postY, float64(points[j].Y))
			}
			postSlope, _, _ := linearRegression(postX, postY)
			report.PostJumpSlopeNSPerFrame = postSlope
		}
		if !persistent {
			report.Confidence = "low"
		}
		return report
	}

	spreadLimit := int64(opts.Tolerance)
	if thresholdFrame := firstSeriesThresholdFrame(&report); thresholdFrame != nil {
		startIndex := sort.Search(n, func(i int) bool {
			return points[i].Frame >= *thresholdFrame
		})
		if n-startIndex >= 4 {
			segmentX := frames[startIndex:]
			segmentY := ys[startIndex:]
			segmentSlope, segmentIntercept, segmentR2 := linearRegression(segmentX, segmentY)
			segmentDrift := int64(math.Round(segmentSlope *
				float64(points[n-1].Frame-points[startIndex].Frame)))
			if absInt64(segmentDrift) >= int64(opts.DriftBudget) && segmentR2 >= 0.80 {
				report.Kind = KindLinearDrift
				report.SlopeNSPerFrame = segmentSlope
				report.R2 = segmentR2
				report.DriftNS = segmentDrift
				report.EstimatedOnsetFrame = onsetFrame(segmentSlope, segmentIntercept)
				report.Confidence = confidenceFor(n-startIndex, segmentR2 >= 0.95)
				return report
			}
		}
	}
	if absInt64(report.DriftNS) >= int64(opts.DriftBudget) && report.R2 >= 0.70 {
		report.Kind = KindLinearDrift
		report.Confidence = confidenceFor(n, report.R2 >= 0.9)
		report.EstimatedOnsetFrame = onsetFrame(slope, intercept)
		return report
	}

	if absInt64(report.MedianNS) >= int64(opts.Tolerance) && report.MADNS <= spreadLimit {
		report.Kind = KindFixedOffset
		report.Confidence = confidenceFor(n, true)
		return report
	}

	if absInt64(report.MedianNS) < int64(opts.Tolerance) &&
		absInt64(report.DriftNS) < int64(opts.DriftBudget) &&
		report.MADNS <= spreadLimit {
		report.Kind = KindInSync
		report.Confidence = confidenceFor(n, true)
		return report
	}

	report.Kind = KindIrregular
	report.Confidence = "low"
	return report
}

func confidenceFor(samples int, strong bool) string {
	switch {
	case samples >= 64 && strong:
		return "high"
	case samples >= 8:
		return "medium"
	default:
		return "low"
	}
}

func onsetFrame(slope, intercept float64) *int64 {
	if math.Abs(slope) < 1e-12 {
		return nil
	}
	frame := int64(math.Round(-intercept / slope))
	return &frame
}

func medianInt64(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func medianAbsDeviation(values []int64, median int64) int64 {
	deviations := make([]int64, len(values))
	for i, value := range values {
		deviations[i] = absInt64(value - median)
	}
	return medianInt64(deviations)
}

func medianFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func linearRegression(x, y []float64) (slope, intercept, r2 float64) {
	n := len(x)
	if n < 2 {
		return 0, 0, 0
	}
	var sumX, sumY, sumXX, sumXY float64
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
		sumXX += x[i] * x[i]
		sumXY += x[i] * y[i]
	}
	nf := float64(n)
	denom := nf*sumXX - sumX*sumX
	if denom == 0 {
		return 0, y[0], 0
	}
	slope = (nf*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / nf

	meanY := sumY / nf
	var ssTot, ssRes float64
	for i := 0; i < n; i++ {
		predicted := slope*x[i] + intercept
		ssRes += (y[i] - predicted) * (y[i] - predicted)
		ssTot += (y[i] - meanY) * (y[i] - meanY)
	}
	if ssTot == 0 {
		r2 = 1
	} else {
		r2 = 1 - ssRes/ssTot
		if r2 < 0 {
			r2 = 0
		}
	}
	return slope, intercept, r2
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
