package fixture

import (
	"encoding/binary"
	"math"
)

type MKVSampleSpec struct {
	TimeMS           int16
	DurationMS       uint64
	DiscardPaddingNS int64
	Key              bool
	Data             []byte
}

type MKVTrackSpec struct {
	Number            uint64
	Type              uint64
	CodecID           string
	DefaultDurationNS uint64
	CodecDelayNS      uint64
	Width             uint64
	Height            uint64
	SampleRate        float64
	Channels          uint64
	Samples           []MKVSampleSpec
}

type MKVSpec struct {
	DurationMS     float64
	TimestampScale uint64
	Tracks         []MKVTrackSpec
}

func BuildMKV(spec MKVSpec) []byte {
	if spec.TimestampScale == 0 {
		spec.TimestampScale = 1_000_000
	}
	header := element(0x1A45DFA3,
		append(uintElement(0x4286, 1), uintElement(0x42F7, 1)...))
	var info []byte
	info = append(info, uintElement(0x2AD7B1, spec.TimestampScale)...)
	info = append(info, floatElement(0x4489, spec.DurationMS)...)
	var tracks []byte
	for _, tr := range spec.Tracks {
		tracks = append(tracks, makeMKVTrack(tr)...)
	}
	var cluster []byte
	cluster = append(cluster, uintElement(0xE7, 0)...)
	for _, tr := range spec.Tracks {
		for _, smp := range tr.Samples {
			cluster = append(cluster, makeMKVBlock(tr, smp)...)
		}
	}
	segment := []byte{}
	segment = append(segment, element(0x1549A966, info)...)
	segment = append(segment, element(0x1654AE6B, tracks)...)
	segment = append(segment, element(0x1F43B675, cluster)...)
	out := append(header, element(0x18538067, segment)...)
	return out
}

func makeMKVTrack(tr MKVTrackSpec) []byte {
	var payload []byte
	payload = append(payload, uintElement(0xD7, tr.Number)...)
	payload = append(payload, uintElement(0x73C5, tr.Number+100)...)
	payload = append(payload, uintElement(0x83, tr.Type)...)
	payload = append(payload, stringElement(0x86, tr.CodecID)...)
	if tr.DefaultDurationNS > 0 {
		payload = append(payload, uintElement(0x23E383, tr.DefaultDurationNS)...)
	}
	if tr.CodecDelayNS > 0 {
		payload = append(payload, uintElement(0x56AA, tr.CodecDelayNS)...)
	}
	switch tr.Type {
	case 1:
		var video []byte
		video = append(video, uintElement(0xB0, tr.Width)...)
		video = append(video, uintElement(0xBA, tr.Height)...)
		payload = append(payload, element(0xE0, video)...)
	case 2:
		var audio []byte
		audio = append(audio, floatElement(0xB5, tr.SampleRate)...)
		audio = append(audio, uintElement(0x9F, tr.Channels)...)
		payload = append(payload, element(0xE1, audio)...)
	}
	return element(0xAE, payload)
}

func makeMKVBlock(tr MKVTrackSpec, smp MKVSampleSpec) []byte {
	flags := byte(0)
	if smp.Key {
		flags |= 0x80
	}
	block := append(vintNumber(tr.Number), byte(byte(smp.TimeMS>>8)), byte(smp.TimeMS), flags)
	block = append(block, smp.Data...)
	if smp.DiscardPaddingNS == 0 && smp.DurationMS == 0 {
		return element(0xA3, block)
	}
	var group []byte
	group = append(group, element(0xA1, block)...)
	if smp.DurationMS > 0 {
		group = append(group, uintElement(0x9B, smp.DurationMS)...)
	}
	if smp.DiscardPaddingNS != 0 {
		group = append(group, signedElement(0x75A2, smp.DiscardPaddingNS)...)
	}
	return element(0xA0, group)
}

func element(id uint32, payload []byte) []byte {
	idb := idBytes(id)
	out := append(idb, encodeSize(uint64(len(payload)))...)
	out = append(out, payload...)
	return out
}

func idBytes(id uint32) []byte {
	switch {
	case id <= 0xff:
		return []byte{byte(id)}
	case id <= 0xffff:
		return []byte{byte(id >> 8), byte(id)}
	case id <= 0xffffff:
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	default:
		return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
	}
}

func encodeSize(v uint64) []byte {
	for n := 1; n <= 8; n++ {
		max := uint64(1)<<(7*n) - 1
		if v < max || n == 8 {
			out := make([]byte, n)
			for i := n - 1; i >= 0; i-- {
				out[i] = byte(v)
				v >>= 8
			}
			out[0] |= 1 << (8 - n)
			return out
		}
	}
	return nil
}

func uintElement(id uint32, v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	i := 0
	for i < 7 && b[i] == 0 {
		i++
	}
	return element(id, b[i:])
}

func signedElement(id uint32, v int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	i := 0
	negative := v < 0
	for i < 7 {
		nextOutside := b[i+1] >= 0x80
		if b[i] != 0 && b[i] != 0xff {
			break
		}
		if !negative && b[i] == 0 && !nextOutside {
			i++
			continue
		}
		if negative && b[i] == 0xff && nextOutside {
			i++
			continue
		}
		break
	}
	return element(id, b[i:])
}

func floatElement(id uint32, v float64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], math.Float64bits(v))
	return element(id, b[:])
}

func stringElement(id uint32, v string) []byte {
	return element(id, []byte(v))
}

func vintNumber(v uint64) []byte {
	for n := 1; n <= 8; n++ {
		max := uint64(1)<<(7*n) - 1
		if v < max {
			out := make([]byte, n)
			for i := n - 1; i >= 0; i-- {
				out[i] = byte(v)
				v >>= 8
			}
			out[0] |= 1 << (8 - n)
			return out
		}
	}
	return nil
}
