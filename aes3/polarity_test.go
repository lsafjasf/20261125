package aes3

import "testing"

func TestTimestampOnlyEdgesArePolarityIndependent(t *testing.T) {
	edges := encodeTestStream(testSpecs(3))
	for i := range edges {
		edges[i].Level = LevelUnknown
	}

	report, err := DecodeTimestamps(timesOnly(edges))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Subframes) != 6 || len(report.LockIntervals) != 0 {
		t.Fatalf("sf=%d intervals=%+v", len(report.Subframes), report.LockIntervals)
	}
	for i, sf := range report.Subframes {
		if sf.Audio.Sample24 != testSpecs(3)[i].sample24 {
			t.Fatalf("timestamp-only sf %d sample mismatch", i)
		}
	}
}

func timesOnly(edges []Edge) []float64 {
	times := make([]float64, len(edges))
	for i, edge := range edges {
		times[i] = edge.Time
	}
	return times
}
