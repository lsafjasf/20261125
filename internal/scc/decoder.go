package scc

import (
	"fmt"
	"strings"
)

const (
	Rows = 15
	Cols = 32
)

// Mode is the caption delivery mode of a channel.
type Mode int

const (
	ModePopOn Mode = iota
	ModeRollUp
	ModePaintOn
)

func (m Mode) String() string {
	switch m {
	case ModeRollUp:
		return "roll-up"
	case ModePaintOn:
		return "paint-on"
	}
	return "pop-on"
}

// Options selects between strict and naive reconstruction.
type Options struct {
	SkipBadParity bool // drop bytes whose odd-parity check fails
	DedupControls bool // collapse immediately repeated control pairs
}

// Strict applies CEA-608 transmission rules: control codes are sent
// twice and the duplicate is ignored; bad-parity bytes are dropped.
func Strict() Options { return Options{SkipBadParity: true, DedupControls: true} }

// Naive processes every byte literally: no dedup, no parity filtering.
func Naive() Options { return Options{} }

// Event records one decoded pair (or anomaly) for the report.
type Event struct {
	Frame    int
	Timecode string
	Raw      string // raw hex of the pair, e.g. "9426"
	Channel  int
	Code     Code
	Note     string // "duplicate ignored", "bad parity skipped", ...
}

func (e Event) String() string {
	s := fmt.Sprintf("ch%d %-7s %s", e.Channel, e.Code.Kind, e.Raw)
	if e.Code.Kind == KindControl {
		s += " " + e.Code.Name
	}
	if e.Code.Kind == KindPAC {
		s += fmt.Sprintf("(row=%d indent=%d u=%v)", e.Code.Row+1, e.Code.Indent, e.Code.Underline)
	}
	if e.Note != "" {
		s += " [" + e.Note + "]"
	}
	return s
}

type memory [Rows][Cols]rune

func (m *memory) clear() {
	for r := range m {
		for c := range m[r] {
			m[r][c] = 0
		}
	}
}

func (m *memory) clearRow(r int) {
	for c := range m[r] {
		m[r][c] = 0
	}
}

// ChannelState is the reconstructed state of one caption channel.
type ChannelState struct {
	Mode      Mode
	RollRows  int // roll-up window size: 2, 3 or 4
	BaseRow   int // roll-up window base row (0-based)
	CursorRow int
	CursorCol int
	Display   memory
	NonDisp   memory

	lastB1, lastB2 byte
	hasLast        bool
}

func NewChannelState() *ChannelState {
	return &ChannelState{Mode: ModePopOn, RollRows: 0, BaseRow: Rows - 1, CursorRow: Rows - 1}
}

// active returns the memory new text is written to in the current mode.
func (s *ChannelState) active() *memory {
	if s.Mode == ModePopOn {
		return &s.NonDisp
	}
	return &s.Display
}

// Snapshot renders the state for divergence comparison.
func (s *ChannelState) Snapshot() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mode=%s roll=%d base=%d cursor=%d:%d\n",
		s.Mode, s.RollRows, s.BaseRow+1, s.CursorRow+1, s.CursorCol)
	for r := 0; r < Rows; r++ {
		line := strings.TrimRight(string(s.Display[r][:]), string(rune(0)))
		line = strings.TrimRight(line, "\x00")
		if line != "" {
			fmt.Fprintf(&b, "  row%02d|%s|\n", r+1, line)
		}
	}
	return b.String()
}

// VisibleText returns the non-empty displayed rows.
func (s *ChannelState) VisibleText() []string {
	var out []string
	for r := 0; r < Rows; r++ {
		line := strings.TrimRight(strings.TrimRight(
			string(s.Display[r][:]), "\x00"), " ")
		if line != "" {
			out = append(out, fmt.Sprintf("row%02d %s", r+1, line))
		}
	}
	return out
}

// Decoder reconstructs both caption channels from a byte stream.
type Decoder struct {
	opts   Options
	ch     [2]*ChannelState
	curCh  int // channel selected by the most recent control code
	Events []Event
}

func NewDecoder(opts Options) *Decoder {
	return &Decoder{opts: opts,
		ch: [2]*ChannelState{NewChannelState(), NewChannelState()}}
}

// Channel returns the state of channel 1 or 2.
func (d *Decoder) Channel(n int) *ChannelState { return d.ch[n-1] }

// FeedFrame decodes all byte pairs of one frame, in order.
func (d *Decoder) FeedFrame(f Frame) {
	for i := 0; i+1 < len(f.Bytes); i += 2 {
		d.feedPair(f, f.Bytes[i], f.Bytes[i+1])
	}
}

func (d *Decoder) feedPair(f Frame, x, y Byte) {
	raw := x.RawToken + y.RawToken
	note := ""
	if x.CarriedIn {
		note = "carried from previous frame"
	}
	if d.opts.SkipBadParity && (!x.ParityOK || !y.ParityOK) {
		d.Events = append(d.Events, Event{
			Frame: f.Index, Timecode: f.Timecode, Raw: raw,
			Code: Code{Kind: KindUnknown, Name: "bad-parity"},
			Note: "bad parity skipped",
		})
		return
	}
	b1, b2 := x.Value, y.Value

	// Printable pair: both bytes in 0x20..0x7F.
	if b1 >= 0x20 && b1 <= 0x7F {
		st := d.ch[d.curCh]
		st.hasLast = false
		st.putRune(rune(b1))
		if b2 >= 0x20 && b2 <= 0x7F {
			st.putRune(rune(b2))
		}
		d.Events = append(d.Events, Event{
			Frame: f.Index, Timecode: f.Timecode, Raw: raw, Channel: d.curCh + 1,
			Code: Code{Kind: KindText, Char: rune(b1)}, Note: note,
		})
		return
	}

	code := DecodePair(b1, b2)
	st := d.ch[code.Channel-1]
	if code.Kind == KindUnknown {
		// Unknown code: keep it in the log, change nothing, continue.
		d.Events = append(d.Events, Event{
			Frame: f.Index, Timecode: f.Timecode, Raw: raw,
			Channel: code.Channel, Code: code,
			Note: mergeNote(note, "unknown code kept"),
		})
		st.hasLast = false
		return
	}

	if d.opts.DedupControls && st.hasLast && st.lastB1 == b1 && st.lastB2 == b2 {
		d.Events = append(d.Events, Event{
			Frame: f.Index, Timecode: f.Timecode, Raw: raw,
			Channel: code.Channel, Code: code,
			Note: mergeNote(note, "duplicate ignored"),
		})
		return
	}
	st.lastB1, st.lastB2, st.hasLast = b1, b2, true

	d.Events = append(d.Events, Event{
		Frame: f.Index, Timecode: f.Timecode, Raw: raw,
		Channel: code.Channel, Code: code, Note: note,
	})
	d.curCh = code.Channel - 1
	st.apply(code)
}

func mergeNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func (s *ChannelState) putRune(r rune) {
	m := s.active()
	if s.CursorCol >= Cols {
		s.CursorCol = Cols - 1
	}
	m[s.CursorRow][s.CursorCol] = r
	s.CursorCol++
}

func (s *ChannelState) apply(c Code) {
	switch c.Kind {
	case KindPAC:
		s.CursorRow = c.Row
		s.CursorCol = c.Indent
		if s.Mode == ModeRollUp {
			s.BaseRow = c.Row
		}
	case KindMidRow:
		s.putRune(' ') // mid-row codes render as a space
	case KindTab:
		s.CursorCol += c.Tab
		if s.CursorCol >= Cols {
			s.CursorCol = Cols - 1
		}
	case KindSpecial, KindExtended:
		s.putRune(c.Char)
	case KindControl:
		s.applyControl(c.Name)
	}
}

func (s *ChannelState) applyControl(name string) {
	switch name {
	case "RCL":
		s.Mode = ModePopOn
	case "RDC":
		s.Mode = ModePaintOn
	case "RU2", "RU3", "RU4":
		s.Mode = ModeRollUp
		s.RollRows = int(name[2] - '0')
		s.BaseRow = Rows - 1
		s.CursorRow = s.BaseRow
		s.CursorCol = 0
	case "CR":
		if s.Mode == ModeRollUp {
			top := s.BaseRow - s.RollRows + 1
			if top < 0 {
				top = 0
			}
			for r := top; r < s.BaseRow; r++ {
				s.Display[r] = s.Display[r+1]
			}
			s.Display.clearRow(s.BaseRow)
			s.CursorRow = s.BaseRow
			s.CursorCol = 0
		}
	case "EDM":
		s.Display.clear()
	case "ENM":
		s.NonDisp.clear()
	case "EOC":
		s.Display, s.NonDisp = s.NonDisp, s.Display
	case "BS":
		if s.CursorCol > 0 {
			s.CursorCol--
			s.active()[s.CursorRow][s.CursorCol] = 0
		}
	case "DER":
		m := s.active()
		for c := s.CursorCol; c < Cols; c++ {
			m[s.CursorRow][c] = 0
		}
	}
}
