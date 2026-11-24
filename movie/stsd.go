package movie

import "encoding/binary"

// parseSTSD extracts the first video sample entry codec and dimensions.
// raw is the full stsd box including its 8 byte header.
func parseSTSD(raw []byte) (codec string, width, height uint32) {
	if len(raw) < 16 {
		return "", 0, 0
	}
	// box header 8 + version/flags 4 + entry_count 4
	entryOff := 16
	if entryOff+8 > len(raw) {
		return "", 0, 0
	}
	size := int(binary.BigEndian.Uint32(raw[entryOff : entryOff+4]))
	codec = string(raw[entryOff+4 : entryOff+8])
	if size < 8 || entryOff+size > len(raw) {
		// Declared size inconsistent; still try fixed offsets.
		size = len(raw) - entryOff
	}
	payload := entryOff + 8
	if payload+28 <= len(raw) {
		width = uint32(binary.BigEndian.Uint16(raw[payload+24 : payload+26]))
		height = uint32(binary.BigEndian.Uint16(raw[payload+26 : payload+28]))
	}
	return codec, width, height
}

// parseAvcLengthSize locates avcC/hvcC inside the first sample entry and
// returns the configured NAL length prefix size (1, 2 or 4).
func parseAvcLengthSize(stsdRaw []byte) int {
	if len(stsdRaw) < 16+8 {
		return 0
	}
	entryOff := 16
	entrySize := int(binary.BigEndian.Uint32(stsdRaw[entryOff : entryOff+4]))
	if entrySize < 8 || entryOff+entrySize > len(stsdRaw) {
		entrySize = len(stsdRaw) - entryOff
	}
	entryEnd := entryOff + entrySize

	// VisualSampleEntry fixed fields occupy 78 payload bytes before children.
	child := entryOff + 8 + 78
	for child+8 <= entryEnd {
		csize := int(binary.BigEndian.Uint32(stsdRaw[child : child+4]))
		ctype := string(stsdRaw[child+4 : child+8])
		if csize < 8 || child+csize > entryEnd {
			break
		}
		payload := child + 8
		switch ctype {
		case "avcC":
			// payload[4] holds lengthSizeMinusOne in its low 2 bits.
			if payload+5 <= child+csize {
				return int(stsdRaw[payload+4]&0x03) + 1
			}
		case "hvcC":
			// lengthSizeMinusOne lives in low 2 bits of byte 21.
			if payload+22 <= child+csize {
				return int(stsdRaw[payload+21]&0x03) + 1
			}
		}
		child += csize
	}
	return 4
}
