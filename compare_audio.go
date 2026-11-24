package main

import "fmt"

func compareAudioIntervals(a, b *Archive, rep *Report) {
	groupsA := groupAudioIntervalsByTrack(a.Manifest.AudioIntervals)
	groupsB := groupAudioIntervalsByTrack(b.Manifest.AudioIntervals)
	usedB := map[uint32]bool{}

	for _, trackA := range a.Manifest.Tracks {
		if trackA.Kind != "audio" || len(groupsA[trackA.TrackID]) == 0 {
			continue
		}
		idB := matchAudioTrackID(trackA, groupsA[trackA.TrackID], b.Manifest.Tracks, groupsB, usedB)
		usedB[idB] = true
		compareAudioIntervalList(a, b, groupsA[trackA.TrackID], groupsB[idB], rep)
	}

	for _, trackB := range b.Manifest.Tracks {
		if trackB.Kind != "audio" || usedB[trackB.TrackID] || len(groupsB[trackB.TrackID]) == 0 {
			continue
		}
		usedB[trackB.TrackID] = true
		compareAudioIntervalList(a, b, nil, groupsB[trackB.TrackID], rep)
	}
}

func groupAudioIntervalsByTrack(intervals []AudioIntervalHash) map[uint32][]AudioIntervalHash {
	out := map[uint32][]AudioIntervalHash{}
	for _, interval := range intervals {
		out[interval.TrackID] = append(out[interval.TrackID], interval)
	}
	return out
}

func matchAudioTrackID(trackA TrackInfo, intervalsA []AudioIntervalHash, tracksB []TrackInfo, groupsB map[uint32][]AudioIntervalHash, used map[uint32]bool) uint32 {
	for _, trackB := range tracksB {
		if trackB.TrackID == trackA.TrackID {
			return trackB.TrackID
		}
	}
	for _, trackB := range tracksB {
		if used[trackB.TrackID] || trackB.Kind != "audio" {
			continue
		}
		intervalsB := groupsB[trackB.TrackID]
		if len(intervalsA) != len(intervalsB) {
			continue
		}
		equal := true
		for i := range intervalsA {
			if intervalsA[i].SHA256 != intervalsB[i].SHA256 {
				equal = false
				break
			}
		}
		if equal {
			return trackB.TrackID
		}
	}
	return matchTrackID(trackA.TrackID, trackA, tracksB, used)
}

func compareAudioIntervalList(a, b *Archive, beforeList, afterList []AudioIntervalHash, rep *Report) {
	bByIndex := map[int]AudioIntervalHash{}
	hashElsewhere := map[string]bool{}
	for _, interval := range afterList {
		bByIndex[interval.Index] = interval
		hashElsewhere[interval.SHA256] = true
	}
	normalReorderOnly := len(rep.Rearrangements) > 0
	usedAfter := map[int]bool{}

	for _, before := range beforeList {
		after, ok := bByIndex[before.Index]
		if ok {
			usedAfter[before.Index] = true
			if before.SHA256 == after.SHA256 {
				continue
			}
		} else if hashElsewhere[before.SHA256] {
			rep.addRearrangement(Change{
				Layer:        LayerAudioInterval,
				Kind:         "audio_interval_relocated",
				Message:      "identical audio interval appears at another interval index",
				OriginalHash: before.SHA256,
				CopyHash:     before.SHA256,
				Media:        audioMediaLocation(before),
			})
			continue
		}

		change := audioIntervalChange(a, b, before, after, ok)
		if normalReorderOnly {
			rep.addRearrangement(change)
		} else {
			rep.addMedia(change)
		}
	}

	for _, after := range afterList {
		if usedAfter[after.Index] {
			continue
		}
		change := Change{
			Layer:      LayerAudioInterval,
			Kind:       "audio_interval_added",
			Message:    "copy has an additional audio interval",
			CopyOffset: after.Offset,
			CopyEnd:    after.Offset + after.Size,
			CopyHash:   after.SHA256,
			Media:      audioMediaLocation(after),
		}
		if normalReorderOnly {
			rep.addRearrangement(change)
		} else {
			rep.addMedia(change)
		}
	}
}

func audioMediaLocation(interval AudioIntervalHash) *MediaLocation {
	return &MediaLocation{
		TrackID:       interval.TrackID,
		TrackKind:     "audio",
		IntervalIndex: interval.Index,
		AudioStart:    interval.StartSample,
		AudioEnd:      interval.EndSample,
		StartTime:     interval.StartSeconds,
		EndTime:       interval.EndSeconds,
		Description: fmt.Sprintf(
			"audio interval %d, samples [%d,%d), %.3f-%.3fs",
			interval.Index, interval.StartSample, interval.EndSample,
			interval.StartSeconds, interval.EndSeconds,
		),
	}
}

func audioIntervalChange(a, b *Archive, before, after AudioIntervalHash, hasAfter bool) Change {
	location := audioMediaLocation(before)
	change := Change{
		Layer:          LayerAudioInterval,
		Kind:           "audio_interval_changed",
		Message:        location.Description + " hash changed",
		OriginalOffset: before.Offset,
		OriginalEnd:    before.Offset + before.Size,
		OriginalHash:   before.SHA256,
		Media:          location,
	}
	if hasAfter {
		change.CopyOffset = after.Offset
		change.CopyEnd = after.Offset + after.Size
		change.CopyHash = after.SHA256
	}

	if hasAfter && before.Size > 0 && after.Size > 0 {
		diff, err := firstRangeDifference(
			a.Reader, before.Offset, before.Offset+before.Size,
			b.Reader, after.Offset, after.Offset+after.Size,
		)
		if err == nil && diff != nil {
			change.RelativeOffset = diff.Relative
			change.OriginalOffset = diff.OriginalOffset
			change.CopyOffset = diff.CopyOffset
			if before.Source == FormatWAV {
				sampleCount := before.EndSample - before.StartSample
				if sampleCount > 0 {
					frameBytes := before.Size / sampleCount
					if frameBytes > 0 {
						location.EarliestSample = before.StartSample + diff.Relative/frameBytes
						location.Description += fmt.Sprintf("; earliest PCM frame %d", location.EarliestSample)
					}
				}
			} else if sample := findSampleAtOffset(a.Manifest.Samples, diff.OriginalOffset); sample != nil {
				location.SampleIndex = sample.Index
				location.PTS = sample.PTS
				location.DTS = sample.DTS
				location.DisplayOrder = sample.DisplayOrder
				location.Sync = sample.Sync
				location.Description += fmt.Sprintf("; first mapped audio sample %d", sample.Index)
			}
		}
	} else if before.Source == FormatMP4 {
		fillFirstDifferentMP4AudioSample(a, b, before, after, hasAfter, &change)
	}
	return change
}

func findSampleAtOffset(samples []SampleHash, offset int64) *SampleHash {
	for i := range samples {
		if offset >= samples[i].Offset && offset < samples[i].Offset+samples[i].Size {
			return &samples[i]
		}
	}
	return nil
}

func fillFirstDifferentMP4AudioSample(a, b *Archive, before, after AudioIntervalHash, hasAfter bool, change *Change) {
	start := int(before.StartSample)
	end := int(before.EndSample)
	for zero := start; zero < end; zero++ {
		beforeSample := findTrackSampleByZeroIndex(a.Manifest.Samples, before.TrackID, zero)
		if beforeSample == nil {
			continue
		}
		var afterSample *SampleHash
		if hasAfter {
			afterSample = findTrackSampleByZeroIndex(b.Manifest.Samples, after.TrackID, zero)
		}
		if afterSample == nil || beforeSample.SHA256 == afterSample.SHA256 {
			continue
		}
		change.OriginalOffset = beforeSample.Offset
		change.OriginalEnd = beforeSample.Offset + beforeSample.Size
		if afterSample != nil {
			change.CopyOffset = afterSample.Offset
			change.CopyEnd = afterSample.Offset + afterSample.Size
			diff, err := firstRangeDifference(
				a.Reader, beforeSample.Offset, beforeSample.Offset+beforeSample.Size,
				b.Reader, afterSample.Offset, afterSample.Offset+afterSample.Size,
			)
			if err == nil && diff != nil {
				change.RelativeOffset = diff.Relative
				change.OriginalOffset = diff.OriginalOffset
				change.CopyOffset = diff.CopyOffset
			}
		}
		if change.Media != nil {
			change.Media.SampleIndex = beforeSample.Index
			change.Media.PTS = beforeSample.PTS
			change.Media.DTS = beforeSample.DTS
			change.Media.DisplayOrder = beforeSample.DisplayOrder
			change.Media.Sync = beforeSample.Sync
		}
		return
	}
}

func findTrackSampleByZeroIndex(samples []SampleHash, trackID uint32, zero int) *SampleHash {
	wantedIndex := zero + 1
	for i := range samples {
		if samples[i].TrackID == trackID && samples[i].Index == wantedIndex {
			return &samples[i]
		}
	}
	return nil
}
