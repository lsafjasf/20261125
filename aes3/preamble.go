package aes3

type preamble struct {
	code    string
	channel string
	block   bool
	states  [8]int
}

// Eight line states occupy the four preamble time slots. Both BMC polarities
// are accepted. X/Z identify sub-frame 1 (left/channel A); Y identifies
// sub-frame 2 (right/channel B).
var preambles = []preamble{
	{code: "X", channel: "left", states: [8]int{1, 1, 1, 0, 0, 0, 1, 0}},
	{code: "Y", channel: "right", states: [8]int{1, 1, 1, 0, 0, 1, 0, 0}},
	{code: "Z", channel: "left", block: true, states: [8]int{1, 1, 1, 0, 1, 0, 0, 0}},
}

func matchPreamble(states [8]Level) *preamble {
	for i := range preambles {
		want := preambles[i].states
		same, complement := true, true
		for k := 0; k < 8; k++ {
			got := int(states[k])
			if got != want[k] {
				same = false
			}
			if got != 1-want[k] {
				complement = false
			}
		}
		if same || complement {
			p := preambles[i]
			return &p
		}
	}
	return nil
}

func preambleByCode(code string) *preamble {
	for i := range preambles {
		if preambles[i].code == code {
			p := preambles[i]
			return &p
		}
	}
	return nil
}
