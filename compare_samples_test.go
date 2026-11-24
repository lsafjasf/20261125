package main

import (
	"bytes"
	"testing"
)

func sampleWithHash(index int, hash string) SampleHash {
	return SampleHash{Key: hash, TrackID: 1, TrackKind: "video", Index: index, SHA256: hash}
}

func TestCompareSamplePairInsertionAlignment(t *testing.T) {
	pair := sampleTrackPair{
		infoA: TrackInfo{TrackID: 1, Kind: "video", Timescale: 1000},
		infoB: TrackInfo{TrackID: 1, Kind: "video", Timescale: 1000},
		a:     []SampleHash{sampleWithHash(1, "a"), sampleWithHash(2, "b"), sampleWithHash(3, "c")},
		b:     []SampleHash{sampleWithHash(1, "a"), sampleWithHash(2, "x"), sampleWithHash(3, "b"), sampleWithHash(4, "c")},
	}
	rep := &Report{}
	compareSamplePair(bytes.NewReader(nil), bytes.NewReader(nil), 0, 0, pair, rep)

	if len(rep.Changes) != 1 {
		t.Fatalf("media changes = %d, want one added sample: %+v", len(rep.Changes), rep.Changes)
	}
	if rep.Changes[0].Kind != "sample_added" || rep.Changes[0].Media.SampleIndex != 2 {
		t.Fatalf("unexpected change: %+v", rep.Changes[0])
	}
}

func TestCompareSamplePairDeletionAlignment(t *testing.T) {
	pair := sampleTrackPair{
		infoA: TrackInfo{TrackID: 1, Kind: "audio"},
		infoB: TrackInfo{TrackID: 1, Kind: "audio"},
		a:     []SampleHash{sampleWithHash(1, "a"), sampleWithHash(2, "x"), sampleWithHash(3, "b")},
		b:     []SampleHash{sampleWithHash(1, "a"), sampleWithHash(2, "b")},
	}
	rep := &Report{}
	compareSamplePair(bytes.NewReader(nil), bytes.NewReader(nil), 0, 0, pair, rep)

	if len(rep.Changes) != 1 {
		t.Fatalf("media changes = %d, want one missing sample: %+v", len(rep.Changes), rep.Changes)
	}
	if rep.Changes[0].Kind != "sample_missing" || rep.Changes[0].Media.SampleIndex != 2 {
		t.Fatalf("unexpected change: %+v", rep.Changes[0])
	}
}

func TestCompareSamplePairPermutationIsRearrangement(t *testing.T) {
	pair := sampleTrackPair{
		infoA: TrackInfo{TrackID: 1, Kind: "video"},
		infoB: TrackInfo{TrackID: 1, Kind: "video"},
		a:     []SampleHash{sampleWithHash(1, "a"), sampleWithHash(2, "b"), sampleWithHash(3, "c")},
		b:     []SampleHash{sampleWithHash(1, "c"), sampleWithHash(2, "a"), sampleWithHash(3, "b")},
	}
	rep := &Report{}
	compareSamplePair(bytes.NewReader(nil), bytes.NewReader(nil), 0, 0, pair, rep)

	if len(rep.Changes) != 0 {
		t.Fatalf("permutation should not be media corruption: %+v", rep.Changes)
	}
	if len(rep.Rearrangements) == 0 {
		t.Fatal("expected rearrangement records")
	}
}
