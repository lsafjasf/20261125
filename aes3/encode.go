package aes3

// SubframeInput is a simple encoder input. It is useful for fixtures and
// round-trip tests; Decode is the production path.
type SubframeInput struct {
	Code                 string
	Sample24             int32
	ValidityBit          bool
	UserBit              bool
	ChannelStatusBit     bool
	ParityBit            *bool
	IntroduceParityError bool
}

// EncodeEdges encodes complete subframes as polarity-explicit BMC edges.
// The returned time coordinate uses one half-bit unit (UI/2) as 1.
func EncodeEdges(subframes []SubframeInput) []Edge {
	const h = 1.0
	edges := []Edge{}
	level := LevelLow

	add := func(half int, next Level) {
		edges = append(edges, Edge{Time: float64(half) * h, Level: next})
		level = next
	}

	for sfIndex, spec := range subframes {
		base := sfIndex * 64
		p := preambleByCode(spec.Code)
		if p == nil {
			p = preambleByCode("X")
		}
		states := p.states
		if Level(states[0]) == level {
			complement := [8]int{}
			for i, b := range p.states {
				complement[i] = 1 - b
			}
			states = complement
		}

		add(base, Level(states[0]))
		for k := 1; k < 8; k++ {
			if states[k] != states[k-1] {
				add(base+k, Level(states[k]))
			}
		}
		level = Level(states[7])

		bits := inputSlotBits(spec)
		half := base + 8
		for _, bit := range bits {
			level = toggleLevel(level)
			add(half, level)
			half++
			if bit == 1 {
				level = toggleLevel(level)
				add(half, level)
			}
			half++
		}
	}

	total := len(subframes) * 64
	edges = append(edges, Edge{Time: float64(total), Level: toggleLevel(level)})
	return edges
}

func toggleLevel(level Level) Level {
	if level == LevelHigh {
		return LevelLow
	}
	return LevelHigh
}

func inputSlotBits(spec SubframeInput) []int {
	bits := make([]int, 28)
	raw := uint32(spec.Sample24) & 0x00ffffff
	for i := 0; i < 24; i++ {
		bits[i] = int((raw >> uint(i)) & 1)
	}
	bits[24] = boolToInt(spec.ValidityBit)
	bits[25] = boolToInt(spec.UserBit)
	bits[26] = boolToInt(spec.ChannelStatusBit)

	if spec.ParityBit != nil {
		bits[27] = boolToInt(*spec.ParityBit)
	} else {
		ones := 0
		for i := 0; i < 27; i++ {
			ones += bits[i]
		}
		bits[27] = ones % 2
	}
	if spec.IntroduceParityError {
		bits[27] ^= 1
	}
	return bits
}
