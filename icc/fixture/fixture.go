// Package fixture builds small synthetic ICC profiles for tests. It only
// emits the matrix/TRC subset (plus deliberately malformed cases) and uses
// nothing beyond the Go standard library.
package fixture

import (
	"encoding/binary"
)

// Builder accumulates a header, tag table and tag payloads.
type Builder struct {
	version   uint32
	class     string
	color     string
	pcs       string
	headerWP  [3]float64
	payloads  map[string][]byte
	truncate  int
	extraSize int

	order   []string // explicit table order; duplicates allowed
	sizeOvr map[string]int
	offOvr  map[string]int
}

type entry struct {
	sig    string
	offset int
	size   int
}

// New starts a builder with sensible display-profile defaults (D50 header).
func New(major, minor uint8) *Builder {
	return &Builder{
		version:  uint32(major)<<24 | uint32(minor)<<20,
		class:    "mntr",
		color:    "RGB ",
		pcs:      "XYZ ",
		headerWP: [3]float64{0.9642, 1.0, 0.8249},
		payloads: map[string][]byte{},
		sizeOvr:  map[string]int{},
		offOvr:   map[string]int{},
	}
}

// SetHeaderWP overrides the header illuminant (s15Fixed16 XYZ).
func (b *Builder) SetHeaderWP(x, y, z float64) *Builder {
	b.headerWP = [3]float64{x, y, z}
	return b
}

// AddXYZ attaches an XYZ-type tag.
func (b *Builder) AddXYZ(sig string, x, y, z float64) *Builder {
	p := make([]byte, 20)
	copy(p[0:4], "XYZ ")
	putS15(p[8:], x)
	putS15(p[12:], y)
	putS15(p[16:], z)
	b.payloads[sig] = p
	return b
}

// AddGamma attaches a curv tag holding a single u8Fixed8 gamma value.
func (b *Builder) AddGamma(sig string, gamma float64) *Builder {
	p := make([]byte, 14)
	copy(p[0:4], "curv")
	binary.BigEndian.PutUint32(p[8:], 1)
	binary.BigEndian.PutUint16(p[12:], u8Fixed8(gamma))
	b.payloads[sig] = p
	return b
}

// AddLUT attaches a curv tag from already-normalized [0,1] entries.
func (b *Builder) AddLUT(sig string, values []float64) *Builder {
	p := make([]byte, 12+2*len(values))
	copy(p[0:4], "curv")
	binary.BigEndian.PutUint32(p[8:], uint32(len(values)))
	for i, v := range values {
		binary.BigEndian.PutUint16(p[12+2*i:], u16Fixed16(v))
	}
	b.payloads[sig] = p
	return b
}

// AddIdentity attaches a curv tag with count zero.
func (b *Builder) AddIdentity(sig string) *Builder {
	p := make([]byte, 12)
	copy(p[0:4], "curv")
	b.payloads[sig] = p
	return b
}

// AddPara attaches a parametricCurveType tag (function type 0..4).
func (b *Builder) AddPara(sig string, fnType uint16, params []float64) *Builder {
	p := make([]byte, 12+4*len(params))
	copy(p[0:4], "para")
	binary.BigEndian.PutUint16(p[8:], fnType)
	for i, v := range params {
		putS15(p[12+4*i:], v)
	}
	b.payloads[sig] = p
	return b
}

// AddRaw attaches an arbitrary payload (used for malformed/unsupported tags).
func (b *Builder) AddRaw(sig string, payload []byte) *Builder {
	b.payloads[sig] = payload
	return b
}

// Duplicate forces the given tag signature to appear twice in the table.
func (b *Builder) Duplicate(sig string) *Builder {
	if _, ok := b.payloads[sig]; ok {
		b.order = append(b.order, sig)
	}
	return b
}

// SetWrongLength overrides the size recorded in the table for a tag while
// leaving the payload itself untouched.
func (b *Builder) SetWrongLength(sig string, size int) *Builder {
	b.sizeOvr[sig] = size
	return b
}

// SetOffset moves a tag's declared offset (used to create out-of-range tags).
func (b *Builder) SetOffset(sig string, offset int) *Builder {
	b.offOvr[sig] = offset
	return b
}

// Build serializes the profile. Table order follows payload insertion order
// unless Duplicate/SetWrongLength/SetOffset mutated an entry afterwards.
func (b *Builder) Build() []byte {
	order := b.tagOrder()

	// Assign each distinct payload one offset after header + table.
	tableBytes := 4 + 12*len(order)
	cursor := 128 + tableBytes
	offsets := map[string]int{}
	placed := map[string]bool{}
	entries := make([]entry, 0, len(order))
	for _, sig := range order {
		if !placed[sig] {
			offsets[sig] = cursor
			cursor += len(b.payloads[sig])
			placed[sig] = true
		}
		size := len(b.payloads[sig])
		if ovr, ok := b.sizeOvr[sig]; ok {
			size = ovr
		}
		off := offsets[sig]
		if ovr, ok := b.offOvr[sig]; ok {
			off = ovr
		}
		entries = append(entries, entry{sig: sig, offset: off, size: size})
	}

	total := cursor + b.extraSize
	if b.truncate > 0 {
		total = b.truncate
	}
	out := make([]byte, total)
	b.writeHeader(out)

	// Tag table.
	binary.BigEndian.PutUint32(out[128:], uint32(len(entries)))
	for i, e := range entries {
		base := 132 + 12*i
		copy(out[base:base+4], pad4(e.sig))
		binary.BigEndian.PutUint32(out[base+4:], uint32(e.offset))
		binary.BigEndian.PutUint32(out[base+8:], uint32(e.size))
	}
	// Payloads.
	for sig, p := range b.payloads {
		off := offsets[sig]
		copy(out[off:], p)
	}
	return out
}
