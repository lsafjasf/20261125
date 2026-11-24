package ffv1

var rawCRCTable = func() *[256]uint32 {
	var table [256]uint32
	for i := 0; i < 256; i++ {
		c := uint32(i) << 24
		for bit := 0; bit < 8; bit++ {
			if c&0x80000000 != 0 {
				c = (c << 1) ^ 0x04C11DB7
			} else {
				c <<= 1
			}
		}
		table[i] = c
	}
	return &table
}()

// CRC32Raw is FFV1's CRC-32: polynomial 0x04C11DB7, initial value 0,
// no input or output reflection.
func CRC32Raw(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = (crc << 8) ^ rawCRCTable[byte(crc>>24)^b]
	}
	return crc
}
