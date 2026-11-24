package sync

import (
	"fmt"
	"math"
)

// Analyze performs frame-correlated stage, offset, drift, and jump analysis.
func Analyze(records []Record, options Options) (*Report, error) {
	opts := options.normalized()
	points, err := Normalize(records)
	if err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("no trace records were provided")
	}

	tracks, err := BuildTracks(points)
	if err != nil {
		return nil, err
	}
	rows := BuildPositionRows(tracks)

	report := &Report{
		Schema:         "avsync.report/v1",
		Options:        opts,
		Series:         []SeriesReport{},
		StageSummaries: []StageSummary{},
		Anomalies:      []Anomaly{},
	}
	anomalies := []Anomaly{}

	for _, stage := range Stages {
		track := tracks[stage]
		if len(track) == 0 {
			continue
		}

		summary := StageSummary{Stage: stage, Samples: len(track)}
		summary.FirstFrame = track[0].Frame
		summary.LastFrame = track[len(track)-1].Frame
		if track[0].PositionNS != nil {
			value := *track[0].PositionNS
			summary.FirstPositionNS = &value
		}
		if track[len(track)-1].PositionNS != nil {
			value := *track[len(track)-1].PositionNS
			summary.LastPositionNS = &value
		}

		frameDeltas := []int64{}
		outputDeltas := []int64{}
		for i := 1; i < len(track); i++ {
			if delta := track[i].Frame - track[i-1].Frame; delta > 0 {
				frameDeltas = append(frameDeltas, delta)
			}
			if track[i-1].OutputFrame != nil && track[i].OutputFrame != nil {
				if delta := *track[i].OutputFrame - *track[i-1].OutputFrame; delta > 0 {
					outputDeltas = append(outputDeltas, delta)
				}
			}
		}
		frameStride := opts.FrameStride
		if frameStride <= 0 {
			frameStride = medianInt64(frameDeltas)
		}
		if frameStride <= 0 {
			frameStride = 1
		}
		outputStride := medianInt64(outputDeltas)
		if outputStride <= 0 {
			outputStride = 1
		}
		summary.FrameStride = frameStride
		summary.OutputStride = outputStride

		for i := 1; i < len(track); i++ {
			previous := track[i-1]
			current := track[i]
			frameDelta := current.Frame - previous.Frame
			if frameDelta > 1 {
				summary.FrameIndexGaps++
			}
			if frameDelta > frameStride {
				missing := frameDelta - frameStride
				anomalies = append(anomalies, Anomaly{
					Frame:         current.Frame,
					Stage:         stage,
					Series:        "frame_index_gap_" + string(stage),
					Kind:          KindFrameSkip,
					MissingFrames: missing,
					Message: fmt.Sprintf("%s 帧号相对采样步长 %d 跳至 %d，疑似少记录/丢弃 %d 帧",
						stage, frameStride, current.Frame, missing),
				})
			}
			if previous.OutputFrame != nil && current.OutputFrame != nil {
				outputDelta := *current.OutputFrame - *previous.OutputFrame
				if outputDelta > 1 {
					summary.OutputIndexGaps++
				}
				if outputDelta > outputStride {
					missing := outputDelta - outputStride
					anomalies = append(anomalies, Anomaly{
						Frame:         current.Frame,
						Stage:         stage,
						Series:        "output_index_gap_" + string(stage),
						Kind:          KindFrameSkip,
						MissingFrames: missing,
						Message: fmt.Sprintf("%s 输出帧号相对步长 %d 跳至 %d，疑似跳帧 %d",
							stage, outputStride, *current.OutputFrame, missing),
					})
				}
			}
			if previous.PositionNS == nil || current.PositionNS == nil || frameDelta <= 0 {
				continue
			}

			positionDelta := *current.PositionNS - *previous.PositionNS
			expectedDelta := int64(math.Round(float64(frameDelta) * float64(opts.FrameDuration)))
			jump := positionDelta - expectedDelta
			if absInt64(jump) < int64(opts.JumpThreshold) {
				continue
			}

			summary.PositionJumps++
			kind := KindLocalJump
			missing := int64(0)
			gridFrames := int64(math.Round(math.Abs(float64(jump)) / float64(opts.FrameDuration)))
			gridResidual := int64(math.Round(math.Abs(math.Abs(float64(jump)) -
				float64(gridFrames)*float64(opts.FrameDuration))))
			allowedGridError := int64(opts.Tolerance + opts.FrameDuration/10)
			if gridFrames >= 1 && gridResidual <= allowedGridError {
				kind = KindFrameSkip
				missing = gridFrames
			}
			anomalies = append(anomalies, Anomaly{
				Frame:         current.Frame,
				Stage:         stage,
				Series:        "position_delta_" + string(stage),
				Kind:          kind,
				DeltaNS:       positionDelta,
				JumpNS:        jump,
				MissingFrames: missing,
				Message: fmt.Sprintf("%s 在帧 %d 出现 %d ns 位置阶跃",
					stage, current.Frame, jump),
			})
		}
		report.StageSummaries = append(report.StageSummaries, summary)
	}

	addSeries := func(series SeriesReport) {
		report.Series = append(report.Series, series)
		if series.JumpFrame == nil || series.JumpNS == nil {
			return
		}
		anomalies = append(anomalies, Anomaly{
			Frame:         *series.JumpFrame,
			Stage:         series.Stage,
			Series:        series.Name,
			Kind:          series.Kind,
			JumpNS:        *series.JumpNS,
			MissingFrames: series.EstimatedSkippedFrames,
			Message: fmt.Sprintf("%s 在帧 %d 出现 %d ns 持续偏移阶跃",
				series.Name, *series.JumpFrame, *series.JumpNS),
		})
	}

	for _, stage := range Stages {
		track := tracks[stage]
		points := []SeriesPoint{}
		haveBase := false
		var baseFrame, basePosition int64
		for _, point := range track {
			if point.PositionNS == nil {
				continue
			}
			if !haveBase {
				baseFrame = point.Frame
				basePosition = *point.PositionNS
				haveBase = true
			}
			expected := basePosition + int64(math.Round(
				float64(point.Frame-baseFrame)*float64(opts.FrameDuration)))
			points = append(points, SeriesPoint{
				Frame: point.Frame,
				Y:     *point.PositionNS - expected,
			})
		}
		addSeries(AnalyzeSeries("timeline_"+string(stage), stage, points, opts))
	}

	positionValue := func(row PositionRow, stage Stage) (int64, bool) {
		value, ok := row.Positions[stage]
		return value, ok
	}
	for i := 0; i+1 < len(Stages); i++ {
		from := Stages[i]
		to := Stages[i+1]
		points := offsetSeries(rows, from, to, positionValue)
		addSeries(AnalyzeSeries(string(from)+"_to_"+string(to), to, points, opts))
	}

	endToEnd := offsetSeries(rows, StageCapture, StagePlayback, positionValue)
	addSeries(AnalyzeSeries("end_to_end_capture_to_playback", StagePlayback, endToEnd, opts))

	for _, stage := range Stages {
		points := []SeriesPoint{}
		for _, row := range rows {
			if offset, ok := row.LipOffset[stage]; ok {
				points = append(points, SeriesPoint{Frame: row.Frame, Y: offset})
			}
		}
		addSeries(AnalyzeSeries("lip_sync_"+string(stage), stage, points, opts))
	}

	sortAnomalies(anomalies)
	report.Anomalies = anomalies

	if len(anomalies) > 0 {
		first := anomalies[0]
		ref := FrameRef{
			Frame:  first.Frame,
			Stage:  first.Stage,
			Series: first.Series,
			Kind:   first.Kind,
			Detail: first.Message,
		}
		report.FirstAbnormalFrame = &ref
		report.FirstRootCauseFrame = &ref
		for _, anomaly := range anomalies {
			if anomaly.Stage == StagePlayback {
				playbackRef := FrameRef{
					Frame:  anomaly.Frame,
					Stage:  anomaly.Stage,
					Series: anomaly.Series,
					Kind:   anomaly.Kind,
					Detail: anomaly.Message,
				}
				report.FirstPlaybackAnomalyFrame = &playbackRef
				break
			}
		}

		evidence := baselineAndTailRows(rows, opts.Context)
		window := evidenceWindow(rows, first.Frame, opts.Context)
		report.EvidenceRows = mergeRows(evidence, window)
	} else {
		report.EvidenceRows = baselineAndTailRows(rows, opts.Context)
	}

	conclude(report)
	if report.FirstAbnormalFrame != nil {
		window := evidenceWindow(rows, report.FirstAbnormalFrame.Frame, opts.Context)
		report.EvidenceRows = mergeRows(report.EvidenceRows, window)
	}
	return report, nil
}

type rowValueFunc func(PositionRow, Stage) (int64, bool)

func offsetSeries(rows []PositionRow, from, to Stage, value rowValueFunc) []SeriesPoint {
	points := []SeriesPoint{}
	for _, row := range rows {
		fromValue, fromOK := value(row, from)
		toValue, toOK := value(row, to)
		if fromOK && toOK {
			points = append(points, SeriesPoint{Frame: row.Frame, Y: toValue - fromValue})
		}
	}
	return points
}

func mergeRows(groups ...[]PositionRow) []PositionRow {
	byFrame := make(map[int64]PositionRow)
	order := []int64{}
	for _, group := range groups {
		for _, row := range group {
			if _, exists := byFrame[row.Frame]; !exists {
				order = append(order, row.Frame)
			}
			byFrame[row.Frame] = row
		}
	}
	sortInt64s(order)
	result := make([]PositionRow, 0, len(order))
	for _, frame := range order {
		result = append(result, byFrame[frame])
	}
	return result
}

func sortInt64s(values []int64) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j-1] > values[j]; j-- {
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}
