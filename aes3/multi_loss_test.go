package aes3

import "testing"

func TestMultipleLossIntervalsStayIndependent(t *testing.T) {
	specs := testSpecs(8)
	clean := encodeTestStream(specs)
	removed := map[int]bool{4: true, 10: true}
	corrupt := make([]Edge, 0, len(clean))
	for _, edge := range clean {
		inRemoved := false
		for index := range removed {
			start := float64(index * 64)
			if edge.Time >= start && edge.Time < start+64 {
				inRemoved = true
				break
			}
		}
		if !inRemoved {
			corrupt = append(corrupt, edge)
		}
	}

	report, err := Decode(corrupt)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.LockIntervals) != len(removed) {
		t.Fatalf("intervals = %+v", report.LockIntervals)
	}
	for i, lost := range report.LockIntervals {
		wantStart := float64(([]int{4, 10})[i] * 64)
		if lost.StartTime != wantStart || lost.Duration != 64 || lost.RelockCode != "Y" {
			t.Fatalf("interval %d = %+v, want start %v duration64 relock Y", i, lost, wantStart)
		}
	}
	if len(report.Subframes) != len(specs)-2 {
		t.Fatalf("subframes = %d, want %d", len(report.Subframes), len(specs)-2)
	}

	wantIndices := []int{0, 1, 2, 3, 5, 6, 7, 8, 9, 11, 12, 13, 14, 15}
	for gotIndex, wantIndex := range wantIndices {
		got := report.Subframes[gotIndex]
		want := specs[wantIndex]
		if got.Audio.Sample24 != want.sample24 || got.Code != want.code || got.ChannelStatusBit != want.status {
			t.Fatalf("mapping report %d -> original %d failed: got %+v want %+v", gotIndex, wantIndex, got, want)
		}
	}

	if len(report.ChannelStatus.Left) != 3 {
		t.Fatalf("left status runs = %d, want before/between/after: %+v", len(report.ChannelStatus.Left), report.ChannelStatus.Left)
	}
}
