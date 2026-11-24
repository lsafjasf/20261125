package aes3

type testSubframe struct {
	code      string
	sample24  int32
	validity  bool // raw V bit; false means audio is valid
	user      bool
	status    bool
	badParity bool
}

func encodeTestStream(specs []testSubframe) []Edge {
	const h = 1.0
	edges := []Edge{}
	level := LevelLow

	add := func(half int, newLevel Level) {
		edges = append(edges, Edge{Time: float64(half) * h, Level: newLevel})
		level = newLevel
	}

	for sfIndex, spec := range specs {
		base := sfIndex * 64
		p := preambleByCode(spec.code)
		want := p.states
		states := want
		if Level(want[0]) == level {
			complement := [8]int{}
			for i, b := range want {
				complement[i] = 1 - b
			}
			states = complement
		}

		// The preamble starts with a transition from the previous parity state.
		add(base, Level(states[0]))
		for k := 1; k < 8; k++ {
			if states[k] != states[k-1] {
				add(base+k, Level(states[k]))
			}
		}
		level = Level(states[7])

		bits := testSlotBits(spec)
		half := base + 8
		for _, bit := range bits {
			if level == LevelHigh {
				level = LevelLow
			} else {
				level = LevelHigh
			}
			add(half, level)
			half++
			if bit == 1 {
				if level == LevelHigh {
					level = LevelLow
				} else {
					level = LevelHigh
				}
				add(half, level)
			}
			half++
		}
	}

	// Add the final UI-boundary transition as a real transmitter would.
	total := len(specs) * 64
	final := LevelLow
	if level == LevelLow {
		final = LevelHigh
	}
	edges = append(edges, Edge{Time: float64(total), Level: final})
	return edges
}

func testSlotBits(spec testSubframe) []int {
	bits := make([]int, 28)
	raw := uint32(spec.sample24) & 0x00ffffff
	for i := 0; i < 24; i++ {
		bits[i] = int((raw >> uint(i)) & 1)
	}
	bits[24] = boolToInt(spec.validity)
	bits[25] = boolToInt(spec.user)
	bits[26] = boolToInt(spec.status)

	ones := 0
	for i := 0; i < 27; i++ {
		ones += bits[i]
	}
	bits[27] = ones % 2 // make slots 4..31 have even parity
	if spec.badParity {
		bits[27] ^= 1
	}
	return bits
}

func testSpecs(frames int) []testSubframe {
	specs := make([]testSubframe, 0, frames*2)
	for f := 0; f < frames; f++ {
		leftCode := "X"
		leftStatus := f%2 == 0
		if f == 0 {
			leftCode = "Z"
			leftStatus = true
		}
		specs = append(specs,
			testSubframe{code: leftCode, sample24: int32(1000 + f*17), status: leftStatus, user: f%3 == 0},
			testSubframe{code: "Y", sample24: int32(-2000 - f*19), status: f%2 == 1, user: f%4 == 0},
		)
	}
	return specs
}
