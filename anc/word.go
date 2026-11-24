// Package anc parses SMPTE ST 291-1 ancillary data (ANC) packets embedded in
// serialized video lines (VANC/HANC).
//
// The package is deliberately tolerant of damaged captures: parity and
// checksum failures are reported instead of dropping packets, every unknown
// DID is preserved together with its payload and raw bytes, and packets that
// are cut at a line boundary are flagged as truncated/inter-line rather than
// mistaken for valid VANC.
package anc

// Word is a 10-bit SDI ANC word. Only the low ten bits are meaningful.
type Word uint16

// Ancillary data flag (ADF) for a component interface: 000h, 3FFh, 3FFh.
const (
	ADFZero Word = 0x000
	ADFOne  Word = 0x3FF
	MaxWord Word = 0x3FF
)

// Data returns the low 8 identifier/data bits of the word.
func (w Word) Data() byte { return byte(w & 0xFF) }

// Bit8 returns bit b8 of the word.
func (w Word) Bit8() byte { return byte((w >> 8) & 1) }

// Bit9 returns bit b9 of the word.
func (w Word) Bit9() byte { return byte((w >> 9) & 1) }

// EvenParity returns the even-parity bit for the supplied 8-bit value: it is
// 1 when v has an odd number of set bits, so v plus the returned bit has even
// parity (SMPTE ST 291-1 §6.1).
func EvenParity(v byte) byte {
	p := v
	p ^= p >> 4
	p ^= p >> 2
	p ^= p >> 1
	return p & 1
}

// WithParity builds a DID/SDID/DC word from its 8-bit value by setting b8 to
// the even parity bit and b9 to the inverse of b8.
func WithParity(v byte) Word {
	p := EvenParity(v)
	w := Word(v) | Word(p)<<8
	if p == 0 {
		w |= 1 << 9 // b9 = NOT b8
	}
	return w
}

// ParityOK reports whether a DID/SDID/DBN/DC word carries the correct even
// parity bit and its inverse.
func ParityOK(w Word) bool {
	p := EvenParity(w.Data())
	return w.Bit8() == p && w.Bit9() == (p^1)
}

// Checksum9 computes the ST 291-1 checksum value: the nine least significant
// bits of the sum of the nine least significant bits of every supplied word
// (DID, SDID/DBN, DC and all UDW). End carries are discarded, i.e. the result
// is taken modulo 512.
func Checksum9(words ...Word) uint16 {
	var sum uint16
	for _, w := range words {
		sum += uint16(w) & 0x1FF
	}
	return sum & 0x1FF
}

// MakeChecksumWord encodes a 9-bit checksum as the CS word: b8..b0 hold the
// checksum and b9 is the inverse of checksum bit b8.
func MakeChecksumWord(cs uint16) Word {
	cs &= 0x1FF
	w := Word(cs)
	if cs&0x100 == 0 {
		w |= 1 << 9 // b9 = NOT b8 (b8 is the MSB of the checksum)
	}
	return w
}

// ChecksumFramingOK reports whether the CS word has b9 = NOT b8.
func ChecksumFramingOK(cs Word) bool {
	return cs.Bit9() == cs.Bit8()^1
}

// Checksum8 computes the byte-wrapping checksum used by the 8-bit ANC
// representation (SMPTE ST 436M 8-bit payloads / MCC byte streams): the
// modulo-256 sum of DID, SDID/DBN, DC and the payload bytes.
func Checksum8(did, sdidOrDBN, dc byte, payload []byte) byte {
	var cs byte
	cs += did
	cs += sdidOrDBN
	cs += dc
	for _, b := range payload {
		cs += b
	}
	return cs
}
