package fixture

import (
	"encoding/binary"
	"fmt"
)

type SampleSpec struct {
	Data     []byte
	Duration uint32
	Key      bool
}

type EditSpec struct {
	SegmentDuration uint32
	MediaTime       int32
}

type MP4Spec struct {
	MovieTimescale    uint32
	DurationMS        uint32
	Width             uint16
	Height            uint16
	Video             []SampleSpec
	VideoTimescale    uint32
	VideoEdits        []EditSpec
	Audio             []SampleSpec
	AudioTimescale    uint32
	AudioRate         uint32
	AudioChannels     uint16
	AudioEdits        []EditSpec
	Subtitle          []SampleSpec
	SubtitleTimescale uint32
	SubtitleEdits     []EditSpec
}

func BuildMP4(spec MP4Spec) []byte {
	if spec.MovieTimescale == 0 {
		spec.MovieTimescale = 1000
	}
	if spec.VideoTimescale == 0 {
		spec.VideoTimescale = 12800
	}
	if spec.AudioTimescale == 0 {
		spec.AudioTimescale = 48000
	}
	if spec.SubtitleTimescale == 0 {
		spec.SubtitleTimescale = 1000
	}
	if spec.Width == 0 {
		spec.Width, spec.Height = 320, 180
	}
	if spec.AudioRate == 0 {
		spec.AudioRate, spec.AudioChannels = 48000, 2
	}
	type trackData struct {
		samples []SampleSpec
		offset  uint32
	}
	tracks := []*trackData{{samples: spec.Video}, {samples: spec.Audio}, {samples: spec.Subtitle}}
	var mdat []byte
	for _, tr := range tracks {
		tr.offset = uint32(len(mdat))
		for _, s := range tr.samples {
			mdat = append(mdat, s.Data...)
		}
	}
	ftyp := box("ftyp", []byte("isom\x00\x00\x02\x00isommp41"))
	mdatBox := box("mdat", mdat)
	mdatPayload := uint32(len(ftyp) + 8)

	var moovChildren []byte
	moovChildren = append(moovChildren, makeMVHD(spec.MovieTimescale, spec.DurationMS)...)
	if len(spec.Video) > 0 {
		moovChildren = append(moovChildren, makeTRAK(
			1, "vide", spec.VideoTimescale, spec.MovieTimescale, uint32(spec.Width), uint32(spec.Height),
			spec.Video, spec.VideoEdits, mdatPayload+tracks[0].offset, true)...)
	}
	if len(spec.Audio) > 0 {
		moovChildren = append(moovChildren, makeTRAK(
			2, "soun", spec.AudioTimescale, spec.MovieTimescale, spec.AudioRate,
			uint32(spec.AudioChannels), spec.Audio, spec.AudioEdits, mdatPayload+tracks[1].offset, false)...)
	}
	if len(spec.Subtitle) > 0 {
		moovChildren = append(moovChildren, makeTRAK(
			3, "sbtl", spec.SubtitleTimescale, spec.MovieTimescale, 0, 0,
			spec.Subtitle, spec.SubtitleEdits, mdatPayload+tracks[2].offset, false)...)
	}
	moov := box("moov", moovChildren)
	out := append(ftyp, mdatBox...)
	out = append(out, moov...)
	return out
}

func makeMVHD(timescale, duration uint32) []byte {
	b := make([]byte, 100)
	binary.BigEndian.PutUint32(b[12:16], timescale)
	binary.BigEndian.PutUint32(b[16:20], duration)
	return box("mvhd", b)
}

func makeTRAK(id uint32, handler string, timescale, movieTimescale uint32, w, h uint32, samples []SampleSpec, edits []EditSpec, offset uint32, video bool) []byte {
	var children []byte
	children = append(children, makeTKHD(id, w, h)...)
	if len(edits) > 0 {
		children = append(children, box("edts", box("elst", makeELST(edits)))...)
	}
	children = append(children, box("mdia", makeMDIA(handler, timescale, samples, offset, video, uint16(w), uint16(h)))...)
	return box("trak", children)
}

func makeTKHD(id uint32, w, h uint32) []byte {
	b := make([]byte, 84)
	binary.BigEndian.PutUint32(b[12:16], id)
	binary.BigEndian.PutUint32(b[76:80], uint32(w)<<16)
	binary.BigEndian.PutUint32(b[80:84], uint32(h)<<16)
	return box("tkhd", b)
}

func makeELST(edits []EditSpec) []byte {
	b := make([]byte, 8+12*len(edits))
	binary.BigEndian.PutUint32(b[4:8], uint32(len(edits)))
	for i, e := range edits {
		pos := 8 + i*12
		binary.BigEndian.PutUint32(b[pos:pos+4], e.SegmentDuration)
		binary.BigEndian.PutUint32(b[pos+4:pos+8], uint32(e.MediaTime))
		binary.BigEndian.PutUint32(b[pos+8:pos+12], 0x00010000)
	}
	return b
}

func makeMDIA(handler string, timescale uint32, samples []SampleSpec, offset uint32, video bool, w, h uint16) []byte {
	mdhd := make([]byte, 24)
	var duration uint64
	for _, s := range samples {
		duration += uint64(s.Duration)
	}
	binary.BigEndian.PutUint32(mdhd[12:16], timescale)
	binary.BigEndian.PutUint32(mdhd[16:20], uint32(duration))
	hdlr := make([]byte, 20)
	copy(hdlr[8:12], handler)
	var stsd []byte
	if video {
		stsd = makeSTSD(makeVideoSampleEntry(w, h))
	} else if handler == "soun" {
		stsd = makeSTSD(makeAudioSampleEntry())
	} else {
		stsd = makeSTSD(makeSubtitleSampleEntry())
	}
	stts := make([]byte, 16)
	binary.BigEndian.PutUint32(stts[4:8], 1)
	binary.BigEndian.PutUint32(stts[8:12], uint32(len(samples)))
	binary.BigEndian.PutUint32(stts[12:16], samples[0].Duration)
	stsc := make([]byte, 20)
	binary.BigEndian.PutUint32(stsc[4:8], 1)
	binary.BigEndian.PutUint32(stsc[8:12], 1)
	binary.BigEndian.PutUint32(stsc[12:16], uint32(len(samples)))
	binary.BigEndian.PutUint32(stsc[16:20], 1)
	stsz := make([]byte, 12+4*len(samples))
	binary.BigEndian.PutUint32(stsz[8:12], uint32(len(samples)))
	for i, s := range samples {
		binary.BigEndian.PutUint32(stsz[12+i*4:16+i*4], uint32(len(s.Data)))
	}
	stco := make([]byte, 12)
	binary.BigEndian.PutUint32(stco[4:8], 1)
	binary.BigEndian.PutUint32(stco[8:12], offset)
	stbl := []byte{}
	stbl = append(stbl, stsd...)
	stbl = append(stbl, box("stts", stts)...)
	stbl = append(stbl, box("stsc", stsc)...)
	stbl = append(stbl, box("stsz", stsz)...)
	stbl = append(stbl, box("stco", stco)...)
	if video {
		var sync []byte
		count := 0
		for _, s := range samples {
			if s.Key {
				count++
			}
		}
		sync = make([]byte, 8+4*count)
		binary.BigEndian.PutUint32(sync[4:8], uint32(count))
		pos := 8
		for i, s := range samples {
			if s.Key {
				binary.BigEndian.PutUint32(sync[pos:pos+4], uint32(i+1))
				pos += 4
			}
		}
		stbl = append(stbl, box("stss", sync)...)
	}
	minf := box("minf", box("stbl", stbl))
	return append(append(box("mdhd", mdhd), box("hdlr", hdlr)...), minf...)
}

func makeSTSD(entry []byte) []byte {
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[4:8], 1)
	return box("stsd", append(header, entry...))
}

func makeVideoSampleEntry(w, h uint16) []byte {
	b := make([]byte, 28)
	binary.BigEndian.PutUint16(b[6:8], 1)
	binary.BigEndian.PutUint16(b[24:26], w)
	binary.BigEndian.PutUint16(b[26:28], h)
	return box("avc1", b)
}

func makeAudioSampleEntry() []byte {
	b := make([]byte, 28)
	binary.BigEndian.PutUint16(b[6:8], 1)
	binary.BigEndian.PutUint16(b[16:18], 2)
	binary.BigEndian.PutUint32(b[24:28], 48000<<16)
	return box("mp4a", b)
}

func makeSubtitleSampleEntry() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint16(b[6:8], 1)
	return box("tx3g", b)
}

func box(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[:4], uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

func TextSample(i int) []byte {
	text := []byte(fmt.Sprintf("V%03d", i))
	data := make([]byte, 2+len(text))
	binary.BigEndian.PutUint16(data[:2], uint16(len(text)))
	copy(data[2:], text)
	return data
}
