package sps

import (
	"encoding/binary"
	"fmt"
)

// ColorFromConfig extracts bitstream colour info from a codec
// configuration record (avcC, hvcC or vpcC payload).
func ColorFromConfig(configType string, d []byte) (*Color, error) {
	switch configType {
	case "avcC":
		return colorFromAvcC(d)
	case "hvcC":
		return colorFromHvcC(d)
	case "vpcC":
		return colorFromVpcC(d)
	}
	return nil, fmt.Errorf("unsupported config type %q", configType)
}

func colorFromAvcC(d []byte) (*Color, error) {
	if len(d) < 7 {
		return nil, fmt.Errorf("avcC too short")
	}
	numSPS := int(d[5] & 0x1f)
	off := 6
	for i := 0; i < numSPS; i++ {
		if off+2 > len(d) {
			return nil, fmt.Errorf("avcC SPS %d truncated", i)
		}
		n := int(binary.BigEndian.Uint16(d[off : off+2]))
		off += 2
		if off+n > len(d) {
			return nil, fmt.Errorf("avcC SPS %d overruns record", i)
		}
		if c, err := ParseH264SPS(d[off : off+n]); err == nil {
			return c, nil
		}
		off += n
	}
	return nil, ErrNoVUI
}

func colorFromHvcC(d []byte) (*Color, error) {
	if len(d) < 23 {
		return nil, fmt.Errorf("hvcC too short")
	}
	numArrays := int(d[22])
	off := 23
	for a := 0; a < numArrays; a++ {
		if off+3 > len(d) {
			return nil, fmt.Errorf("hvcC array %d truncated", a)
		}
		nalType := d[off] & 0x3f
		numNalus := int(binary.BigEndian.Uint16(d[off+1 : off+3]))
		off += 3
		for i := 0; i < numNalus; i++ {
			if off+2 > len(d) {
				return nil, fmt.Errorf("hvcC nalu %d truncated", i)
			}
			n := int(binary.BigEndian.Uint16(d[off : off+2]))
			off += 2
			if off+n > len(d) {
				return nil, fmt.Errorf("hvcC nalu %d overruns record", i)
			}
			if nalType == 33 {
				if c, err := ParseH265SPS(d[off : off+n]); err == nil {
					return c, nil
				}
			}
			off += n
		}
	}
	return nil, ErrNoVUI
}

// colorFromVpcC reads the colour fields of a VP9 codec configuration box.
// Layout: version/flags(4) profile(1) level(1) bitDepth/chroma/subsampling(1)
// chromaSubsampling+codecInitialisationDataSize(1) ... then primaries(1)
// transfer(1) matrix(1) fullRange(1).
func colorFromVpcC(d []byte) (*Color, error) {
	if len(d) < 12 {
		return nil, fmt.Errorf("vpcC too short")
	}
	return &Color{
		Primaries: uint16(d[8]),
		Transfer:  uint16(d[9]),
		Matrix:    uint16(d[10]),
		FullRange: d[11] != 0,
	}, nil
}
