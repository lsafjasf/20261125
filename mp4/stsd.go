package mp4

import (
	"encoding/binary"
	"fmt"
)

// videoSampleEntryHeader is the offset where child boxes of a visual
// sample entry begin: 8 (box header) + 6 reserved + 2 dref + 70 visual
// fields.
const videoSampleEntryHeader = 86

// parseStsd parses a Sample Table Description box payload.
func parseStsd(d []byte) ([]SampleDescription, error) {
	if len(d) < 8 {
		return nil, fmt.Errorf("stsd too short: %d bytes", len(d))
	}
	count := binary.BigEndian.Uint32(d[4:8])
	off := 8
	var out []SampleDescription
	for i := uint32(0); i < count; i++ {
		if off+8 > len(d) {
			return nil, fmt.Errorf("stsd entry %d truncated", i)
		}
		size := int(binary.BigEndian.Uint32(d[off : off+4]))
		format := string(d[off+4 : off+8])
		if size < 8 || off+size > len(d) {
			return nil, fmt.Errorf("stsd entry %d (%s) bad size %d", i, format, size)
		}
		entry := d[off : off+size]
		sd := SampleDescription{Format: format}
		if isVideoFormat(format) && len(entry) >= videoSampleEntryHeader {
			sd.Width = int(binary.BigEndian.Uint16(entry[32:34]))
			sd.Height = int(binary.BigEndian.Uint16(entry[34:36]))
			parseEntryChildren(&sd, entry[videoSampleEntryHeader:])
		}
		out = append(out, sd)
		off += size
	}
	return out, nil
}

func isVideoFormat(f string) bool {
	switch f {
	case "avc1", "avc3", "hev1", "hvc1", "vp08", "vp09", "av01",
		"mp4v", "dvh1", "dvhe", "h263", "s263":
		return true
	}
	return false
}

// parseEntryChildren scans child boxes of a visual sample entry for
// colour (colr) and codec configuration (avcC/hvcC/vpcC/av1C) boxes.
func parseEntryChildren(sd *SampleDescription, d []byte) {
	for len(d) >= 8 {
		size := int(binary.BigEndian.Uint32(d[0:4]))
		typ := string(d[4:8])
		if size < 8 || size > len(d) {
			break
		}
		payload := d[8:size]
		switch typ {
		case "colr":
			if ci, colrType, ok := parseColr(payload); ok {
				sd.ContainerColr = ci
				sd.ColrType = colrType
			}
		case "avcC", "hvcC", "vpcC", "av1C":
			sd.CodecConfig = append([]byte(nil), payload...)
			sd.ConfigType = typ
		}
		d = d[size:]
	}
}

// parseColr parses a colr box (nclx or nclc parameterisation).
func parseColr(d []byte) (*ColorInfo, string, bool) {
	if len(d) < 4 {
		return nil, "", false
	}
	method := string(d[0:4])
	switch method {
	case "nclx":
		if len(d) < 11 {
			return nil, "", false
		}
		ci := &ColorInfo{
			Primaries: binary.BigEndian.Uint16(d[4:6]),
			Transfer:  binary.BigEndian.Uint16(d[6:8]),
			Matrix:    binary.BigEndian.Uint16(d[8:10]),
			Source:    "colr/nclx",
		}
		// Full-range flag is the top bit of the final byte; it may be
		// absent in files written against the older 9-byte layout.
		if len(d) >= 11 {
			ci.FullRange = d[10]&0x80 != 0
			ci.RangeKnown = true
		}
		nameColor(ci)
		return ci, "nclx", true
	case "nclc":
		if len(d) < 10 {
			return nil, "", false
		}
		ci := &ColorInfo{
			Primaries: binary.BigEndian.Uint16(d[4:6]),
			Transfer:  binary.BigEndian.Uint16(d[6:8]),
			Matrix:    binary.BigEndian.Uint16(d[8:10]),
			Source:    "colr/nclc",
		}
		nameColor(ci)
		return ci, "nclc", true
	}
	return nil, "", false
}

func nameColor(ci *ColorInfo) {
	ci.PrimariesName = PrimariesName(ci.Primaries)
	ci.TransferName = TransferName(ci.Transfer)
	ci.MatrixName = MatrixName(ci.Matrix)
}

// NameColor exposes name filling for bitstream-derived ColorInfo.
func NameColor(ci *ColorInfo) { nameColor(ci) }
