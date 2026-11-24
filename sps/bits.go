package sps

import "errors"

// ErrNoVUI is returned when the SPS carries no VUI colour description.
var ErrNoVUI = errors.New("no VUI colour description present")

// bitReader reads bits MSB-first from a byte slice.
type bitReader struct {
	data []byte
	pos  int // bit position
	err  error
}

func newBitReader(b []byte) *bitReader { return &bitReader{data: b} }

func (r *bitReader) bitsLeft() int { return len(r.data)*8 - r.pos }

func (r *bitReader) u(n int) uint64 {
	if r.err != nil {
		return 0
	}
	if n > 64 || r.bitsLeft() < n {
		r.err = errors.New("bitstream overrun")
		return 0
	}
	var v uint64
	for i := 0; i < n; i++ {
		byteIdx := (r.pos + i) / 8
		bitIdx := 7 - (r.pos+i)%8
		v = v<<1 | uint64((r.data[byteIdx]>>bitIdx)&1)
	}
	r.pos += n
	return v
}

func (r *bitReader) ue() uint64 {
	if r.err != nil {
		return 0
	}
	zeros := 0
	for {
		if r.bitsLeft() < 1 {
			r.err = errors.New("bitstream overrun in ue(v)")
			return 0
		}
		if r.u(1) == 1 {
			break
		}
		zeros++
		if zeros > 63 {
			r.err = errors.New("ue(v) prefix too long")
			return 0
		}
	}
	if zeros == 0 {
		return 0
	}
	return (1 << zeros) - 1 + r.u(zeros)
}

func (r *bitReader) se() int64 {
	k := r.ue()
	if k%2 == 0 {
		return -int64(k / 2)
	}
	return int64(k+1) / 2
}

// ebspToRbsp strips emulation-prevention bytes (00 00 03 -> 00 00).
func ebspToRbsp(b []byte) []byte {
	out := make([]byte, 0, len(b))
	zeros := 0
	for _, c := range b {
		if zeros >= 2 && c == 0x03 {
			zeros = 0
			continue
		}
		if c == 0 {
			zeros++
		} else {
			zeros = 0
		}
		out = append(out, c)
	}
	return out
}
