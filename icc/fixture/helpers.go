package fixture

import "encoding/binary"

// tagOrder returns the table signature order: every distinct payload once
// (sorted for reproducibility), followed by any explicit duplicate entries.
func (b *Builder) tagOrder() []string {
	base := make([]string, 0, len(b.payloads))
	for sig := range b.payloads {
		base = append(base, sig)
	}
	for i := 1; i < len(base); i++ {
		for j := i; j > 0 && base[j-1] > base[j]; j-- {
			base[j-1], base[j] = base[j], base[j-1]
		}
	}
	if len(b.order) == 0 {
		return base
	}
	return append(base, b.order...)
}

func (b *Builder) writeHeader(out []byte) {
	binary.BigEndian.PutUint32(out[0:], uint32(len(out)))
	copy(out[4:8], pad4("lcms"))
	binary.BigEndian.PutUint32(out[8:], b.version)
	copy(out[12:16], pad4(b.class))
	copy(out[16:20], pad4(b.color))
	copy(out[20:24], pad4(b.pcs))
	copy(out[36:40], pad4("acsp"))
	putS15(out[68:], b.headerWP[0])
	putS15(out[72:], b.headerWP[1])
	putS15(out[76:], b.headerWP[2])
	copy(out[80:84], pad4("ADBE"))
}

func pad4(s string) []byte {
	p := make([]byte, 4)
	copy(p, s)
	return p
}

func putS15(p []byte, v float64) {
	binary.BigEndian.PutUint32(p, uint32(int32(v*65536.0+0.5)))
}

func u16Fixed16(v float64) uint16 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 65535
	}
	return uint16(v*65535.0 + 0.5)
}

func u8Fixed8(v float64) uint16 {
	return uint16(v*256.0 + 0.5)
}
