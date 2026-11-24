package hdrmeta

import (
	"reflect"
	"testing"
)

const testFrameDuration Tick = 3_000 // 30 fps on the 90 kHz PTS clock

func testPTS(frame int64) Tick {
	return Tick(frame) * testFrameDuration
}

func testFrames(numbers ...int64) []Frame {
	frames := make([]Frame, len(numbers))
	for i, number := range numbers {
		frames[i] = Frame{Number: number, PTS: testPTS(number)}
	}
	return frames
}

func testParams(label string, peakNits int) Params {
	return Params{Label: label, PeakNits: peakNits, AverageNits: peakNits / 4, ToneCurveID: 7}
}

func adoptionIDs(records []FrameRecord[Params]) []int64 {
	ids := make([]int64, len(records))
	for i, record := range records {
		ids[i] = record.GenerationID
	}
	return ids
}

func TestMetadataAtUsesDisplayPTSAndNeverFutureScene(t *testing.T) {
	machine := NewMachine()

	updates := []Metadata[Params]{
		{Scene: 100, FrameNumber: 0, PTS: testPTS(0), UpdatedAt: 1_000, Parameters: testParams("scene-100-frame-0", 100)},
		{Scene: 100, FrameNumber: 1, PTS: testPTS(1), UpdatedAt: 1_001, Parameters: testParams("scene-100-frame-1", 120)},
		// The next scene has already arrived, but its activation PTS is frame 3.
		{Scene: 200, FrameNumber: 3, PTS: testPTS(3), UpdatedAt: 1_003, Parameters: testParams("scene-200-frame-3", 1_000)},
	}
	for _, update := range updates {
		machine.Ingest(update)
	}

	checks := []struct {
		at        Tick
		wantID    int64
		wantScene int64
	}{
		{testPTS(0), 1, 100},
		{testPTS(1), 2, 100},
		{testPTS(2), 2, 100}, // no block for frame 2: hold the last eligible generation
		{testPTS(3), 3, 200},
	}
	for _, check := range checks {
		snapshot := machine.MetadataAt(check.at)
		if !snapshot.Found || snapshot.Generation.ID != check.wantID {
			t.Fatalf("MetadataAt(%d) = %+v, want generation %d", check.at, snapshot, check.wantID)
		}
		if snapshot.Generation.Scene != check.wantScene {
			t.Fatalf("MetadataAt(%d) scene = %d, want %d", check.at, snapshot.Generation.Scene, check.wantScene)
		}
	}

	records := machine.PresentAll(testFrames(0, 1, 2, 3))
	if got := adoptionIDs(records); !reflect.DeepEqual(got, []int64{1, 2, 2, 3}) {
		t.Fatalf("adopted generations = %v, want [1 2 2 3]", got)
	}

	wantAdoption := []Adoption{AdoptionNew, AdoptionNew, AdoptionHeld, AdoptionNew}
	for i, want := range wantAdoption {
		if records[i].Adoption != want {
			t.Fatalf("frame %d adoption = %q, want %q", i, records[i].Adoption, want)
		}
	}
}

func TestMissingFrameNumbersAreRecordedAndGenerationHeldAcrossGap(t *testing.T) {
	machine := NewMachine()
	machine.Ingest(Metadata[Params]{
		Scene: 7, FrameNumber: 0, PTS: testPTS(0), UpdatedAt: 500,
		Parameters: testParams("before-gap", 110),
	})
	machine.Ingest(Metadata[Params]{
		Scene: 8, FrameNumber: 5, PTS: testPTS(5), UpdatedAt: 505,
		Parameters: testParams("after-gap", 900),
	})

	records := machine.PresentAll(testFrames(0, 1, 2, 5))
	if got := adoptionIDs(records); !reflect.DeepEqual(got, []int64{1, 1, 1, 2}) {
		t.Fatalf("adopted generations = %v, want [1 1 1 2]", got)
	}

	gap := records[3].Gap
	if gap.Count != 2 || gap.From != 3 || gap.To != 4 {
		t.Fatalf("gap = %+v, want frames 3..4", gap)
	}
	if got := gap.Numbers(); !reflect.DeepEqual(got, []int64{3, 4}) {
		t.Fatalf("gap numbers = %v, want [3 4]", got)
	}
	if records[3].Adoption != AdoptionNew {
		t.Fatalf("frame after gap adoption = %q, want new", records[3].Adoption)
	}
	if records[1].Adoption != AdoptionHeld || records[2].Adoption != AdoptionHeld {
		t.Fatalf("pre-gap holds = %q/%q, want held", records[1].Adoption, records[2].Adoption)
	}
}

func TestFutureOnlyMetadataDoesNotBecomeInitialGeneration(t *testing.T) {
	machine := NewMachine()
	machine.Ingest(Metadata[Params]{Scene: 9, PTS: testPTS(4), UpdatedAt: 9, Parameters: testParams("future", 800)})

	snapshot := machine.MetadataAt(testPTS(0))
	if snapshot.Found || snapshot.State != StateWaiting {
		t.Fatalf("snapshot at frame 0 = %+v, want waiting and not found", snapshot)
	}

	record := machine.Present(Frame{Number: 0, PTS: testPTS(0)})
	if record.GenerationID != 0 || record.Found || record.State != StateWaiting {
		t.Fatalf("early frame adopted future metadata: %+v", record)
	}
	if len(machine.History()) != 1 || machine.History()[0].GenerationID != 0 {
		t.Fatalf("audit history was not retained correctly: %+v", machine.History())
	}
}
