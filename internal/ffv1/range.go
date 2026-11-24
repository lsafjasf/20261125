package ffv1

import "errors"

var errSymbolTooLarge = errors.New("FFV1 range-coded symbol is too large")

// The FFV1 range coder uses 8-bit probability states. The default
// transition table is generated exactly like FFmpeg's
// ff_build_rac_states(c, 0.05 * (1<<32), 256-8).
var zeroState, oneState = buildRACStates()

func buildRACStates() ([256]byte, [256]byte) {
	var zero, one [256]byte
	const one32 int64 = 1 << 32
	const maxP = 256 - 8
	// int(0.05 * (1<<32)) as used by FFmpeg.
	const factor int64 = 214748364

	lastP8 := 0
	p := one32 / 2
	for i := 0; i < 128; i++ {
		p8 := int((256*p + one32/2) >> 32)
		if p8 <= lastP8 {
			p8 = lastP8 + 1
		}
		if lastP8 != 0 && lastP8 < 256 && p8 <= maxP {
			one[lastP8] = byte(p8)
		}
		p += ((one32-p)*factor + one32/2) >> 32
		lastP8 = p8
	}

	for i := 256 - maxP; i <= maxP; i++ {
		if one[i] != 0 {
			continue
		}
		p := (int64(i)*one32 + 128) >> 8
		p += ((one32-p)*factor + one32/2) >> 32
		p8 := int((256*p + one32/2) >> 32)
		if p8 <= i {
			p8 = i + 1
		}
		if p8 > maxP {
			p8 = maxP
		}
		one[i] = byte(p8)
	}

	for i := 1; i < 255; i++ {
		zero[i] = byte(256 - int(one[256-i]))
	}
	return zero, one
}

// RangeDecoder is FFmpeg's byte-oriented range decoder as used by FFV1.
type RangeDecoder struct {
	data []byte
	pos  int
	end  int
	low  uint32
	rng  uint32
}

// NewRangeDecoder initializes a decoder over an FFV1 byte stream.
func NewRangeDecoder(data []byte) *RangeDecoder {
	d := &RangeDecoder{
		data: data,
		pos:  2,
		end:  len(data),
		rng:  0xFF00,
	}
	if len(data) >= 2 {
		d.low = uint32(data[0])<<8 | uint32(data[1])
	}
	if d.low >= 0xFF00 {
		d.low = 0xFF00
		d.end = d.pos
	}
	if d.pos > d.end {
		d.pos = d.end
	}
	return d
}

// SetEnd limits refills to data[:end].
func (d *RangeDecoder) SetEnd(end int) {
	if end < d.pos {
		end = d.pos
	}
	if end > len(d.data) {
		end = len(d.data)
	}
	d.end = end
}

func (d *RangeDecoder) refill() {
	if d.rng >= 0x100 {
		return
	}
	d.rng <<= 8
	d.low <<= 8
	if d.pos < d.end {
		d.low += uint32(d.data[d.pos])
	}
	d.pos++
}

// bit decodes one binary symbol with an adaptive probability state.
func (d *RangeDecoder) bit(state *byte) int {
	r1 := (d.rng * uint32(*state)) >> 8
	d.rng -= r1
	var b int
	if d.low < d.rng {
		*state = zeroState[*state]
		b = 0
	} else {
		d.low -= d.rng
		*state = oneState[*state]
		d.rng = r1
		b = 1
	}
	d.refill()
	return b
}

// symbol decodes FFV1's range non-binary value. Context bytes are the
// same 32-byte adaptive state block used by FFmpeg's get_symbol_inline.
func (d *RangeDecoder) symbol(state *[32]byte, signed bool) (int, error) {
	if d.bit(&state[0]) != 0 {
		return 0, nil
	}
	e := 0
	for e <= 31 {
		idx := 1 + e
		if idx > 10 {
			idx = 10
		}
		if d.bit(&state[idx]) == 0 {
			break
		}
		e++
	}
	if e > 31 {
		return 0, errSymbolTooLarge
	}
	a := 1
	for i := 0; i < e; i++ {
		// The encoder writes the most significant extra bit first,
		// using state 22+e-1 down to 22.
		a = a*2 + d.bit(&state[22+(e-1-i)])
	}
	if !signed {
		return a, nil
	}
	if d.bit(&state[11+min(e, 10)]) != 0 {
		return -a, nil
	}
	return a, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
