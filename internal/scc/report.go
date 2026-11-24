package scc

import (
	"fmt"
	"strings"
)

// Divergence records the first frame where strict and naive
// reconstruction produce different channel states.
type Divergence struct {
	Frame    int
	Timecode string
	Channel  int
	Strict   string
	Naive    string
}

func (d Divergence) String() string {
	return fmt.Sprintf("first divergence at frame %d (%s) channel %d:\n--- strict ---\n%s--- naive ---\n%s",
		d.Frame, d.Timecode, d.Channel, indent(d.Strict), indent(d.Naive))
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n") + "\n"
}

// Run feeds every frame to a strict and a naive decoder in lockstep and
// returns both decoders plus the first state divergence, if any.
func Run(frames []Frame) (strict, naive *Decoder, div *Divergence) {
	strict = NewDecoder(Strict())
	naive = NewDecoder(Naive())
	for _, f := range frames {
		strict.FeedFrame(f)
		naive.FeedFrame(f)
		if div == nil {
			for ch := 1; ch <= 2; ch++ {
				ss := strict.Channel(ch).Snapshot()
				ns := naive.Channel(ch).Snapshot()
				if ss != ns {
					div = &Divergence{
						Frame: f.Index, Timecode: f.Timecode, Channel: ch,
						Strict: ss, Naive: ns,
					}
					break
				}
			}
		}
	}
	return strict, naive, div
}

// Report renders the full human-readable analysis, feeding both
// decoders frame by frame so per-frame text reflects that moment.
func Report(frames []Frame, errs []ParseError) string {
	strict := NewDecoder(Strict())
	naive := NewDecoder(Naive())
	var div *Divergence
	var b strings.Builder

	if len(errs) > 0 {
		fmt.Fprintf(&b, "PARSE WARNINGS (%d)\n", len(errs))
		for _, e := range errs {
			fmt.Fprintf(&b, "  %s\n", e.Error())
		}
		b.WriteString("\n")
	}

	ev := 0
	for _, f := range frames {
		strict.FeedFrame(f)
		naive.FeedFrame(f)
		if div == nil {
			for ch := 1; ch <= 2; ch++ {
				if strict.Channel(ch).Snapshot() != naive.Channel(ch).Snapshot() {
					div = &Divergence{
						Frame: f.Index, Timecode: f.Timecode, Channel: ch,
						Strict: strict.Channel(ch).Snapshot(),
						Naive:  naive.Channel(ch).Snapshot(),
					}
					break
				}
			}
		}

		var raw []string
		for _, by := range f.Bytes {
			raw = append(raw, markByte(by))
		}
		if f.CarryOut != nil {
			raw = append(raw, markByte(*f.CarryOut)+"v") // dangles into next frame
		}
		fmt.Fprintf(&b, "FRAME %d  %s\n  raw: %s\n", f.Index, f.Timecode, strings.Join(raw, " "))
		for ev < len(strict.Events) && strict.Events[ev].Frame == f.Index {
			fmt.Fprintf(&b, "  %s\n", strict.Events[ev])
			ev++
		}
		for ch := 1; ch <= 2; ch++ {
			for _, line := range strict.Channel(ch).VisibleText() {
				fmt.Fprintf(&b, "  ch%d text: %s\n", ch, line)
			}
		}
	}

	b.WriteString("\n")
	if div != nil {
		b.WriteString(div.String() + "\n")
	} else {
		b.WriteString("no state divergence between strict and naive reconstruction\n")
	}
	return b.String()
}

func markByte(by Byte) string {
	tok := by.RawToken
	if !by.ParityOK {
		tok += "!" // bad odd parity
	}
	if by.CarriedIn {
		tok += "^" // carried in from previous frame
	}
	return tok
}
