package hdrmeta_test

import (
	"fmt"

	"hdrmeta"
)

func ExampleStateMachine_Present() {
	machine := hdrmeta.NewMachine()

	machine.Ingest(hdrmeta.Metadata[hdrmeta.Params]{
		Scene: 10, FrameNumber: 0, PTS: 0, UpdatedAt: 100,
		Parameters: hdrmeta.Params{Label: "dark-scene", PeakNits: 120},
	})
	machine.Ingest(hdrmeta.Metadata[hdrmeta.Params]{
		Scene: 20, FrameNumber: 2, PTS: 2 * 3000, UpdatedAt: 102,
		Parameters: hdrmeta.Params{Label: "bright-scene", PeakNits: 1000},
	})

	for number := int64(0); number <= 2; number++ {
		record := machine.Present(hdrmeta.Frame{
			Number: number,
			PTS:    hdrmeta.Tick(number) * 3000,
		})
		fmt.Printf("frame=%d generation=%d scene=%d adoption=%s\n",
			record.Frame.Number,
			record.GenerationID,
			record.Generation.Scene,
			record.Adoption,
		)
	}

	// Output:
	// frame=0 generation=1 scene=10 adoption=new_generation
	// frame=1 generation=1 scene=10 adoption=held_generation
	// frame=2 generation=2 scene=20 adoption=new_generation
}
