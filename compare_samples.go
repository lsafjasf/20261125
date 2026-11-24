package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type sampleTrackPair struct {
	infoA TrackInfo
	infoB TrackInfo
	a     []SampleHash
	b     []SampleHash
}

func compareTrackSamples(a, b *Archive, rep *Report) {
	groupsA := groupSamplesByTrack(a.Manifest.Samples)
	groupsB := groupSamplesByTrack(b.Manifest.Samples)
	infoA := trackInfoMap(a.Manifest.Tracks)
	infoB := trackInfoMap(b.Manifest.Tracks)
	usedB := map[uint32]bool{}

	for _, trackA := range a.Manifest.Tracks {
		trackID := trackA.TrackID
		samplesA := groupsA[trackID]
		if samplesA == nil {
			continue
		}
		idB := matchSampleTrackID(trackID, infoA[trackID], samplesA, b.Manifest.Tracks, groupsB, usedB)
		usedB[idB] = true
		samplesB := groupsB[idB]
		pair := sampleTrackPair{infoA: infoA[trackID], infoB: infoB[idB], a: samplesA, b: samplesB}
		compareSamplePair(a.Reader, b.Reader, a.Size, b.Size, pair, rep)
	}

	for _, trackB := range b.Manifest.Tracks {
		trackID := trackB.TrackID
		if usedB[trackID] || groupsB[trackID] == nil {
			continue
		}
		usedB[trackID] = true
		pair := sampleTrackPair{infoB: infoB[trackID], b: groupsB[trackID]}
		compareSamplePair(a.Reader, b.Reader, a.Size, b.Size, pair, rep)
	}
}

func groupSamplesByTrack(samples []SampleHash) map[uint32][]SampleHash {
	out := map[uint32][]SampleHash{}
	for _, sample := range samples {
		out[sample.TrackID] = append(out[sample.TrackID], sample)
	}
	return out
}

func trackInfoMap(tracks []TrackInfo) map[uint32]TrackInfo {
	out := map[uint32]TrackInfo{}
	for _, track := range tracks {
		out[track.TrackID] = track
	}
	return out
}

func matchTrackID(idA uint32, infoA TrackInfo, tracksB []TrackInfo, used map[uint32]bool) uint32 {
	for _, track := range tracksB {
		if track.TrackID == idA {
			return track.TrackID
		}
	}
	for _, track := range tracksB {
		if used[track.TrackID] {
			continue
		}
		if infoA.Kind != "" && infoA.Kind != "unknown" && track.Kind == infoA.Kind {
			return track.TrackID
		}
	}
	for _, track := range tracksB {
		if !used[track.TrackID] {
			return track.TrackID
		}
	}
	return idA
}

func matchSampleTrackID(idA uint32, infoA TrackInfo, samplesA []SampleHash, tracksB []TrackInfo, groupsB map[uint32][]SampleHash, used map[uint32]bool) uint32 {
	for _, track := range tracksB {
		if track.TrackID == idA {
			return track.TrackID
		}
	}
	for _, track := range tracksB {
		if used[track.TrackID] || (infoA.Kind != "" && infoA.Kind != "unknown" && track.Kind != infoA.Kind) {
			continue
		}
		if orderedSampleHashesEqual(samplesA, groupsB[track.TrackID]) {
			return track.TrackID
		}
	}
	for _, track := range tracksB {
		if used[track.TrackID] || (infoA.Kind != "" && infoA.Kind != "unknown" && track.Kind != infoA.Kind) {
			continue
		}
		if sampleMultisetEqual(samplesA, groupsB[track.TrackID]) {
			return track.TrackID
		}
	}
	return matchTrackID(idA, infoA, tracksB, used)
}

func compareSamplePair(ar, br io.ReaderAt, sizeA, sizeB int64, pair sampleTrackPair, rep *Report) {
	sa, sb := pair.a, pair.b
	if orderedSampleHashesEqual(sa, sb) {
		compareSampleMetadata(pair, rep)
		return
	}

	if sampleMultisetEqual(sa, sb) {
		reportSampleRearrangement(pair, rep)
		return
	}

	posA := hashPositionMap(sa)
	posB := hashPositionMap(sb)
	i, j := 0, 0
	for i < len(sa) || j < len(sb) {
		if i < len(sa) && j < len(sb) && sa[i].SHA256 == sb[j].SHA256 {
			i++
			j++
			continue
		}

		if i >= len(sa) {
			for ; j < len(sb); j++ {
				rep.addMedia(addedSampleChange(sb[j], sizeA, pair.infoB))
			}
			return
		}
		if j >= len(sb) {
			for ; i < len(sa); i++ {
				rep.addMedia(missingSampleChange(sa[i], sizeB, pair.infoA))
			}
			return
		}

		nextB := nextPosition(posB, sa[i].SHA256, j+1)
		nextA := nextPosition(posA, sb[j].SHA256, i+1)
		switch {
		case nextB >= 0 && (nextA < 0 || nextB-j <= nextA-i):
			for ; j < nextB; j++ {
				rep.addMedia(addedSampleChange(sb[j], sa[i].Offset, pair.infoB))
			}
		case nextA >= 0:
			for ; i < nextA; i++ {
				rep.addMedia(missingSampleChange(sa[i], sb[j].Offset, pair.infoA))
			}
		default:
			rep.addMedia(modifiedSampleChange(ar, br, sa[i], sb[j], pair.infoA, pair.infoB))
			i++
			j++
		}
	}
}

func compareSampleMetadata(pair sampleTrackPair, rep *Report) {
	n := len(pair.a)
	if len(pair.b) < n {
		n = len(pair.b)
	}
	for i := 0; i < n; i++ {
		before, after := pair.a[i], pair.b[i]
		var differences []string
		if before.DTS != after.DTS {
			differences = append(differences, fmt.Sprintf("DTS %d->%d", before.DTS, after.DTS))
		}
		if before.PTS != after.PTS {
			differences = append(differences, fmt.Sprintf("PTS %d->%d", before.PTS, after.PTS))
		}
		if before.Duration != after.Duration {
			differences = append(differences, fmt.Sprintf("duration %d->%d", before.Duration, after.Duration))
		}
		if before.Sync != after.Sync {
			differences = append(differences, fmt.Sprintf("sync %v->%v", before.Sync, after.Sync))
		}
		if len(differences) == 0 {
			continue
		}
		location := sampleMediaLocation(before, pair.infoA)
		location.Description += "; timing/sync metadata changed"
		rep.addMedia(Change{
			Layer:        LayerSample,
			Kind:         "sample_metadata_changed",
			Message:      strings.Join(differences, "; "),
			MetadataOnly: true,
			Media:        &location,
		})
	}
}

func orderedSampleHashesEqual(a, b []SampleHash) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].SHA256 != b[i].SHA256 {
			return false
		}
	}
	return true
}

func sampleMultisetEqual(a, b []SampleHash) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, sample := range a {
		counts[sample.SHA256]++
	}
	for _, sample := range b {
		counts[sample.SHA256]--
		if counts[sample.SHA256] < 0 {
			return false
		}
	}
	return true
}

func hashPositionMap(samples []SampleHash) map[string][]int {
	out := map[string][]int{}
	for i, sample := range samples {
		out[sample.SHA256] = append(out[sample.SHA256], i)
	}
	return out
}

func nextPosition(positions map[string][]int, hash string, min int) int {
	list := positions[hash]
	idx := sort.Search(len(list), func(k int) bool { return list[k] >= min })
	if idx == len(list) {
		return -1
	}
	return list[idx]
}
