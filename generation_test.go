package hdrmeta

import (
	"fmt"
	"testing"
)

func TestDuplicateSceneNumbersRemainIndependentGenerations(t *testing.T) {
	machine := NewMachine()
	updates := []Metadata[Params]{
		{Scene: 42, FrameNumber: 0, PTS: testPTS(0), UpdatedAt: 700, Parameters: testParams("scene-42-a", 100)},
		{Scene: 43, FrameNumber: 1, PTS: testPTS(1), UpdatedAt: 701, Parameters: testParams("scene-43", 200)},
		{Scene: 42, FrameNumber: 2, PTS: testPTS(2), UpdatedAt: 702, Parameters: testParams("scene-42-b", 300)},
		{Scene: 44, FrameNumber: 3, PTS: testPTS(3), UpdatedAt: 703, Parameters: testParams("scene-44", 400)},
		{Scene: 42, FrameNumber: 4, PTS: testPTS(4), UpdatedAt: 704, Parameters: testParams("scene-42-c", 500)},
	}
	for _, update := range updates {
		machine.Ingest(update)
	}

	records := machine.PresentAll(testFrames(0, 1, 2, 3, 4))
	wantScenes := []int64{42, 43, 42, 44, 42}
	for i, record := range records {
		if record.GenerationID != int64(i+1) {
			t.Fatalf("frame %d generation = %d, want %d", i, record.GenerationID, i+1)
		}
		if record.Generation.Scene != wantScenes[i] {
			t.Fatalf("frame %d scene = %d, want %d", i, record.Generation.Scene, wantScenes[i])
		}
		if record.Generation.UpdatedAt != int64(700+i) {
			t.Fatalf("frame %d update timestamp = %d, want %d", i, record.Generation.UpdatedAt, 700+i)
		}
	}

	duplicate := machine.Ingest(updates[0])
	if !duplicate.Duplicate || duplicate.Generation.ID != 1 {
		t.Fatalf("exact reingest = %+v, want duplicate generation 1", duplicate)
	}
	if len(machine.Generations()) != len(updates) {
		t.Fatalf("exact duplicate added a generation: %d generations", len(machine.Generations()))
	}

	corrected := updates[2]
	corrected.Parameters.Label = "scene-42-b-fixed"
	result := machine.Ingest(corrected)
	if !result.Corrected || result.Generation.ID != int64(len(updates)+1) {
		t.Fatalf("correction = %+v, want corrected generation 6", result)
	}
	snapshot := machine.MetadataAt(testPTS(2))
	if snapshot.GenerationID != 6 || snapshot.Generation.Parameters.Label != "scene-42-b-fixed" {
		t.Fatalf("PTS 2 selected %+v, want correction generation 6", snapshot)
	}
}

func TestConstantSceneWithContinuousTimestampsAdvancesGenerations(t *testing.T) {
	machine := NewMachine()
	for i := int64(0); i < 5; i++ {
		machine.Ingest(Metadata[Params]{
			Scene:       51,
			FrameNumber: i,
			PTS:         testPTS(i),
			UpdatedAt:   9_000 + i,
			Parameters:  testParams(fmt.Sprintf("continuous-%d", i), 200+int(i)),
		})
	}

	records := machine.PresentAll(testFrames(0, 1, 2, 3, 4))
	for i, record := range records {
		if record.GenerationID != int64(i+1) || record.Generation.Scene != 51 {
			t.Fatalf("frame %d = gen %d scene %d, want generation %d and scene 51",
				i, record.GenerationID, record.Generation.Scene, i+1)
		}
		if record.Generation.UpdatedAt != 9_000+int64(i) {
			t.Fatalf("frame %d update timestamp = %d", i, record.Generation.UpdatedAt)
		}
		if record.Adoption != AdoptionNew {
			t.Fatalf("frame %d adoption = %q, want new continuous generation", i, record.Adoption)
		}
	}
}

func TestOutOfOrderIngestStillIndexesByDisplayPTS(t *testing.T) {
	ordered := []Metadata[Params]{
		{Scene: 1, FrameNumber: 0, PTS: testPTS(0), UpdatedAt: 800, Parameters: testParams("p0", 100)},
		{Scene: 1, FrameNumber: 1, PTS: testPTS(1), UpdatedAt: 801, Parameters: testParams("p1", 110)},
		{Scene: 2, FrameNumber: 2, PTS: testPTS(2), UpdatedAt: 802, Parameters: testParams("p2", 200)},
		{Scene: 2, FrameNumber: 3, PTS: testPTS(3), UpdatedAt: 803, Parameters: testParams("p3", 210)},
	}

	machine := NewMachine()
	for _, index := range []int{3, 0, 2, 1} {
		machine.Ingest(ordered[index])
	}

	for frame := int64(0); frame < 4; frame++ {
		snapshot := machine.MetadataAt(testPTS(frame))
		if !snapshot.Found {
			t.Fatalf("frame %d metadata not found", frame)
		}
		if snapshot.Generation.UpdatedAt != 800+frame {
			t.Fatalf("frame %d selected update timestamp %d, want %d", frame, snapshot.Generation.UpdatedAt, 800+frame)
		}
		if snapshot.Generation.FrameNumber != frame {
			t.Fatalf("frame %d selected metadata frame %d", frame, snapshot.Generation.FrameNumber)
		}
	}
}

func TestLateMetadataDoesNotRewriteActualFrameAdoption(t *testing.T) {
	machine := NewMachine()

	early := machine.Present(Frame{Number: 0, PTS: testPTS(0)})
	if early.Found || early.GenerationID != 0 || early.Adoption != AdoptionNone {
		t.Fatalf("early record = %+v, want no adopted generation", early)
	}

	machine.Ingest(Metadata[Params]{
		Scene: 3, FrameNumber: 0, PTS: testPTS(0), UpdatedAt: 900,
		Parameters: testParams("late", 300),
	})

	history := machine.History()
	if history[0].Found || history[0].GenerationID != 0 {
		t.Fatalf("late ingest rewrote actual history: %+v", history[0])
	}

	next := machine.Present(Frame{Number: 1, PTS: testPTS(1)})
	if next.GenerationID != 1 || next.Adoption != AdoptionNew {
		t.Fatalf("first adoption after late ingest = %+v, want new generation 1", next)
	}

	held := machine.Present(Frame{Number: 2, PTS: testPTS(2)})
	if held.GenerationID != 1 || held.Adoption != AdoptionHeld {
		t.Fatalf("subsequent frame = %+v, want held generation 1", held)
	}
}
