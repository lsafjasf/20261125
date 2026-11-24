package fixture

import (
	"encoding/binary"

	"mp4index/bmff"
)

type indexRec struct {
	rel  int
	size int
	key  bool
}

func buildMoov(cfg Config, stsd []byte, recs []indexRec, payloadStart int64) []byte {
	n := len(recs)
	duration := uint32(cfg.Frames) * cfg.SampleDuration

	mvhdPayload := make([]byte, 96)
	bmff.PutU32(mvhdPayload[8:12], cfg.Timescale)
	bmff.PutU32(mvhdPayload[12:16], duration)
	bmff.PutU32(mvhdPayload[16:20], 0x00010000)
	mvhdPayload[20] = 1
	putIdentity(mvhdPayload[32:68])
	bmff.PutU32(mvhdPayload[92:96], 2)
	mvhd := bmff.BuildFullBox("mvhd", 0, 0, mvhdPayload)

	tkhdPayload := make([]byte, 80)
	bmff.PutU32(tkhdPayload[8:12], 1)
	bmff.PutU32(tkhdPayload[16:20], duration)
	tkhdPayload[32] = 1
	putIdentity(tkhdPayload[36:72])
	bmff.PutU32(tkhdPayload[72:76], uint32(cfg.Width)<<16)
	bmff.PutU32(tkhdPayload[76:80], uint32(cfg.Height)<<16)
	tkhd := bmff.BuildFullBox("tkhd", 0, 7, tkhdPayload)

	mdhdPayload := make([]byte, 20)
	bmff.PutU32(mdhdPayload[8:12], cfg.Timescale)
	bmff.PutU32(mdhdPayload[12:16], duration)
	binary.BigEndian.PutUint16(mdhdPayload[16:18], 0x55C4)
	mdhd := bmff.BuildFullBox("mdhd", 0, 0, mdhdPayload)

	hdlrPayload := append(make([]byte, 4), "vide"...)
	hdlrPayload = append(hdlrPayload, make([]byte, 12)...)
	hdlrPayload = append(hdlrPayload, "VideoHandler\x00"...)
	hdlr := bmff.BuildFullBox("hdlr", 0, 0, hdlrPayload)

	vmhd := bmff.BuildFullBox("vmhd", 1, 1, make([]byte, 8))
	url := bmff.BuildFullBox("url ", 0, 1, nil)
	drefPayload := make([]byte, 4)
	bmff.PutU32(drefPayload, 1)
	drefPayload = append(drefPayload, url...)
	dinf := bmff.BuildBox("dinf", bmff.BuildFullBox("dref", 0, 0, drefPayload))

	sttsPayload := make([]byte, 12)
	bmff.PutU32(sttsPayload[0:4], 1)
	bmff.PutU32(sttsPayload[4:8], uint32(cfg.Frames))
	bmff.PutU32(sttsPayload[8:12], cfg.SampleDuration)
	stts := bmff.BuildFullBox("stts", 0, 0, sttsPayload)

	stscPayload := make([]byte, 16)
	bmff.PutU32(stscPayload[0:4], 1)
	bmff.PutU32(stscPayload[4:8], 1)
	bmff.PutU32(stscPayload[8:12], 1)
	bmff.PutU32(stscPayload[12:16], 1)
	stsc := bmff.BuildFullBox("stsc", 0, 0, stscPayload)

	stszPayload := make([]byte, 8+4*n)
	bmff.PutU32(stszPayload[4:8], uint32(n))
	for i, r := range recs {
		bmff.PutU32(stszPayload[8+4*i:12+4*i], uint32(r.size))
	}
	stsz := bmff.BuildFullBox("stsz", 0, 0, stszPayload)

	stcoPayload := make([]byte, 4+4*n)
	bmff.PutU32(stcoPayload[0:4], uint32(n))
	for i, r := range recs {
		bmff.PutU32(stcoPayload[4+4*i:8+4*i], uint32(payloadStart+int64(r.rel)))
	}
	stco := bmff.BuildFullBox("stco", 0, 0, stcoPayload)

	var keyNums []uint32
	for i, r := range recs {
		if r.key {
			keyNums = append(keyNums, uint32(i+1))
		}
	}
	stssPayload := make([]byte, 4+4*len(keyNums))
	bmff.PutU32(stssPayload[0:4], uint32(len(keyNums)))
	for i, k := range keyNums {
		bmff.PutU32(stssPayload[4+4*i:8+4*i], k)
	}
	stss := bmff.BuildFullBox("stss", 0, 0, stssPayload)

	stbl := bmff.BuildBox("stbl", concat(stsd, stts, stsc, stsz, stco, stss))
	minf := bmff.BuildBox("minf", concat(vmhd, dinf, stbl))
	mdia := bmff.BuildBox("mdia", concat(mdhd, hdlr, minf))
	trak := bmff.BuildBox("trak", concat(tkhd, mdia))
	return bmff.BuildBox("moov", concat(mvhd, trak))
}

func buildSTSD(w, h uint16) []byte {
	// Minimal avcC with fake parameter sets; scanner does not decode them.
	sps := []byte{0x67, 0x42, 0x00, 0x1e, 0xab}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	avccPayload := []byte{
		1, sps[1], sps[2], sps[3],
		0xff, // lengthSizeMinusOne = 3 (4 byte lengths)
		0xe1, // one SPS
	}
	l := make([]byte, 2)
	binary.BigEndian.PutUint16(l, uint16(len(sps)))
	avccPayload = append(avccPayload, l...)
	avccPayload = append(avccPayload, sps...)
	avccPayload = append(avccPayload, 1) // one PPS
	binary.BigEndian.PutUint16(l, uint16(len(pps)))
	avccPayload = append(avccPayload, l...)
	avccPayload = append(avccPayload, pps...)
	avcC := bmff.BuildBox("avcC", avccPayload)

	entry := make([]byte, 78)
	binary.BigEndian.PutUint16(entry[24:26], w)
	binary.BigEndian.PutUint16(entry[26:28], h)
	bmff.PutU32(entry[28:32], 0x00480000)
	bmff.PutU32(entry[32:36], 0x00480000)
	binary.BigEndian.PutUint16(entry[40:42], 1)
	entry = append(entry, avcC...)
	avc1 := bmff.BuildBox("avc1", entry)

	buf := make([]byte, 8)
	bmff.PutU32(buf[0:4], 1)
	buf = append(buf, avc1...)
	return bmff.BuildFullBox("stsd", 0, 0, buf)
}

func putIdentity(b []byte) {
	for i, v := range []uint32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000} {
		bmff.PutU32(b[4*i:4*i+4], v)
	}
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
