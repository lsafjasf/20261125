package repair

import (
	"encoding/binary"

	"mp4index/bmff"
)

type moovParams struct {
	timescale  uint32
	duration   uint32 // fits v0 mvhd/mdhd (test/sample scale)
	trackID    uint32
	width      uint32 // pixels
	height     uint32
	stsd       []byte // complete stsd box
	durations  []uint32
	ctts       []int32
	sizes      []uint32
	offsets    []uint64
	keyframes  []uint32
	force64    bool
	extraTraks [][]byte
}

func buildMoov(p moovParams) []byte {
	timescale := p.timescale
	if timescale == 0 {
		timescale = 15360
	}
	trackID := p.trackID
	if trackID == 0 {
		trackID = 1
	}

	mvhd := buildMVHD(timescale, p.duration)
	trak := buildTrak(p, trackID, timescale)
	payload := append(append([]byte{}, mvhd...), trak...)
	for _, extra := range p.extraTraks {
		payload = append(payload, extra...)
	}
	return bmff.BuildBox("moov", payload)
}

func buildMVHD(timescale, duration uint32) []byte {
	// Offsets are relative to the payload after version+flags (4 bytes).
	payload := make([]byte, 96)
	bmff.PutU32(payload[8:12], timescale)
	bmff.PutU32(payload[12:16], duration)
	bmff.PutU32(payload[16:20], 0x00010000) // rate 1.0
	payload[20] = 0x01
	payload[21] = 0x00 // volume 1.0
	putMatrix(payload[32:68])
	bmff.PutU32(payload[92:96], 2) // next track id
	return bmff.BuildFullBox("mvhd", 0, 0, payload)
}

func buildTrak(p moovParams, trackID, timescale uint32) []byte {
	// Offsets exclude the 4 byte fullbox header.
	tkhdPayload := make([]byte, 80)
	bmff.PutU32(tkhdPayload[8:12], trackID)
	bmff.PutU32(tkhdPayload[16:20], p.duration)
	binary.BigEndian.PutUint16(tkhdPayload[28:30], 0) // layer
	binary.BigEndian.PutUint16(tkhdPayload[30:32], 0) // alternate group
	tkhdPayload[32] = 0x01
	tkhdPayload[33] = 0x00 // volume 1.0
	putMatrix(tkhdPayload[36:72])
	bmff.PutU32(tkhdPayload[72:76], p.width<<16)
	bmff.PutU32(tkhdPayload[76:80], p.height<<16)
	tkhd := bmff.BuildFullBox("tkhd", 0, 0x000007, tkhdPayload)

	mdhdPayload := make([]byte, 20)
	bmff.PutU32(mdhdPayload[8:12], timescale)
	bmff.PutU32(mdhdPayload[12:16], p.duration)
	binary.BigEndian.PutUint16(mdhdPayload[16:18], 0x55C4) // language "und"
	mdhd := bmff.BuildFullBox("mdhd", 0, 0, mdhdPayload)

	hdlrPayload := make([]byte, 0, 33)
	hdlrPayload = append(hdlrPayload, 0, 0, 0, 0) // pre_defined
	hdlrPayload = append(hdlrPayload, "vide"...)
	hdlrPayload = append(hdlrPayload, make([]byte, 12)...) // reserved
	hdlrPayload = append(hdlrPayload, "VideoHandler\x00"...)
	hdlr := bmff.BuildFullBox("hdlr", 0, 0, hdlrPayload)

	vmhd := bmff.BuildFullBox("vmhd", 1, 1, make([]byte, 8))

	urlBox := bmff.BuildFullBox("url ", 0, 1, nil)
	drefPayload := make([]byte, 4)
	bmff.PutU32(drefPayload[0:4], 1)
	drefPayload = append(drefPayload, urlBox...)
	dref := bmff.BuildFullBox("dref", 0, 0, drefPayload)
	dinf := bmff.BuildBox("dinf", dref)

	stbl := buildSTBL(p)

	mdia := bmff.BuildBox("mdia", append(append(append([]byte{}, mdhd...), hdlr...),
		bmff.BuildBox("minf", append(append([]byte{}, vmhd...), append(dinf, stbl...)...))...))
	return bmff.BuildBox("trak", append(tkhd, mdia...))
}

func buildSTBL(p moovParams) []byte {
	var children []byte
	if len(p.stsd) > 0 {
		children = append(children, p.stsd...)
	} else {
		children = append(children, minimalSTSD()...)
	}
	children = append(children, buildSTTS(p.durations)...)
	if len(p.ctts) > 0 {
		children = append(children, buildCTTS(p.ctts)...)
	}
	children = append(children, buildSTSC()...)
	children = append(children, buildSTSZ(p.sizes)...)
	if p.force64 || (len(p.offsets) > 0 && maxUint64(p.offsets) >= uint64(1<<32)) {
		children = append(children, buildCO64(p.offsets)...)
	} else {
		children = append(children, buildSTCO(p.offsets)...)
	}
	if len(p.keyframes) > 0 {
		children = append(children, buildSTSS(p.keyframes)...)
	}
	return bmff.BuildBox("stbl", children)
}

func buildSTTS(durations []uint32) []byte {
	type run struct {
		count    uint32
		duration uint32
	}
	var runs []run
	for _, d := range durations {
		if len(runs) > 0 && runs[len(runs)-1].duration == d {
			runs[len(runs)-1].count++
		} else {
			runs = append(runs, run{1, d})
		}
	}
	payload := make([]byte, 4+8*len(runs))
	bmff.PutU32(payload[0:4], uint32(len(runs)))
	for i, r := range runs {
		off := 4 + i*8
		bmff.PutU32(payload[off:off+4], r.count)
		bmff.PutU32(payload[off+4:off+8], r.duration)
	}
	return bmff.BuildFullBox("stts", 0, 0, payload)
}

// buildCTTS run-length encodes per-sample composition offsets.
func buildCTTS(offsets []int32) []byte {
	type run struct {
		count  uint32
		offset int32
	}
	var runs []run
	negative := false
	for _, o := range offsets {
		if o < 0 {
			negative = true
		}
		if len(runs) > 0 && runs[len(runs)-1].offset == o {
			runs[len(runs)-1].count++
		} else {
			runs = append(runs, run{1, o})
		}
	}
	payload := make([]byte, 4+8*len(runs))
	bmff.PutU32(payload[0:4], uint32(len(runs)))
	for i, r := range runs {
		off := 4 + i*8
		bmff.PutU32(payload[off:off+4], r.count)
		binary.BigEndian.PutUint32(payload[off+4:off+8], uint32(r.offset))
	}
	version := byte(0)
	if negative {
		version = 1
	}
	return bmff.BuildFullBox("ctts", version, 0, payload)
}

func buildSTSC() []byte {
	payload := make([]byte, 16)
	bmff.PutU32(payload[0:4], 1)   // entry count
	bmff.PutU32(payload[4:8], 1)   // first_chunk
	bmff.PutU32(payload[8:12], 1)  // samples_per_chunk
	bmff.PutU32(payload[12:16], 1) // sample_description_index
	return bmff.BuildFullBox("stsc", 0, 0, payload)
}

func buildSTSZ(sizes []uint32) []byte {
	payload := make([]byte, 8+4*len(sizes))
	bmff.PutU32(payload[0:4], 0) // uniform sample size
	bmff.PutU32(payload[4:8], uint32(len(sizes)))
	for i, s := range sizes {
		bmff.PutU32(payload[8+4*i:12+4*i], s)
	}
	return bmff.BuildFullBox("stsz", 0, 0, payload)
}

func buildSTCO(offsets []uint64) []byte {
	payload := make([]byte, 4+4*len(offsets))
	bmff.PutU32(payload[0:4], uint32(len(offsets)))
	for i, o := range offsets {
		bmff.PutU32(payload[4+4*i:8+4*i], uint32(o))
	}
	return bmff.BuildFullBox("stco", 0, 0, payload)
}

func buildCO64(offsets []uint64) []byte {
	payload := make([]byte, 4+8*len(offsets))
	bmff.PutU32(payload[0:4], uint32(len(offsets)))
	for i, o := range offsets {
		binary.BigEndian.PutUint64(payload[4+8*i:12+8*i], o)
	}
	return bmff.BuildFullBox("co64", 0, 0, payload)
}

func buildSTSS(keys []uint32) []byte {
	payload := make([]byte, 4+4*len(keys))
	bmff.PutU32(payload[0:4], uint32(len(keys)))
	for i, k := range keys {
		bmff.PutU32(payload[4+4*i:8+4*i], k)
	}
	return bmff.BuildFullBox("stss", 0, 0, payload)
}

func putMatrix(b []byte) {
	// ISO identity transformation matrix in 16.16 / 2.30 fixed point.
	vals := []uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000}
	for i, v := range vals {
		bmff.PutU32(b[4*i:4*i+4], v)
	}
}

func maxUint64(vs []uint64) uint64 {
	var m uint64
	for _, v := range vs {
		if v > m {
			m = v
		}
	}
	return m
}

// minimalSTSD is a last resort video sample description. Real repairs normally
// reuse the original stsd (avcC) instead.
func minimalSTSD() []byte {
	// Raw avc1 visual entry with empty avcC is not decodable but keeps boxes
	// structurally valid; callers should prefer the source stsd.
	entryPayload := make([]byte, 78)
	binary.BigEndian.PutUint16(entryPayload[24:26], 320)
	binary.BigEndian.PutUint16(entryPayload[26:28], 240)
	bmff.PutU32(entryPayload[28:32], 0x00480000)       // horiz resolution 72 dpi
	bmff.PutU32(entryPayload[32:36], 0x00480000)       // vert resolution
	binary.BigEndian.PutUint16(entryPayload[40:42], 1) // frame count
	entry := bmff.BuildBox("avc1", entryPayload)
	buf := make([]byte, 8)
	bmff.PutU32(buf[0:4], 1) // entry count
	buf = append(buf, entry...)
	return bmff.BuildFullBox("stsd", 0, 0, buf)
}
