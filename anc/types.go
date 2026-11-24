package anc

// Classification is the primary, mutually exclusive verdict for a candidate
// ANC packet. Independent detail (parity, checksum, duplication, ...) is also
// exposed via Packet.Labels and the dedicated status structs.
type Classification string

const (
	// ClassValidVANC is a complete packet whose parity (when present) and
	// checksum verify.
	ClassValidVANC Classification = "valid_vanc"
	// ClassDuplicate is a later occurrence of an earlier identical packet.
	ClassDuplicate Classification = "duplicate"
	// ClassTruncated is a packet whose DC runs past the end of its video
	// line ("行间截断"), optionally reassembled with the adjacent line.
	ClassTruncated Classification = "truncated"
	// ClassInvalid is a complete packet with parity/checksum/framing errors.
	ClassInvalid Classification = "invalid"
)

// Label tags attached to a packet regardless of primary classification.
const (
	LabelValidVANC        = "valid_vanc"
	LabelDuplicate        = "duplicate"
	LabelExactDuplicate   = "exact_duplicate"
	LabelTruncated        = "truncated"
	LabelCrossLine        = "cross_line"
	LabelReassembledValid = "reassembled_checksum_ok"
	LabelParityError      = "parity_error"
	LabelChecksumError    = "checksum_error"
	LabelChecksumFraming  = "checksum_framing_error"
	LabelUnknownDID       = "unknown_did"
	LabelHeaderShort      = "header_incomplete"
	LabelDIDLineMismatch  = "did_line_mismatch"
)

// Encoding selects how the bytes of a video line map to ANC words.
type Encoding string

const (
	// EncWords uses explicit 10-bit word integers (JSON "words").
	EncWords Encoding = "words"
	// EncU16BE stores each 10-bit word in two bytes, big endian.
	EncU16BE Encoding = "u16be"
	// EncU16LE stores each 10-bit word in two bytes, little endian.
	EncU16LE Encoding = "u16le"
	// EncANC8 is the 8-bit ANC byte layout: 00 FF FF DID SDID DC UDW... CS8.
	EncANC8 Encoding = "anc8"
	// EncV210 is packed V210 (3 ten-bit samples in 4 bytes).
	EncV210 Encoding = "v210"
)

// ParityStatus records the parity verdict for the DID/SDID/DC words.
type ParityStatus struct {
	Present bool   `json:"present"`
	DID     string `json:"did"`
	SDID    string `json:"sdid_or_dbn"`
	DC      string `json:"dc"`
}

// ChecksumInfo records checksum verification details.
type ChecksumInfo struct {
	Mode     string `json:"mode"` // "st291_9bit", "sum8" or "absent"
	Status   string `json:"status"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Framing  string `json:"framing,omitempty"` // "ok"/"error" for 10-bit CS bit b9
}

// Location identifies where a packet (or continuation) was found.
type Location struct {
	Field        int `json:"field"`
	Line         int `json:"line"`
	ByteOffset   int `json:"byte_offset"`
	WordOffset   int `json:"word_offset,omitempty"`
	StreamOffset int `json:"stream_offset,omitempty"`
}

// FieldOffsets gives the byte offset of each structural ANC word within the
// video line. Checksum is omitted when the packet is truncated before CS.
type FieldOffsets struct {
	ADF       int `json:"adf"`
	DID       int `json:"did"`
	SDIDOrDBN int `json:"sdid_or_dbn"`
	DC        int `json:"dc"`
	Payload   int `json:"payload"`
	Checksum  int `json:"checksum,omitempty"`
}

// Continuation describes how a truncated packet may continue on a following
// video line.
type Continuation struct {
	Location
	UnitsTaken    int    `json:"units_taken"`
	MissingUnits  int    `json:"missing_units"`
	Reassembled   bool   `json:"reassembled"`
	ChecksumOK    bool   `json:"checksum_ok"`
	NextStartsADF bool   `json:"next_starts_adf,omitempty"`
	RawHex        string `json:"raw_hex,omitempty"`
}

// Packet is one located ANC candidate. Unknown DIDs are fully retained.
type Packet struct {
	Index          int            `json:"index"`
	Classification Classification `json:"classification"`
	Labels         []string       `json:"labels,omitempty"`
	Reasons        []string       `json:"reasons,omitempty"`

	Field        int    `json:"field"`
	Line         int    `json:"line"`
	ByteOffset   int    `json:"byte_offset"`
	WordOffset   int    `json:"word_offset,omitempty"`
	StreamOffset int    `json:"stream_offset,omitempty"`
	Encoding     string `json:"encoding,omitempty"`

	DID        byte   `json:"did"`
	DIDHex     string `json:"did_hex"`
	DIDName    string `json:"did_name"`
	UnknownDID bool   `json:"unknown_did"`
	IDType     string `json:"id_type"` // "type1_dbn" or "type2_sdid"
	SDIDOrDBN  byte   `json:"sdid_or_dbn"`
	SDIDHex    string `json:"sdid_or_dbn_hex"`
	SDIDName   string `json:"sdid_name,omitempty"`

	DC           int    `json:"dc"`
	PayloadHex   string `json:"payload_hex"`
	PayloadASCII string `json:"payload_ascii,omitempty"`
	UDW          []int  `json:"udw,omitempty"`

	RawHex    string        `json:"raw_hex"`
	RawLength int           `json:"raw_length"`
	Offsets   *FieldOffsets `json:"offsets,omitempty"`

	Parity   *ParityStatus `json:"parity,omitempty"`
	Checksum *ChecksumInfo `json:"checksum,omitempty"`

	DuplicateOf    *Location     `json:"duplicate_of,omitempty"`
	DuplicateMatch string        `json:"duplicate_match,omitempty"` // "raw" or "content"
	Continuation   *Continuation `json:"continuation,omitempty"`
}

// Summary aggregates report counts.
type Summary struct {
	Packets        int `json:"packets"`
	ValidVANC      int `json:"valid_vanc"`
	Duplicate      int `json:"duplicate"`
	Truncated      int `json:"truncated"`
	Invalid        int `json:"invalid"`
	UnknownDID     int `json:"unknown_did"`
	ParityErrors   int `json:"parity_errors"`
	ChecksumErrors int `json:"checksum_errors"`
	CrossLine      int `json:"cross_line"`
	ReassembledOK  int `json:"reassembled_ok"`
}

// Report is the complete scan result.
type Report struct {
	Schema       string    `json:"schema"`
	Standard     string    `json:"standard,omitempty"`
	Encoding     string    `json:"encoding,omitempty"`
	ScannedLines int       `json:"scanned_lines"`
	Summary      Summary   `json:"summary"`
	Packets      []*Packet `json:"packets"`
}

// Options controls scanning behaviour.
type Options struct {
	// Encoding is the fallback encoding for lines that do not specify one.
	Encoding Encoding
	// ContinuationLines is how many subsequent input lines may be consulted
	// when reconstructing a truncated packet. Defaults to 1.
	ContinuationLines int
	// ExpectedLines optionally maps a DID byte to the video lines on which it
	// is expected for the current standard. A mismatch only adds an advisory
	// label; it never changes parsing or drops the packet.
	ExpectedLines map[byte][]int
}

func (o *Options) withDefaults() Options {
	cp := Options{}
	if o != nil {
		cp = *o
	}
	if cp.Encoding == "" {
		cp.Encoding = EncU16BE
	}
	if cp.ContinuationLines <= 0 {
		cp.ContinuationLines = 1
	}
	return cp
}
