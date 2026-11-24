package aes3

import (
	"math"
	"testing"
)

func TestDecodeCleanStreamRecoversClockAndFields(t *testing.T) {
	specs := testSpecs(3)
	edges := encodeTestStream(specs)

	report, err := Decode(edges)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(report.HalfUnit-1) > 1e-9 {
		t.Fatalf("half unit = %v, want 1", report.HalfUnit)
	}
	if len(report.Subframes) != len(specs) {
		t.Fatalf("subframes = %d, want %d", len(report.Subframes), len(specs))
	}
	if len(report.LockIntervals) != 0 {
		t.Fatalf("unexpected lock intervals: %+v", report.LockIntervals)
	}

	for i, want := range specs {
		got := report.Subframes[i]
		if got.Code != want.code {
			t.Fatalf("sf %d code = %q, want %q", i, got.Code, want.code)
		}
		wantChannel := "left"
		if want.code == "Y" {
			wantChannel = "right"
		}
		if got.Channel != wantChannel {
			t.Fatalf("sf %d channel = %q, want %q", i, got.Channel, wantChannel)
		}
		if got.Audio.Sample24 != want.sample24 {
			t.Fatalf("sf %d sample = %d, want %d", i, got.Audio.Sample24, want.sample24)
		}
		if got.ChannelStatusBit != want.status {
			t.Fatalf("sf %d status = %v, want %v", i, got.ChannelStatusBit, want.status)
		}
		if got.ValidityBit != want.validity || !got.AudioValid {
			t.Fatalf("sf %d validity raw=%v valid=%v", i, got.ValidityBit, got.AudioValid)
		}
		if !got.ParityOK || !got.Trusted {
			t.Fatalf("sf %d should be trusted: parity=%v errors=%v", i, got.ParityOK, got.Errors)
		}
	}

	if report.Subframes[0].Frame != 0 || report.Subframes[1].Frame != 0 {
		t.Fatalf("first L/R should share frame 0: %+v %+v", report.Subframes[0].Frame, report.Subframes[1].Frame)
	}
	if report.Subframes[2].Frame != 1 || report.Subframes[3].Frame != 1 {
		t.Fatalf("second L/R should share frame 1")
	}
}

func TestLossIntervalRelockDoesNotPolluteChannels(t *testing.T) {
	specs := testSpecs(5)
	edges := encodeTestStream(specs)

	// Remove the complete BMC edge span of subframe 4 (the left sample in
	// audio frame 2). The following Y preamble is the relock boundary.
	corrupt := make([]Edge, 0, len(edges))
	for _, e := range edges {
		if e.Time >= 4*64 && e.Time < 5*64 {
			continue
		}
		corrupt = append(corrupt, e)
	}

	report, err := Decode(corrupt)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.LockIntervals) != 1 {
		t.Fatalf("lock intervals = %+v", report.LockIntervals)
	}
	lost := report.LockIntervals[0]
	if lost.StartTime != 4*64 || lost.EndTime != 5*64 {
		t.Fatalf("interval times = %v..%v, want 256..320", lost.StartTime, lost.EndTime)
	}
	if lost.EndEdge >= len(corrupt) || corrupt[lost.EndEdge].Time != 5*64 {
		t.Fatalf("relock edge index %d does not point at time 320", lost.EndEdge)
	}
	if lost.MissingSlots != 1 {
		t.Fatalf("missing slots = %d, want one subframe", lost.MissingSlots)
	}

	if len(report.Subframes) != len(specs)-1 {
		t.Fatalf("decoded %d subframes, want %d", len(report.Subframes), len(specs)-1)
	}

	// Four good subframes precede the loss.
	for i := 0; i < 4; i++ {
		if report.Subframes[i].Audio.Sample24 != specs[i].sample24 {
			t.Fatalf("before loss sf %d polluted", i)
		}
	}

	// Relock starts at original subframe 5 and every later value is intact.
	for gotIndex := 4; gotIndex < len(report.Subframes); gotIndex++ {
		wantIndex := gotIndex + 1
		got := report.Subframes[gotIndex]
		want := specs[wantIndex]
		if got.Code != want.code || got.ChannelStatusBit != want.status {
			t.Fatalf("after relock sf %d metadata = %+v, want code %s status %v", gotIndex, got, want.code, want.status)
		}
		if got.Audio.Sample24 != want.sample24 {
			t.Fatalf("after relock sf %d sample = %d, want %d", gotIndex, got.Audio.Sample24, want.sample24)
		}
	}

	if len(report.ChannelStatus.Left) != 2 {
		t.Fatalf("left status segments = %d, want split before/after loss: %+v", len(report.ChannelStatus.Left), report.ChannelStatus.Left)
	}
}

func TestParityBadBitIsIdentifiedButQuarantined(t *testing.T) {
	specs := testSpecs(3)
	specs[2].badParity = true // left of second frame
	edges := encodeTestStream(specs)

	report, err := Decode(edges)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Subframes) != 6 {
		t.Fatalf("subframes = %d", len(report.Subframes))
	}
	bad := report.Subframes[2]
	if bad.ParityOK || bad.Trusted {
		t.Fatalf("bad parity subframe not quarantined: %+v", bad)
	}
	if bad.Audio.Sample24 != specs[2].sample24 {
		t.Fatalf("raw fields should still decode: got %d", bad.Audio.Sample24)
	}

	// The bad left bit splits left status; right status remains one run.
	if len(report.ChannelStatus.Left) != 2 {
		t.Fatalf("left segments = %+v", report.ChannelStatus.Left)
	}
	if got := report.ChannelStatus.Left[1].StartSubframe; got != 4 {
		t.Fatalf("post-error left segment starts at %d, want 4", got)
	}
	if len(report.ChannelStatus.Right) != 1 {
		t.Fatalf("right should not be split by a left parity error: %+v", report.ChannelStatus.Right)
	}
	if len(report.ChannelStatus.Right[0].Bits) != 3 {
		t.Fatalf("right status bits = %v, want three contiguous bits", report.ChannelStatus.Right[0].Bits)
	}
}

func TestJitteredEdgesStillDecode(t *testing.T) {
	edges := encodeTestStream(testSpecs(2))
	for i := range edges {
		// Deterministic bounded jitter well inside the 0.5h decision margin.
		jitter := 0.06 * math.Sin(float64(i)*1.7)
		edges[i].Time += jitter
	}

	report, err := Decode(edges)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Subframes) != 4 || len(report.LockIntervals) != 0 {
		t.Fatalf("jitter decode sf=%d intervals=%+v", len(report.Subframes), report.LockIntervals)
	}
	for _, sf := range report.Subframes {
		if !sf.ParityOK {
			t.Fatalf("jitter caused parity failure: %+v", sf)
		}
	}
}

func TestSpuriousEdgesDefineLossInterval(t *testing.T) {
	clean := encodeTestStream(testSpecs(5))
	corrupt := make([]Edge, 0, len(clean)+3)
	for _, edge := range clean {
		if edge.Time >= 4*64 && edge.Time < 5*64 {
			continue
		}
		corrupt = append(corrupt, edge)
		if edge.Time == 192 {
			corrupt = append(corrupt,
				Edge{Time: 257.25, Level: LevelHigh},
				Edge{Time: 258.10, Level: LevelLow},
				Edge{Time: 261.75, Level: LevelHigh},
			)
		}
	}
	corrupt = prepareEdges(corrupt)

	report, err := Decode(corrupt)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.LockIntervals) != 1 {
		t.Fatalf("intervals = %+v", report.LockIntervals)
	}
	interval := report.LockIntervals[0]
	if got := corrupt[interval.StartEdge].Time; got != 257.25 {
		t.Fatalf("start edge time = %v, want first spurious edge 257.25", got)
	}
	if interval.EndEdge >= len(corrupt) || corrupt[interval.EndEdge].Time < 319 || corrupt[interval.EndEdge].Time > 321 {
		t.Fatalf("end edge %d is not relock near 320: %+v", interval.EndEdge, corrupt)
	}
	if len(report.Subframes) != 9 {
		t.Fatalf("subframes = %d, want good pre/post runs only", len(report.Subframes))
	}
}
