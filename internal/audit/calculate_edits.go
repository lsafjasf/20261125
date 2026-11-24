package audit

import (
	"fmt"
	"math/big"
)

func calculateEdits(raw []rawEdit, movie *MovieInfo, tkhd *TKHDInfo, mdhd *MDHDInfo, trackIndex int) ([]EditReport, *AudioStart, []Finding) {
	var findings []Finding
	edits := make([]EditReport, 0, len(raw))
	var audioStart *AudioStart
	compositionCursor := new(big.Int)

	var movieTS, mediaTS uint32
	if movie != nil {
		movieTS = movie.Timescale
	}
	if mdhd != nil {
		mediaTS = mdhd.Timescale
	}

	for i, rawEdit := range raw {
		compStart := new(big.Rat).SetInt(compositionCursor)
		segDur := uintRat(rawEdit.segmentDuration)
		report := EditReport{
			Index:           i,
			SegmentDuration: rawEdit.segmentDuration,
			Composition:     intervalFrom(compStart, segDur, movieTS),
			RateRaw:         fmt.Sprintf("0x%08x", rawEdit.rateRaw),
		}

		rateSigned := int32(rawEdit.rateRaw)
		rate := big.NewRat(int64(rateSigned), 65536)
		report.RateRational = rate.RatString()
		report.RateFixed = decimalString(rate, 6)

		if rawEdit.mediaTime == -1 {
			report.Empty = true
			report.Kind = "empty_edit"
			report.Warnings = append(report.Warnings, "media_time=-1: standard empty edit; composition interval is reserved and has no source media interval")
		} else {
			if rawEdit.mediaTime < -1 {
				report.NegativeMediaTime = true
				report.Warnings = append(report.Warnings, "nonstandard negative media_time (<-1); strict FFmpeg rejects it, but signed arithmetic is retained for audit")
				findings = append(findings, Finding{
					Severity: "warning", Code: "negative_media_time_field", TrackIndex: trackIndex, EditIndex: i,
					BoxPath: "/moov/trak/edts/elst",
					Message: fmt.Sprintf("edit %d has nonstandard media_time=%d", i, rawEdit.mediaTime),
				})
			}

			mediaStart := intRat(rawEdit.mediaTime)
			if rawEdit.rateRaw == 0 {
				report.Dwell = true
				report.Kind = "dwell_zero_rate"
				zero := new(big.Rat)
				interval := intervalFrom(mediaStart, zero, mediaTS)
				report.Media = &interval
				report.MediaDurationRound = roundedUnits(zero)
				report.Warnings = append(report.Warnings, "media_rate=0: ISO BMFF dwell; present the single media-time point for the whole segment and consume zero media duration")
				findings = append(findings, Finding{
					Severity: "warning", Code: "zero_rate_dwell", TrackIndex: trackIndex, EditIndex: i,
					BoxPath: "/moov/trak/edts/elst",
					Message: fmt.Sprintf("edit %d is a zero-rate dwell at media time %d", i, rawEdit.mediaTime),
				})
			} else if movieTS == 0 || mediaTS == 0 {
				report.Warnings = append(report.Warnings, "cannot calculate cross-timescale media interval because mvhd or mdhd timescale is zero")
				findings = append(findings, Finding{
					Severity: "error", Code: "zero_timescale", TrackIndex: trackIndex, EditIndex: i,
					Message: "mvhd and mdhd timescales must be non-zero for edit conversion",
				})
			} else {
				oneXMediaDuration := new(big.Rat).Mul(segDur, big.NewRat(int64(mediaTS), int64(movieTS)))
				mediaDuration := new(big.Rat).Mul(oneXMediaDuration, rate)
				interval := intervalFrom(mediaStart, mediaDuration, mediaTS)
				report.Media = &interval
				report.MediaDurationRound = roundedUnits(mediaDuration)

				scaledMediaStart := new(big.Rat).Mul(mediaStart, big.NewRat(int64(movieTS), int64(mediaTS)))
				origin := new(big.Rat).Sub(compStart, new(big.Rat).Quo(scaledMediaStart, rate))
				point := pointFromUnits(origin, movieTS)
				report.MediaOriginOnComposition = &point
				if origin.Sign() < 0 {
					report.Warnings = append(report.Warnings, "media time zero maps to a negative composition time; sample timestamps require a negative shift")
				}
				if rate.Cmp(big.NewRat(1, 1)) != 0 {
					report.Warnings = append(report.Warnings, fmt.Sprintf("non-unit media rate %s: media interval duration is rate-scaled", report.RateFixed))
				}
			}

			if report.Kind == "" {
				report.Kind = "media_playback"
			}

			if audioStart == nil {
				start := &AudioStart{
					EditIndex:               i,
					Kind:                    report.Kind,
					SourceMediaTimeUnits:    rawEdit.mediaTime,
					SourceMediaTimeSeconds:  signedRationalSeconds(rawEdit.mediaTime, mediaTS),
					CompositionStartUnits:   compStart.RatString(),
					CompositionStartSeconds: ratSeconds(compStart, movieTS),
				}
				if compStart.Sign() > 0 {
					start.LeadingEmptySeconds = ratSeconds(compStart, movieTS)
				}
				if movieTS > 0 && mediaTS > 0 && rate.Cmp(big.NewRat(1, 1)) == 0 {
					compositionInMediaUnits := new(big.Rat).Mul(compStart, big.NewRat(int64(mediaTS), int64(movieTS)))
					offset := new(big.Rat).Sub(intRat(rawEdit.mediaTime), compositionInMediaUnits)
					point := pointFromUnits(offset, mediaTS)
					start.UnitRateOffsetMedia = &point
				}
				if report.MediaOriginOnComposition != nil {
					point := *report.MediaOriginOnComposition
					start.MediaZeroCompUnits = &point
					start.NegativeTimelineOrigin = point.SecondsExact != "0" && isNegativeRatString(point.UnitsExact)
				}
				audioStart = start
			}
		}

		edits = append(edits, report)
		compositionCursor.Add(compositionCursor, new(big.Int).SetUint64(rawEdit.segmentDuration))
	}

	findings = append(findings, checkEditStructure(raw, edits, movie, tkhd, mdhd, trackIndex, compositionCursor)...)
	return edits, audioStart, findings
}

func isNegativeRatString(s string) bool {
	return len(s) > 0 && s[0] == '-' && s != "-0" && s != "0"
}
