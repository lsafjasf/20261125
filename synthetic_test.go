package hdrmeta

import (
	"fmt"
	"reflect"
	"testing"
)

// syntheticHDRMaterial builds an in-memory test stream. Presented frame 6 has
// no metadata block, and frame numbers 4 and 7 are absent entirely. Scene 77
// is reused after an intervening scene, while source update timestamps remain
// continuous.
func syntheticHDRMaterial() ([]Metadata[Params], []Frame, []int64) {
	presented := []int64{0, 1, 2, 3, 5, 6, 8, 9}
	metadataFrames := []int64{0, 1, 2, 3, 5, 8, 9}
	scenes := map[int64]int64{
		0: 77, 1: 77, 2: 77,
		3: 88, 5: 88,
		8: 77, 9: 77,
	}

	updates := make([]Metadata[Params], 0, len(metadataFrames))
	for i, frameNumber := range metadataFrames {
		updates = append(updates, Metadata[Params]{
			Scene:       scenes[frameNumber],
			FrameNumber: frameNumber,
			PTS:         testPTS(frameNumber),
			UpdatedAt:   42_001 + int64(i),
			Parameters: testParams(
				fmt.Sprintf("scene-%d-update-%d", scenes[frameNumber], i+1),
				100+50*i,
			),
		})
	}

	return updates, testFrames(presented...), []int64{1, 2, 3, 4, 5, 5, 6, 7}
}

func TestCodeConstructedSyntheticStreamKeepsActualGenerations(t *testing.T) {
	updates, frames, wantIDs := syntheticHDRMaterial()
	machine := ResolveStream(updates, frames)
	records := machine.History()

	if got := adoptionIDs(records); !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("adopted generations = %v, want %v", got, wantIDs)
	}

	wantScenes := []int64{77, 77, 77, 88, 88, 88, 77, 77}
	for i, record := range records {
		if record.Generation.Scene != wantScenes[i] {
			t.Fatalf("presented frame %d scene = %d, want %d", record.Frame.Number, record.Generation.Scene, wantScenes[i])
		}
	}

	if records[4].Gap.Count != 1 || records[4].Gap.From != 4 || records[4].Gap.To != 4 {
		t.Fatalf("first gap = %+v, want missing frame 4", records[4].Gap)
	}
	if records[6].Gap.Count != 1 || records[6].Gap.From != 7 || records[6].Gap.To != 7 {
		t.Fatalf("second gap = %+v, want missing frame 7", records[6].Gap)
	}
	if records[5].Adoption != AdoptionHeld || records[5].Generation.FrameNumber != 5 {
		t.Fatalf("metadata-less frame 6 = %+v, want hold frame-5 generation", records[5])
	}

	generations := machine.Generations()
	for i, generation := range generations {
		if want := int64(42_001 + i); generation.UpdatedAt != want {
			t.Fatalf("generation %d update timestamp = %d, want continuous %d", generation.ID, generation.UpdatedAt, want)
		}
	}

	if state := machine.State(); state != StateActive {
		t.Fatalf("final state = %q, want active", state)
	}
}
