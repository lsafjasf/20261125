package main

import (
	"fmt"
	"io"
)

func reportSampleRearrangement(pair sampleTrackPair, rep *Report) {
	positions := hashPositionMap(pair.b)
	cursors := map[string]int{}

	for i, before := range pair.a {
		list := positions[before.SHA256]
		pos := cursors[before.SHA256]
		if pos >= len(list) {
			continue
		}
		j := list[pos]
		cursors[before.SHA256] = pos + 1
		if j == i {
			continue
		}
		after := pair.b[j]
		location := sampleMediaLocation(before, pair.infoA)
		location.Description = fmt.Sprintf("identical %s moved from sample %d to copy sample %d", pair.infoA.Kind, before.Index, after.Index)
		rep.addRearrangement(Change{
			Layer:          LayerSample,
			Kind:           "sample_reordered",
			Message:        "normal content rearrangement; sample hash is unchanged",
			OriginalOffset: before.Offset,
			CopyOffset:     after.Offset,
			OriginalEnd:    before.Offset + before.Size,
			CopyEnd:        after.Offset + after.Size,
			OriginalHash:   before.SHA256,
			CopyHash:       after.SHA256,
			Media:          &location,
		})
	}
}

func sampleMediaLocation(sample SampleHash, track TrackInfo) MediaLocation {
	timescale := track.Timescale
	if timescale == 0 {
		timescale = 1
	}
	location := MediaLocation{
		TrackID:      sample.TrackID,
		TrackKind:    sample.TrackKind,
		SampleIndex:  sample.Index,
		DisplayOrder: sample.DisplayOrder,
		Sync:         sample.Sync,
		PTS:          sample.PTS,
		DTS:          sample.DTS,
		StartTime:    float64(sample.PTS) / float64(timescale),
		EndTime:      float64(sample.PTS+int64(sample.Duration)) / float64(timescale),
	}
	if track.TrackID != 0 && location.TrackID == 0 {
		location.TrackID = track.TrackID
	}
	if track.Kind != "" && location.TrackKind == "" {
		location.TrackKind = track.Kind
	}
	switch location.TrackKind {
	case "video":
		location.Description = fmt.Sprintf("video sample %d", sample.Index)
		if sample.DisplayOrder > 0 {
			location.Description += fmt.Sprintf(" / picture %d in display order", sample.DisplayOrder)
		}
	case "audio":
		location.Description = fmt.Sprintf("audio media sample %d", sample.Index)
	default:
		location.Description = fmt.Sprintf("media sample %d", sample.Index)
	}
	return location
}

func modifiedSampleChange(ar, br io.ReaderAt, before, after SampleHash, infoA, infoB TrackInfo) Change {
	location := sampleMediaLocation(before, infoA)
	change := Change{
		Layer:          LayerSample,
		Kind:           "sample_modified",
		Message:        fmt.Sprintf("%s hash changed", location.Description),
		OriginalOffset: before.Offset,
		CopyOffset:     after.Offset,
		OriginalEnd:    before.Offset + before.Size,
		CopyEnd:        after.Offset + after.Size,
		OriginalHash:   before.SHA256,
		CopyHash:       after.SHA256,
		Media:          &location,
	}
	if before.Size != after.Size {
		change.Kind = "sample_size_changed"
	}

	diff, err := firstRangeDifference(
		ar, before.Offset, before.Offset+before.Size,
		br, after.Offset, after.Offset+after.Size,
	)
	if err == nil && diff != nil {
		change.RelativeOffset = diff.Relative
		change.OriginalOffset = diff.OriginalOffset
		change.CopyOffset = diff.CopyOffset
		if diff.LengthMismatch {
			change.Message += fmt.Sprintf("; sample lengths differ (%d vs %d)", before.Size, after.Size)
		} else {
			change.Message += fmt.Sprintf("; first changed byte is %d bytes into sample", diff.Relative)
		}
		if change.Media != nil {
			change.Media.Description += fmt.Sprintf("; byte +%d", diff.Relative)
		}
	}
	return change
}

func addedSampleChange(after SampleHash, originalBoundary int64, info TrackInfo) Change {
	location := sampleMediaLocation(after, info)
	return Change{
		Layer:          LayerSample,
		Kind:           "sample_added",
		Message:        fmt.Sprintf("copy contains additional %s", location.Description),
		OriginalOffset: originalBoundary,
		CopyOffset:     after.Offset,
		OriginalEnd:    originalBoundary,
		CopyEnd:        after.Offset + after.Size,
		CopyHash:       after.SHA256,
		Media:          &location,
	}
}

func missingSampleChange(before SampleHash, copyBoundary int64, info TrackInfo) Change {
	location := sampleMediaLocation(before, info)
	return Change{
		Layer:          LayerSample,
		Kind:           "sample_missing",
		Message:        fmt.Sprintf("original %s is absent from copy", location.Description),
		OriginalOffset: before.Offset,
		CopyOffset:     copyBoundary,
		OriginalEnd:    before.Offset + before.Size,
		CopyEnd:        copyBoundary,
		OriginalHash:   before.SHA256,
		Media:          &location,
	}
}

func compareTrackFormats(a, b *Manifest, rep *Report) {
	infoB := trackInfoMap(b.Tracks)
	usedB := map[uint32]bool{}

	for _, trackA := range a.Tracks {
		idA := trackA.TrackID
		idB := matchTrackID(idA, trackA, b.Tracks, usedB)
		usedB[idB] = true
		trackB := infoB[idB]

		wavRateCoveredByFormat := trackA.Audio != nil && trackB.Audio != nil &&
			trackA.Timescale == trackA.Audio.SampleRate && trackB.Timescale == trackB.Audio.SampleRate
		if trackA.Timescale != 0 && trackB.Timescale != 0 && trackA.Timescale != trackB.Timescale && !wavRateCoveredByFormat {
			location := trackMediaLocation(trackA)
			rep.addMedia(Change{
				Layer:        LayerBox,
				Kind:         "timescale_changed",
				Message:      fmt.Sprintf("track %d timescale %d -> %d", idA, trackA.Timescale, trackB.Timescale),
				MetadataOnly: true,
				Media:        location,
			})
		}
		audioFormatChanged := !audioFormatEqual(trackA.Audio, trackB.Audio)
		if audioFormatChanged {
			location := trackMediaLocation(trackA)
			path := boxPathForType(a.Boxes, "fmt ")
			if path == "" {
				path = boxPathForType(a.Boxes, "mp4a")
			}
			change := Change{
				Layer:        LayerBox,
				BoxPath:      path,
				Kind:         "audio_format_changed",
				Message:      fmt.Sprintf("audio format changed: %+v -> %+v", trackA.Audio, trackB.Audio),
				MetadataOnly: true,
				Media:        location,
			}
			if boxA := findBoxByType(a.Boxes, "fmt "); boxA != nil {
				change.OriginalOffset = boxA.Start
				change.OriginalEnd = boxA.End
			}
			if boxB := findBoxByType(b.Boxes, "fmt "); boxB != nil {
				change.CopyOffset = boxB.Start
				change.CopyEnd = boxB.End
			}
			rep.addMedia(change)
		}
	}
}

func trackMediaLocation(track TrackInfo) *MediaLocation {
	return &MediaLocation{TrackID: track.TrackID, TrackKind: track.Kind, Description: track.Kind + " track format/timing"}
}

func audioFormatEqual(a, b *AudioFormat) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func boxPathForType(boxes []BoxHash, typ string) string {
	for _, box := range boxes {
		if box.Type == typ {
			return box.Path
		}
	}
	return ""
}

func findBoxByType(boxes []BoxHash, typ string) *BoxHash {
	for i := range boxes {
		if boxes[i].Type == typ {
			return &boxes[i]
		}
	}
	return nil
}
