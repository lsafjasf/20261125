// Package fixture creates deterministic Matroska/FFV1 variants used by the
// repository's tests. It mutates block payloads and recomputes container
// CRC-32 elements entirely in Go.
package fixture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"

	"ffv1compare/internal/ebml"
	"ffv1compare/internal/ffv1"
)

type element struct {
	id         uint32
	offset     int
	dataOffset int
	end        int
	headerSize int
}

func readElement(data []byte, off, limit int) (element, error) {
	if off >= limit || off >= len(data) {
		return element{}, fmt.Errorf("element offset %d out of range", off)
	}
	idWidth := ebml.IDWidth(data[off])
	if idWidth == 0 || idWidth > 4 || off+idWidth > limit {
		return element{}, fmt.Errorf("invalid element id at %d", off)
	}
	var id uint32
	for i := 0; i < idWidth; i++ {
		id = id<<8 | uint32(data[off+i])
	}
	sizeOff := off + idWidth
	sizeWidth := ebml.IDWidth(data[sizeOff])
	if sizeWidth == 0 || sizeWidth > 8 || sizeOff+sizeWidth > limit {
		return element{}, fmt.Errorf("invalid element size at %d", sizeOff)
	}
	size, _, unknown, err := ebml.ReadVINT(data[sizeOff : sizeOff+sizeWidth])
	if err != nil {
		return element{}, err
	}
	dataOff := sizeOff + sizeWidth
	end := limit
	if !unknown {
		if size > uint64(limit-dataOff) {
			return element{}, fmt.Errorf("element %s at %d overruns parent", ebml.Name(id), off)
		}
		end = dataOff + int(size)
	}
	return element{
		id:         id,
		offset:     off,
		dataOffset: dataOff,
		end:        end,
		headerSize: dataOff - off,
	}, nil
}

func children(data []byte, start, end int) ([]element, error) {
	var out []element
	for off := start; off < end; {
		e, err := readElement(data, off, end)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
		if e.end <= off {
			return nil, fmt.Errorf("element at %d does not advance", off)
		}
		off = e.end
	}
	return out, nil
}

func segment(data []byte) (element, error) {
	elems, err := children(data, 0, len(data))
	if err != nil {
		return element{}, err
	}
	for _, e := range elems {
		if e.id == ebml.IDSegment {
			return e, nil
		}
	}
	return element{}, fmt.Errorf("Segment not found")
}

// Copy writes a byte-identical copy of src to dst.
func Copy(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// ReorderSlices swaps two slice blobs (including their footers) in every
// SimpleBlock/Block frame. Each FFV1 slice header carries its own raster
// coordinates, so decoded pixels remain unchanged.
func ReorderSlices(src, dst string, swapA, swapB int) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	seg, err := segment(data)
	if err != nil {
		return err
	}
	blocks, err := findBlocks(data, seg.dataOffset, seg.end)
	if err != nil {
		return err
	}
	changed := 0
	for _, b := range blocks {
		payload := data[b.dataOffset:b.end]
		_, trackWidth, _, err := ebml.ReadVINT(payload)
		if err != nil || trackWidth+3 > len(payload) {
			return fmt.Errorf("block at %d: invalid header", b.offset)
		}
		sample := payload[trackWidth+3:]
		report := ffv1.AnalyzeSlices(sample, 0)
		if report.Error != "" || !report.CRCEnabled || report.Count <= swapA || report.Count <= swapB {
			continue
		}
		blobs := make([][]byte, report.Count)
		for i, s := range report.Slices {
			blobs[i] = sample[s.Start:s.End]
		}
		blobs[swapA], blobs[swapB] = blobs[swapB], blobs[swapA]
		var rebuilt bytes.Buffer
		for _, blob := range blobs {
			rebuilt.Write(blob)
		}
		if rebuilt.Len() != len(sample) {
			return fmt.Errorf("block at %d: reorder changed sample size", b.offset)
		}
		copy(sample, rebuilt.Bytes())
		changed++
	}
	if changed == 0 {
		return fmt.Errorf("no reorderable FFV1 blocks found")
	}
	if err := rewriteContainerCRCs(data, seg); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// ShiftTimecodes adds delta to every block-relative timecode. The 16-bit
// field has a fixed size, so this changes PTS without touching sample bytes.
func ShiftTimecodes(src, dst string, delta int16) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	seg, err := segment(data)
	if err != nil {
		return err
	}
	blocks, err := findBlocks(data, seg.dataOffset, seg.end)
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return fmt.Errorf("no blocks found")
	}
	for _, b := range blocks {
		payload := data[b.dataOffset:b.end]
		_, trackWidth, _, err := ebml.ReadVINT(payload)
		if err != nil || trackWidth+3 > len(payload) {
			return fmt.Errorf("block at %d: invalid header", b.offset)
		}
		relOff := b.dataOffset + trackWidth
		rel := int16(binary.BigEndian.Uint16(data[relOff : relOff+2]))
		binary.BigEndian.PutUint16(data[relOff:relOff+2], uint16(rel+delta))
	}
	if err := rewriteContainerCRCs(data, seg); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func findBlocks(data []byte, start, end int) ([]element, error) {
	var out []element
	elems, err := children(data, start, end)
	if err != nil {
		return nil, err
	}
	for _, e := range elems {
		switch e.id {
		case ebml.IDSimpleBlock, ebml.IDBlock:
			out = append(out, e)
		}
		if ebml.MasterIDs[e.id] {
			sub, err := findBlocks(data, e.dataOffset, e.end)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		}
	}
	return out, nil
}

func rewriteContainerCRCs(data []byte, seg element) error {
	return fixMaster(data, seg.dataOffset, seg.end)
}

func fixMaster(data []byte, start, end int) error {
	elems, err := children(data, start, end)
	if err != nil {
		return err
	}
	for _, e := range elems {
		if ebml.MasterIDs[e.id] {
			if err := fixMaster(data, e.dataOffset, e.end); err != nil {
				return err
			}
		}
	}
	for _, e := range elems {
		if e.id != ebml.IDCRC32 || e.end-e.dataOffset != 4 {
			continue
		}
		sum := crc32.ChecksumIEEE(data[e.end:end])
		if e.dataOffset+4 > len(data) {
			return fmt.Errorf("CRC element at %d overruns file", e.offset)
		}
		binary.LittleEndian.PutUint32(data[e.dataOffset:e.dataOffset+4], sum)
	}
	return nil
}
