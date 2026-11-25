package icc

import "encoding/binary"

// reader is a bounds-checked big-endian view over the raw profile bytes.
type reader struct {
	data []byte
}

func (r reader) len() int { return len(r.data) }

func (r reader) u8(off int) (uint8, bool) {
	if off < 0 || off+1 > len(r.data) {
		return 0, false
	}
	return r.data[off], true
}

func (r reader) u16(off int) (uint16, bool) {
	if off < 0 || off+2 > len(r.data) {
		return 0, false
	}
	return binary.BigEndian.Uint16(r.data[off:]), true
}

func (r reader) u32(off int) (uint32, bool) {
	if off < 0 || off+4 > len(r.data) {
		return 0, false
	}
	return binary.BigEndian.Uint32(r.data[off:]), true
}

func (r reader) s32(off int) (int32, bool) {
	v, ok := r.u32(off)
	return int32(v), ok
}

func (r reader) sig(off int) (string, bool) {
	if off < 0 || off+4 > len(r.data) {
		return "", false
	}
	return string(r.data[off : off+4]), true
}

func (r reader) slice(off, size int) ([]byte, bool) {
	if off < 0 || size < 0 || off+size > len(r.data) || off+size < off {
		return nil, false
	}
	return r.data[off : off+size], true
}

// s15Fixed16 decodes a signed 16.16 fixed point value.
func fromS15Fixed16(v uint32) float64 {
	return float64(int32(v)) / 65536.0
}

// u8Fixed8 decodes an unsigned 8.8 fixed point value (curv count == 1).
func fromU8Fixed8(v uint16) float64 {
	return float64(v) / 256.0
}

// u16Fixed16 decodes an unsigned 16.16 fixed point value (curv LUT entries).
func fromU16Fixed16(v uint16) float64 {
	return float64(v) / 65535.0
}

func toS15Fixed16(v float64) uint32 {
	return uint32(int32(v*65536.0 + 0.5))
}

func toU16Fixed16(v float64) uint16 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 65535
	}
	return uint16(v*65535.0 + 0.5)
}

func toU8Fixed8(v float64) uint16 {
	if v <= 0 {
		return 0
	}
	return uint16(v*256.0 + 0.5)
}
