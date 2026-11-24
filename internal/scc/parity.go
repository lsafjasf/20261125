package scc

// OddParity reports whether b has an odd number of set bits, as required
// for CEA-608 bytes carried in SCC (bit 7 is the odd-parity bit).
func OddParity(b byte) bool {
	b ^= b >> 4
	b ^= b >> 2
	b ^= b >> 1
	return b&1 == 1
}

// StripParity clears the parity bit, yielding the 7-bit code value.
func StripParity(b byte) byte { return b & 0x7F }
