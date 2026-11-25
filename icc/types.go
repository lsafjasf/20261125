package icc

import "fmt"

// Version is the ICC profile spec version encoded in the header (major.minor.bugfix).
type Version struct {
	Major  int
	Minor  int
	Bugfix int
	Raw    uint32
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Bugfix)
}

// XYZ is a CIE XYZ tristimulus value already converted from s15Fixed16.
type XYZ struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// TagKind classifies a tag relative to the supported matrix/TRC subset.
type TagKind int

const (
	TagUnsupported TagKind = iota // present in the tag table, outside the declared subset
	TagMatrix                     // rXYZ/gXYZ/bXYZ/wtpt
	TagTRC                        // rTRC/gTRC/bTRC
)

// TRCKind enumerates the only curve encodings the subset understands.
type TRCKind int

const (
	TRCIdentity TRCKind = iota // curv count == 0
	TRCGamma                   // curv count == 1 or para type 0
	TRCLUT                     // curv count >= 2
	TRCPara                    // para type 1..4
)

// TRC is a tone reproduction curve in one of the supported encodings.
type TRC struct {
	Kind     TRCKind
	Gamma    float64   // TRCGamma / TRCPara type 0
	ParaType int       // para function type, 1..4
	Params   []float64 // para parameters in file order (g, a, b, c, d, ...)
	Entries  []float64 // LUT entries, already normalized to [0,1]
}

// Tag is one entry of the tag table.
type Tag struct {
	Signature string // 4-byte tag signature
	Offset    int
	Size      int
	Kind      TagKind

	// Filled only for tags that belong to the supported subset.
	Type   string // type signature, e.g. "XYZ ", "curv", "para"
	XYZ    *XYZ
	TRC    *TRC
	Loaded bool

	Issues []string // parse/validation problems specific to this tag
}

// Profile is the parsed subset of one ICC file.
type Profile struct {
	Path       string
	Size       int
	Version    Version
	Class      string
	ColorSpace string
	PCS        string
	HeaderWP   XYZ // header illuminant at offset 68
	Tags       []*Tag
	Issues     []string // file-level and tag-table problems
}

// TagCount returns how many entries the tag table declares.
func (p *Profile) TagCount() int {
	return len(p.Tags)
}

func (p *Profile) addIssue(format string, args ...any) {
	p.Issues = append(p.Issues, fmt.Sprintf(format, args...))
}

// FindTag returns the first tag with the given signature and whether it exists.
func (p *Profile) FindTag(sig string) (*Tag, bool) {
	for _, t := range p.Tags {
		if t.Signature == sig {
			return t, true
		}
	}
	return nil, false
}

// DuplicateTags returns signatures that appear more than once, first..nth order.
func (p *Profile) DuplicateTags() map[string]int {
	counts := map[string]int{}
	for _, t := range p.Tags {
		counts[t.Signature]++
	}
	for sig, n := range counts {
		if n < 2 {
			delete(counts, sig)
		}
	}
	return counts
}
