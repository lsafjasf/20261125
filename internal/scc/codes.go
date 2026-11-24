package scc

// Kind classifies a decoded two-byte code pair.
type Kind int

const (
	KindUnknown  Kind = iota
	KindText          // printable basic characters
	KindControl       // misc control: RCL, RU2..RU4, CR, EDM, EOC, ...
	KindPAC           // preamble address code: row + indent/underline
	KindMidRow        // mid-row style code (consumes one space)
	KindTab           // tab offset 1..3
	KindSpecial       // special character (0x11/0x19 + 0x30..0x3F)
	KindExtended      // extended character set (0x12/0x13, 0x1A/0x1B)
)

func (k Kind) String() string {
	switch k {
	case KindText:
		return "TEXT"
	case KindControl:
		return "CTRL"
	case KindPAC:
		return "PAC"
	case KindMidRow:
		return "MIDROW"
	case KindTab:
		return "TAB"
	case KindSpecial:
		return "SPECIAL"
	case KindExtended:
		return "EXTENDED"
	}
	return "UNKNOWN"
}

// Code is a decoded two-byte pair (parity already stripped).
type Code struct {
	B1, B2  byte
	Kind    Kind
	Channel int // 1 or 2
	Name    string
	// PAC fields
	Row       int  // 0-based row, PAC only
	Indent    int  // column indent, PAC only
	Underline bool // PAC / mid-row
	// Tab offset
	Tab int
	// Rendered character for text/special/extended
	Char rune
}

var controlNames = map[byte]string{
	0x20: "RCL", // resume caption loading (pop-on)
	0x21: "BS",  // backspace
	0x22: "AOF", // alarm off (reserved)
	0x23: "AON", // alarm on (reserved)
	0x24: "DER", // delete to end of row
	0x25: "RU2", // roll-up, 2 rows
	0x26: "RU3", // roll-up, 3 rows
	0x27: "RU4", // roll-up, 4 rows
	0x28: "FON", // flash on
	0x29: "RDC", // resume direct captioning (paint-on)
	0x2A: "TR",  // text restart
	0x2B: "RTD", // resume text display
	0x2C: "EDM", // erase displayed memory
	0x2D: "CR",  // carriage return (roll up)
	0x2E: "ENM", // erase non-displayed memory
	0x2F: "EOC", // end of caption (flip memories)
}

var specialChars = map[byte]rune{
	0x30: '®', 0x31: '°', 0x32: '½', 0x33: '¿', 0x34: '™',
	0x35: '¢', 0x36: '£', 0x37: '♪', 0x38: 'à', 0x39: ' ',
	0x3A: 'è', 0x3B: 'â', 0x3C: 'ê', 0x3D: 'î', 0x3E: 'ô', 0x3F: 'û',
}

// pacRow maps (b1 low 3 bits, b2 bit 5) to a 0-based row, per the
// CEA-608 PAC table: 0x11->rows 1/2, 0x12->3/4, 0x15->5/6, 0x16->7/8,
// 0x17->9/10, 0x10->11, 0x13->12/13, 0x14->14/15.
var pacRow = [8][2]int{
	{10, 10}, // b1 low = 0 (0x10): row 11
	{0, 1},   // 1 (0x11): rows 1, 2
	{2, 3},   // 2 (0x12): rows 3, 4
	{11, 12}, // 3 (0x13): rows 12, 13
	{13, 14}, // 4 (0x14): rows 14, 15
	{4, 5},   // 5 (0x15): rows 5, 6
	{6, 7},   // 6 (0x16): rows 7, 8
	{8, 9},   // 7 (0x17): rows 9, 10
}

// channelOf returns 1 or 2 for a control byte, or 0 if not a control byte.
func channelOf(b1 byte) int {
	switch {
	case b1 >= 0x10 && b1 <= 0x17:
		return 1
	case b1 >= 0x18 && b1 <= 0x1F:
		return 2
	}
	return 0
}

// DecodePair classifies a two-byte pair. b1 and b2 must already have
// parity stripped. Unknown pairs are returned with KindUnknown so the
// caller can keep them and continue scanning.
func DecodePair(b1, b2 byte) Code {
	ch := channelOf(b1)
	if ch == 0 {
		return Code{B1: b1, B2: b2, Kind: KindUnknown, Name: "not-a-control-byte"}
	}
	c := Code{B1: b1, B2: b2, Channel: ch}

	// PAC: b2 in 0x40..0x7F
	if b2 >= 0x40 && b2 <= 0x7F {
		c.Kind = KindPAC
		c.Row = pacRow[b1&0x07][(b2>>5)&1]
		c.Underline = b2&0x01 == 0x01
		if b2&0x10 == 0 {
			c.Indent = int((b2&0x0E)>>1) * 4
		}
		c.Name = "PAC"
		return c
	}

	low1 := b1 & 0x07
	switch {
	// Misc control codes: 0x14 (ch1) / 0x1C (ch2) + 0x20..0x2F
	case b2 >= 0x20 && b2 <= 0x2F && low1 == 4:
		if name, ok := controlNames[b2]; ok {
			c.Kind = KindControl
			c.Name = name
			return c
		}
	// Mid-row style: 0x11/0x19 + 0x20..0x2F
	case b2 >= 0x20 && b2 <= 0x2F && low1 == 1:
		c.Kind = KindMidRow
		c.Underline = b2&0x01 == 0x01
		c.Name = "MIDROW"
		return c
	// Tab offsets: 0x17/0x1F + 0x21..0x23
	case low1 == 7 && b2 >= 0x21 && b2 <= 0x23:
		c.Kind = KindTab
		c.Tab = int(b2 - 0x20)
		c.Name = "TAB"
		return c
	// Special characters: 0x11/0x19 + 0x30..0x3F
	case low1 == 1 && b2 >= 0x30 && b2 <= 0x3F:
		c.Kind = KindSpecial
		c.Char = specialChars[b2]
		c.Name = "SPECIAL"
		return c
	// Extended characters: 0x12/0x13 (ch1), 0x1A/0x1B (ch2) + 0x20..0x3F
	case (low1 == 2 || low1 == 3) && b2 >= 0x20 && b2 <= 0x3F:
		c.Kind = KindExtended
		c.Char = extendedChar(b1, b2)
		c.Name = "EXTENDED"
		return c
	}
	c.Kind = KindUnknown
	c.Name = "unknown-control"
	return c
}

func extendedChar(b1, b2 byte) rune {
	// Spanish/French (0x12/0x1A) and Portuguese/German/Danish (0x13/0x1B)
	// subsets; unmapped positions fall back to a placeholder.
	spanish := map[byte]rune{
		0x20: 'Á', 0x21: 'É', 0x22: 'Ó', 0x23: 'Ú', 0x24: 'Ü', 0x25: 'ü',
		0x26: '‘', 0x27: '¡', 0x28: '*', 0x29: '\'', 0x2A: '—', 0x2B: '©',
		0x2C: '℠', 0x2D: '•', 0x2E: '“', 0x2F: '”', 0x30: 'À', 0x31: 'Â',
		0x32: 'Ç', 0x33: 'È', 0x34: 'Ê', 0x35: 'Ë', 0x36: 'ë', 0x37: 'Î',
		0x38: 'Ï', 0x39: 'ï', 0x3A: 'Ô', 0x3B: 'Ù', 0x3C: 'ù', 0x3D: 'Û',
		0x3E: '«', 0x3F: '»',
	}
	portuguese := map[byte]rune{
		0x20: 'Ã', 0x21: 'ã', 0x22: 'Í', 0x23: 'Ì', 0x24: 'ì', 0x25: 'Ò',
		0x26: 'ò', 0x27: 'Õ', 0x28: 'õ', 0x29: '{', 0x2A: '}', 0x2B: '\\',
		0x2C: '^', 0x2D: '_', 0x2E: '|', 0x2F: '~', 0x30: 'Ä', 0x31: 'ä',
		0x32: 'Ö', 0x33: 'ö', 0x34: 'ß', 0x35: '¥', 0x36: '¤', 0x37: '│',
		0x38: 'Å', 0x39: 'å', 0x3A: 'Ø', 0x3B: 'ø', 0x3C: '┌', 0x3D: '┐',
		0x3E: '└', 0x3F: '┘',
	}
	if b1&0x07 == 2 {
		if r, ok := spanish[b2]; ok {
			return r
		}
	} else if r, ok := portuguese[b2]; ok {
		return r
	}
	return '�'
}
