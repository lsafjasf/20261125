package main

import (
	"fmt"
	"io"
)

type stscRun struct {
	FirstChunk      int
	SamplesPerChunk int
}

func tableBox(stbl *boxNode, typ string) *boxNode {
	return stbl.child(typ)
}

func parseSTSZ(r io.ReaderAt, stbl *boxNode) (sizes []uint32, uniform uint32, count int, err error) {
	box := tableBox(stbl, "stsz")
	if box == nil {
		return nil, 0, 0, fmt.Errorf("missing stsz/stz2")
	}
	data, err := readFullBoxPayload(r, box)
	if err != nil {
		return nil, 0, 0, err
	}
	if box.Type == "stz2" {
		if len(data) < 8 {
			return nil, 0, 0, fmt.Errorf("short stz2 box")
		}
		fieldSize := data[3]
		count = int(beUint32(data, 4))
		if count > maxSamples {
			return nil, 0, 0, fmt.Errorf("sample count %d exceeds limit", count)
		}
		sizes = make([]uint32, count)
		pos := 8
		for i := range sizes {
			switch fieldSize {
			case 8:
				if pos >= len(data) {
					return nil, 0, 0, fmt.Errorf("truncated stz2 entry %d", i)
				}
				sizes[i] = uint32(data[pos])
				pos++
			case 16:
				if pos+2 > len(data) {
					return nil, 0, 0, fmt.Errorf("truncated stz2 entry %d", i)
				}
				sizes[i] = uint32(beUint16(data, pos))
				pos += 2
			default:
				return nil, 0, 0, fmt.Errorf("unsupported stz2 field size %d", fieldSize)
			}
		}
		return sizes, 0, count, nil
	}

	if len(data) < 8 {
		return nil, 0, 0, fmt.Errorf("short stsz box")
	}
	uniform = beUint32(data, 0)
	count32 := beUint32(data, 4)
	if count32 > maxSamples {
		return nil, 0, 0, fmt.Errorf("sample count %d exceeds limit", count32)
	}
	count = int(count32)
	if uniform != 0 {
		return nil, uniform, count, nil
	}
	if len(data) < 8+4*count {
		return nil, 0, 0, fmt.Errorf("truncated stsz: need %d sizes, have %d bytes", count, len(data)-8)
	}
	sizes = make([]uint32, count)
	for i := range sizes {
		sizes[i] = beUint32(data, 8+i*4)
	}
	return sizes, 0, count, nil
}

func parseChunkOffsets(r io.ReaderAt, stbl *boxNode) ([]int64, error) {
	if box := tableBox(stbl, "stco"); box != nil {
		data, err := readFullBoxPayload(r, box)
		if err != nil {
			return nil, err
		}
		if len(data) < 4 {
			return nil, fmt.Errorf("short stco box")
		}
		count := int(beUint32(data, 0))
		if len(data) < 4+4*count {
			return nil, fmt.Errorf("truncated stco: want %d entries", count)
		}
		out := make([]int64, count)
		for i := range out {
			out[i] = int64(beUint32(data, 4+i*4))
		}
		return out, nil
	}

	box := tableBox(stbl, "co64")
	if box == nil {
		return nil, fmt.Errorf("missing stco/co64 chunk offsets")
	}
	data, err := readFullBoxPayload(r, box)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("short co64 box")
	}
	count := int(beUint32(data, 0))
	if len(data) < 4+8*count {
		return nil, fmt.Errorf("truncated co64: want %d entries", count)
	}
	out := make([]int64, count)
	for i := range out {
		out[i] = int64(beUint64(data, 4+i*8))
	}
	return out, nil
}

func parseSTSC(r io.ReaderAt, stbl *boxNode) ([]stscRun, error) {
	box := tableBox(stbl, "stsc")
	if box == nil {
		return nil, fmt.Errorf("missing stsc")
	}
	data, err := readFullBoxPayload(r, box)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("short stsc box")
	}
	count := int(beUint32(data, 0))
	if len(data) < 4+12*count {
		return nil, fmt.Errorf("truncated stsc: want %d entries", count)
	}
	runs := make([]stscRun, count)
	for i := range runs {
		off := 4 + i*12
		runs[i] = stscRun{
			FirstChunk:      int(beUint32(data, off)),
			SamplesPerChunk: int(beUint32(data, off+4)),
		}
		if runs[i].FirstChunk < 1 || runs[i].SamplesPerChunk < 1 {
			return nil, fmt.Errorf("invalid stsc entry %d", i+1)
		}
	}
	return runs, nil
}
