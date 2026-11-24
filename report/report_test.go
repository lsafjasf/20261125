package report

import (
	"bytes"
	"testing"

	"trackmeta/mp4"
	"trackmeta/testutil"
)

func buildMixedFile(t *testing.T) []byte {
	t.Helper()
	return testutil.BuildMixedFile()
}

func parse(t *testing.T, data []byte) *Report {
	t.Helper()
	tracks, err := mp4.ParseFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return Build("mixed.mp4", tracks)
}

func issuesFor(rep *Report, trackIndex int, code string) []Issue {
	var out []Issue
	for _, iss := range rep.Inconsistencies {
		if iss.Code == code && (trackIndex < 0 || iss.TrackIndex == trackIndex) {
			out = append(out, iss)
		}
	}
	return out
}

func TestTrackCountAndClassification(t *testing.T) {
	rep := parse(t, buildMixedFile(t))
	if rep.VideoTrackCount != 3 {
		t.Fatalf("want 3 video tracks, got %d", rep.VideoTrackCount)
	}
	want := []string{"SDR", "HDR", "HDR"}
	for i, tr := range rep.Tracks {
		if tr.Classification != want[i] {
			t.Errorf("track %d: want %s, got %s", i, want[i], tr.Classification)
		}
	}
}

func TestBitstreamColorExtracted(t *testing.T) {
	rep := parse(t, buildMixedFile(t))
	tr2 := rep.Tracks[1]
	bs := tr2.Samples[0].BitstreamColor
	if bs == nil {
		t.Fatal("track 2: no bitstream color extracted from hvcC")
	}
	if bs.Primaries != 9 || bs.Transfer != 16 || bs.Matrix != 9 || !bs.FullRange {
		t.Errorf("track 2 bitstream color wrong: %+v", bs)
	}
	if bs.TransferName != "PQ (SMPTE ST 2084)" {
		t.Errorf("unexpected transfer name %q", bs.TransferName)
	}
}

func TestMissingColrFlagged(t *testing.T) {
	rep := parse(t, buildMixedFile(t))
	if got := issuesFor(rep, 1, "missing-colr"); len(got) != 1 {
		t.Errorf("want 1 missing-colr on track 2, got %d", len(got))
	}
	if got := issuesFor(rep, 0, "missing-colr"); len(got) != 0 {
		t.Errorf("track 1 must not be flagged missing-colr")
	}
}

func TestColrVsBitstreamMismatch(t *testing.T) {
	rep := parse(t, buildMixedFile(t))
	mismatches := issuesFor(rep, 2, "colr-vs-bitstream")
	if len(mismatches) != 3 { // primaries, transfer, matrix
		t.Errorf("want 3 field mismatches on track 3, got %d", len(mismatches))
	}
	if got := issuesFor(rep, 2, "range-mismatch"); len(got) != 1 {
		t.Errorf("want 1 range-mismatch on track 3, got %d", len(got))
	}
	if got := issuesFor(rep, 0, "colr-vs-bitstream"); len(got) != 0 {
		t.Errorf("consistent track 1 must not be flagged")
	}
}

func TestMixAndFirstTrackTag(t *testing.T) {
	rep := parse(t, buildMixedFile(t))
	if got := issuesFor(rep, -1, "hdr-sdr-mix"); len(got) != 1 {
		t.Errorf("want hdr-sdr-mix info, got %d", len(got))
	}
	if got := issuesFor(rep, 1, "first-track-tag-mismatch"); len(got) != 1 {
		t.Errorf("want first-track-tag-mismatch on track 2, got %d", len(got))
	}
}
