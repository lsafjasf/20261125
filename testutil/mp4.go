package testutil

import (
	"bytes"
	"encoding/binary"
)

// box assembles one ISO-BMFF box.
func box(typ string, payload []byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(8+len(payload)))
	b.WriteString(typ)
	b.Write(payload)
	return b.Bytes()
}

func fullBox(typ string, version byte, flags uint32, payload []byte) []byte {
	head := []byte{version, byte(flags >> 16), byte(flags >> 8), byte(flags)}
	return box(typ, append(head, payload...))
}

func colrNclx(primaries, transfer, matrix uint16, fullRange bool) []byte {
	var b bytes.Buffer
	b.WriteString("nclx")
	binary.Write(&b, binary.BigEndian, primaries)
	binary.Write(&b, binary.BigEndian, transfer)
	binary.Write(&b, binary.BigEndian, matrix)
	if fullRange {
		b.WriteByte(0x80)
	} else {
		b.WriteByte(0x00)
	}
	return box("colr", b.Bytes())
}

// visualSampleEntry builds a video stsd entry with optional child boxes.
func visualSampleEntry(format string, width, height uint16, children ...[]byte) []byte {
	var e bytes.Buffer
	e.Write(make([]byte, 6))                      // reserved
	binary.Write(&e, binary.BigEndian, uint16(1)) // data_reference_index
	e.Write(make([]byte, 16))                     // pre_defined + reserved
	binary.Write(&e, binary.BigEndian, width)
	binary.Write(&e, binary.BigEndian, height)
	binary.Write(&e, binary.BigEndian, uint32(0x00480000)) // horizresolution
	binary.Write(&e, binary.BigEndian, uint32(0x00480000)) // vertresolution
	e.Write(make([]byte, 4))                               // reserved
	binary.Write(&e, binary.BigEndian, uint16(1))          // frame_count
	e.Write(make([]byte, 32))                              // compressorname
	binary.Write(&e, binary.BigEndian, uint16(0x0018))     // depth
	binary.Write(&e, binary.BigEndian, int16(-1))          // pre_defined
	for _, c := range children {
		e.Write(c)
	}
	return box(format, e.Bytes())
}

func stsd(entries ...[]byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(len(entries)))
	for _, e := range entries {
		b.Write(e)
	}
	return fullBox("stsd", 0, 0, b.Bytes())
}

func tkhd(trackID uint32) []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 8)) // ctime, mtime
	binary.Write(&b, binary.BigEndian, trackID)
	b.Write(make([]byte, 4))  // reserved
	b.Write(make([]byte, 4))  // duration
	b.Write(make([]byte, 8))  // reserved
	b.Write(make([]byte, 8))  // layer, alt_group, volume, reserved
	b.Write(make([]byte, 36)) // matrix
	binary.Write(&b, binary.BigEndian, uint32(1920<<16))
	binary.Write(&b, binary.BigEndian, uint32(1080<<16))
	return fullBox("tkhd", 0, 3, b.Bytes())
}

func mdhd() []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 8)) // ctime, mtime
	binary.Write(&b, binary.BigEndian, uint32(90000))
	binary.Write(&b, binary.BigEndian, uint32(90000))
	b.Write(make([]byte, 4)) // language + pre_defined
	return fullBox("mdhd", 0, 0, b.Bytes())
}

func hdlr(handler string) []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 4)) // pre_defined
	b.WriteString(handler)
	b.Write(make([]byte, 12)) // reserved
	b.WriteByte(0)            // name
	return fullBox("hdlr", 0, 0, b.Bytes())
}

func videoTrak(trackID uint32, entries ...[]byte) []byte {
	vmhd := fullBox("vmhd", 0, 1, make([]byte, 8))
	stbl := box("stbl", stsd(entries...))
	minf := box("minf", append(vmhd, stbl...))
	mdia := box("mdia", bytes.Join([][]byte{mdhd(), hdlr("vide"), minf}, nil))
	return box("trak", append(tkhd(trackID), mdia...))
}

// buildMP4 assembles ftyp+moov from the given traks.
func buildMP4(traks ...[]byte) []byte {
	ftyp := box("ftyp", []byte("isom\x00\x00\x02\x00isomiso2"))
	mvhd := fullBox("mvhd", 0, 0, make([]byte, 96))
	moovPayload := append(mvhd, bytes.Join(traks, nil)...)
	return append(ftyp, box("moov", moovPayload)...)
}
