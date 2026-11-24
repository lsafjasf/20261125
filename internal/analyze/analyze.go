package analyze

import (
	"encoding/json"
	"fmt"
	"sort"

	"tearcheck/internal/detect"
	"tearcheck/internal/frameio"
	"tearcheck/internal/review"
)

type Options struct {
	Settings   detect.Settings
	ReviewFile *review.File
}

type loadedFrame struct {
	info            FrameInfo
	evidence        detect.FrameEvidence
	duplicateSource int
}

func MarshalReport(report *Report) ([]byte, error) {
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}
	return append(raw, '\n'), nil
}

func Analyze(plan *frameio.Plan, options Options) (*Report, error) {
	if plan == nil || len(plan.Frames) == 0 {
		return nil, fmt.Errorf("no frames to analyze")
	}
	if err := validateSettings(options.Settings); err != nil {
		return nil, err
	}
	reviewByID, reviewHash, err := prepareReview(options.ReviewFile, plan)
	if err != nil {
		return nil, err
	}

	loaded := make([]loadedFrame, len(plan.Frames))
	firstByHash := make(map[string]int, len(plan.Frames))
	evidenceByHash := make(map[string]detect.FrameEvidence)
	previousHash := ""
	previousRun := 0
	width, height := 0, 0
	uniqueCount := 0
	settings := options.Settings
	for index, spec := range plan.Frames {
		frame, err := frameio.LoadFrame(spec)
		if err != nil {
			return nil, err
		}
		if width == 0 {
			width, height = frame.Width, frame.Height
			settings = fitSettingsToFrame(settings, width, height)
			if err := validateSettings(settings); err != nil {
				return nil, err
			}
			if err := validateDimensions(width, height, settings); err != nil {
				return nil, err
			}
		} else if frame.Width != width || frame.Height != height {
			return nil, fmt.Errorf("frame %q is %dx%d, expected %dx%d", spec.ID, frame.Width, frame.Height, width, height)
		}

		info := FrameInfo{
			ID:         spec.ID,
			Index:      index,
			File:       spec.File,
			PTSUS:      spec.PTSUS,
			PTSMS:      spec.PTSUS / 1000,
			Timecode:   Timecode(spec.PTSUS),
			DurationUS: spec.DurationUS,
			Hash:       frame.Hash,
		}
		loaded[index] = loadedFrame{info: info, duplicateSource: -1}

		if source, exists := firstByHash[frame.Hash]; exists {
			run := 1
			if index > 0 && previousHash == frame.Hash {
				run = previousRun + 1
			}
			previousRun = run
			loaded[index].info.Duplicate = true
			loaded[index].info.DuplicateOf = loaded[source].info.ID
			loaded[index].info.DuplicateRun = run
			loaded[index].duplicateSource = source
			loaded[index].evidence = cloneEvidence(evidenceByHash[frame.Hash])
		} else {
			previousRun = 0
			firstByHash[frame.Hash] = index
			uniqueCount++
			evidence := detect.Detect(frame.Gray, settings)
			loaded[index].evidence = evidence
			evidenceByHash[frame.Hash] = evidence
		}
		previousHash = frame.Hash
	}

	built := buildEvents(loaded, settings)
	for index := range loaded {
		finalizeMachine(&loaded[index], built)
	}
	for index := range loaded {
		if loaded[index].duplicateSource >= 0 {
			source := loaded[index].duplicateSource
			if loaded[source].info.Machine.EventID != "" {
				loaded[index].info.Machine.EventID = loaded[source].info.Machine.EventID
			}
		}
	}

	report := &Report{
		Schema:      SchemaVersion,
		ToolVersion: ToolVersion,
		Input: InputInfo{
			FrameCount:        len(loaded),
			UniqueFrameCount:  uniqueCount,
			Width:             width,
			Height:            height,
			TimestampSource:   plan.TimestampSource,
			ManifestSHA256:    plan.ManifestSHA256,
			ManifestFrameRate: plan.FrameRate,
		},
		Settings: settings,
		Review: ReviewInfo{
			Applied:    len(reviewByID),
			FileSHA256: reviewHash,
		},
		Events: make([]Event, 0, len(built.events)),
		Frames: make([]FrameInfo, len(loaded)),
	}

	applyReviews(loaded, reviewByID)
	for index := range loaded {
		report.Frames[index] = loaded[index].info
	}
	report.Events = finalizeEvents(built, loaded)
	report.Summary = summarize(report)
	return report, nil
}

func prepareReview(file *review.File, plan *frameio.Plan) (map[string]review.Entry, string, error) {
	if file == nil {
		return map[string]review.Entry{}, "", nil
	}
	normalized, err := review.Normalize(file)
	if err != nil {
		return nil, "", err
	}
	known := make(map[string]bool, len(plan.Frames))
	for _, frame := range plan.Frames {
		known[frame.ID] = true
	}
	entries := make(map[string]review.Entry, len(normalized.Entries))
	for _, entry := range normalized.Entries {
		if !known[entry.FrameID] {
			return nil, "", fmt.Errorf("review entry references unknown frame %q", entry.FrameID)
		}
		entries[entry.FrameID] = entry
	}
	hash, err := review.Hash(normalized)
	if err != nil {
		return nil, "", err
	}
	return entries, hash, nil
}

func validateSettings(settings detect.Settings) error {
	if settings.BandRows < 1 {
		return fmt.Errorf("band_rows must be at least 1")
	}
	if settings.MaxShiftPX < 1 {
		return fmt.Errorf("max_shift_px must be at least 1")
	}
	if settings.MinShiftPX < 1 || settings.MinShiftPX > settings.MaxShiftPX {
		return fmt.Errorf("min_shift_px must be between 1 and max_shift_px")
	}
	if settings.MinTextureMean < 0 ||
		settings.MinPrefilterMismatchBP < 0 ||
		settings.MinMatchScoreBP < 0 ||
		settings.MinShiftGainBP < 0 ||
		settings.MaxBiasRatioBP < 0 ||
		settings.MaxBiasRatioBP > 10000 ||
		settings.MinStructuredDiff < 0 ||
		settings.MinPhotometricStep < 0 ||
		settings.MinLightBiasBP < 0 {
		return fmt.Errorf("detection settings contain an invalid negative or out-of-range value")
	}
	if settings.CandidateRowSeparation < 1 ||
		settings.MaxCandidatesPerFrame < 1 ||
		settings.MaxSeamsPerFrame < 1 ||
		settings.MaxLightEdgesPerFrame < 1 {
		return fmt.Errorf("candidate and seam limits must be positive")
	}
	if settings.MaxEventGapFrames < 1 ||
		settings.MaxRowJumpPX < 1 ||
		settings.MaxShiftJumpPX < 1 ||
		settings.MinContinuousFrames < 1 ||
		settings.BandPairTolerancePX < 0 ||
		settings.MinBandHeightPX < 1 {
		return fmt.Errorf("event settings contain an invalid value")
	}
	return nil
}

func validateDimensions(width, height int, settings detect.Settings) error {
	if width <= 2*(settings.MaxShiftPX+1) {
		return fmt.Errorf("frame width %d is too small for max_shift_px %d", width, settings.MaxShiftPX)
	}
	if height <= 2*settings.BandRows {
		return fmt.Errorf("frame height %d is too small for band_rows %d", height, settings.BandRows)
	}
	return nil
}

func fitSettingsToFrame(settings detect.Settings, width, height int) detect.Settings {
	if maxShift := (width - 16) / 2; maxShift < settings.MaxShiftPX {
		settings.MaxShiftPX = maxShift
	}
	if settings.MaxShiftPX < 1 {
		settings.MaxShiftPX = 1
	}
	if settings.MinShiftPX > settings.MaxShiftPX {
		settings.MinShiftPX = settings.MaxShiftPX
	}
	if bandRows := (height - 1) / 2; bandRows < settings.BandRows {
		settings.BandRows = bandRows
	}
	if settings.BandRows < 1 {
		settings.BandRows = 1
	}
	return settings
}

func cloneEvidence(source detect.FrameEvidence) detect.FrameEvidence {
	return detect.FrameEvidence{
		Seams:       append([]Seam(nil), source.Seams...),
		Bands:       append([]Band(nil), source.Bands...),
		LightEdges:  append([]LightEdge(nil), source.LightEdges...),
		TextureMean: source.TextureMean,
	}
}

func finalizeMachine(frame *loadedFrame, built eventBundle) {
	machine := MachineResult{}
	info := &frame.info
	if info.Duplicate {
		machine.Classification = "duplicate"
		machine.WidthPX = maxSeamWidth(frame.evidence.Seams)
		machine.Continuity = "duplicate"
		machine.Seams = frame.evidence.Seams
		machine.Bands = frame.evidence.Bands
		machine.LightEdges = frame.evidence.LightEdges
		if len(frame.evidence.Seams) > 0 {
			machine.ConfidenceBP = seamConfidence(frame.evidence.Seams[0])
		}
		info.Machine = machine
		return
	}

	if len(frame.evidence.Seams) == 0 {
		if len(frame.evidence.LightEdges) > 0 {
			machine.Classification = "light_effect"
		} else {
			machine.Classification = "clean"
		}
		machine.Continuity = "none"
		machine.LightEdges = frame.evidence.LightEdges
		info.Machine = machine
		return
	}

	bestSeamIndex := 0
	bestEventIndex := -1
	bestRank := -1
	for seamIndex := range frame.evidence.Seams {
		eventIndex := built.seamEvent[info.Index][seamIndex]
		if eventIndex < 0 || eventIndex >= len(built.events) {
			continue
		}
		event := built.events[eventIndex]
		rank := continuityRank(event.continuity)
		if rank > bestRank ||
			(rank == bestRank && frame.evidence.Seams[seamIndex].MatchScoreBP > frame.evidence.Seams[bestSeamIndex].MatchScoreBP) {
			bestRank = rank
			bestSeamIndex = seamIndex
			bestEventIndex = eventIndex
		}
	}
	if bestEventIndex < 0 || bestEventIndex >= len(built.events) {
		machine.Classification = "uncertain"
		machine.Continuity = "isolated"
		machine.Seams = frame.evidence.Seams
		machine.Bands = frame.evidence.Bands
		machine.WidthPX = maxSeamWidth(frame.evidence.Seams)
		info.Machine = machine
		return
	}
	bestSeam := frame.evidence.Seams[bestSeamIndex]
	event := built.events[bestEventIndex]
	machine.ConfidenceBP = seamConfidence(bestSeam)
	machine.WidthPX = maxSeamWidth(frame.evidence.Seams)
	machine.Continuity = event.continuity
	machine.EventID = event.id
	machine.Seams = frame.evidence.Seams
	machine.Bands = frame.evidence.Bands
	machine.LightEdges = frame.evidence.LightEdges

	switch {
	case len(frame.evidence.Bands) > 0:
		machine.Classification = "tear"
	case event.continuity == "continuous" || event.continuity == "intermittent":
		machine.Classification = "tear"
	default:
		machine.Classification = "uncertain"
	}
	info.Machine = machine
}

func applyReviews(frames []loadedFrame, entries map[string]review.Entry) {
	for index := range frames {
		info := &frames[index].info
		entry, reviewed := entries[info.ID]
		if reviewed {
			info.Review = &ReviewDecision{
				Decision: entry.Decision,
				Note:     entry.Note,
				Reviewer: entry.Reviewer,
			}
			switch entry.Decision {
			case review.DecisionConfirmTear:
				info.Final = "tear"
			case review.DecisionClear:
				info.Final = "not_tear"
			case review.DecisionUncertain:
				info.Final = "uncertain"
			}
			continue
		}
		switch info.Machine.Classification {
		case "tear":
			info.Final = "tear"
		case "uncertain", "duplicate":
			info.Final = info.Machine.Classification
		default:
			info.Final = "not_tear"
		}
	}
}

func finalizeEvents(built eventBundle, frames []loadedFrame) []Event {
	result := make([]Event, 0, len(built.events))
	for _, work := range built.events {
		if len(work.members) == 0 {
			continue
		}
		event := Event{
			ID:         work.id,
			FrameIDs:   make([]string, 0, len(work.members)),
			Evidence:   make([]EventFrame, 0, len(work.members)),
			Continuity: work.continuity,
		}
		widths := make([]int, 0, len(work.members))
		finalTear := false
		finalUncertain := false
		for memberIndex, member := range work.members {
			frame := frames[member.frameIndex].info
			if memberIndex == 0 {
				event.StartPTSUS = frame.PTSUS
				event.StartIndex = frame.Index
			}
			event.EndPTSUS = frame.PTSUS
			event.EndIndex = frame.Index
			event.FrameIDs = append(event.FrameIDs, frame.ID)
			widths = append(widths, member.seam.WidthPX)
			event.Evidence = append(event.Evidence, EventFrame{
				FrameID:        frame.ID,
				PTSUS:          frame.PTSUS,
				Timecode:       frame.Timecode,
				Row:            member.seam.Row,
				OffsetPX:       member.seam.OffsetPX,
				WidthPX:        member.seam.WidthPX,
				MatchScoreBP:   member.seam.MatchScoreBP,
				ShiftGainBP:    member.seam.ShiftGainBP,
				BiasRatioBP:    member.seam.BiasRatioBP,
				StructuredDiff: member.seam.StructuredDiff,
			})
			if frame.Review != nil {
				event.ReviewedFrames++
			}
			switch frame.Final {
			case "tear":
				finalTear = true
			case "uncertain":
				finalUncertain = true
			}
		}
		sort.Ints(widths)
		event.WidthPX = medianInt(widths)
		event.MinWidthPX = widths[0]
		event.MaxWidthPX = widths[len(widths)-1]
		switch {
		case finalTear:
			event.FinalClassification = "tear"
		case finalUncertain:
			event.FinalClassification = "uncertain"
		default:
			event.FinalClassification = "not_tear"
		}
		result = append(result, event)
	}
	return result
}

func summarize(report *Report) Summary {
	summary := Summary{
		Frames:       len(report.Frames),
		UniqueFrames: report.Input.UniqueFrameCount,
	}
	for _, frame := range report.Frames {
		switch frame.Machine.Classification {
		case "tear":
			summary.MachineTearFrames++
		case "uncertain":
			summary.MachineUncertainFrames++
		case "light_effect":
			summary.MachineLightEffectFrames++
		case "clean":
			summary.MachineCleanFrames++
		case "duplicate":
			summary.DuplicateFrames++
		}
		switch frame.Final {
		case "tear":
			summary.FinalTearFrames++
			if frame.Machine.WidthPX > summary.MaxWidthPX {
				summary.MaxWidthPX = frame.Machine.WidthPX
			}
		case "not_tear":
			summary.FinalNotTearFrames++
		case "uncertain":
			summary.FinalUncertainFrames++
		case "duplicate":
			summary.FinalDuplicateFrames++
		}
		if frame.Review != nil {
			summary.ReviewedFrames++
		}
	}
	for _, event := range report.Events {
		switch event.Continuity {
		case "continuous":
			summary.ContinuousEvents++
		case "intermittent":
			summary.IntermittentEvents++
		case "isolated":
			summary.IsolatedEvents++
		}
		if event.FinalClassification == "tear" {
			summary.TearEvents++
		}
	}
	return summary
}

func continuityRank(value string) int {
	switch value {
	case "continuous":
		return 3
	case "intermittent":
		return 2
	default:
		return 1
	}
}

func seamConfidence(seam Seam) int {
	value := (2*seam.MatchScoreBP + seam.ShiftGainBP) / 3
	if value < 0 {
		return 0
	}
	if value > 10000 {
		return 10000
	}
	return value
}

func maxSeamWidth(seams []Seam) int {
	width := 0
	for _, seam := range seams {
		if seam.WidthPX > width {
			width = seam.WidthPX
		}
	}
	return width
}

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

func Timecode(ptsUS int64) string {
	sign := ""
	if ptsUS < 0 {
		sign = "-"
		ptsUS = -ptsUS
	}
	milliseconds := ptsUS / 1000
	hours := milliseconds / 3_600_000
	minutes := (milliseconds / 60_000) % 60
	seconds := (milliseconds / 1000) % 60
	remainder := milliseconds % 1000
	return fmt.Sprintf("%s%02d:%02d:%02d.%03d", sign, hours, minutes, seconds, remainder)
}
