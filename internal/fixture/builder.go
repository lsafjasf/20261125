package fixture

import (
	"encoding/binary"
	"math"
)

// Edit is one version-0 edit list entry.
type Edit struct {
	Duration  uint32
	MediaTime int32
	Rate      uint32
}

// Config describes a tiny mono PCM QuickTime fixture.
type Config struct {
	FileName       string
	MovieTimescale uint32
	MovieDuration  uint32
	TrackID        uint32
	TrackDuration  uint32
	MediaTimescale uint32
	MediaDuration  uint32
	PCMFrames      int
	ToneHz         float64
	Edits          []Edit
}

// BuildMOV builds a small mono sowt/PCM MOV with one trak.
func BuildMOV(cfg Config) []byte {
	ftyp := buildFileTypeBox()

	pcm := make([]byte, cfg.PCMFrames*2)
	for i := 0; i < cfg.PCMFrames; i++ {
		var sample int16
		if cfg.ToneHz != 0 && cfg.MediaTimescale != 0 {
			value := math.Sin(2 * math.Pi * cfg.ToneHz * float64(i) / float64(cfg.MediaTimescale))
			sample = int16(value * 12000)
		}
		// sowt is little-endian signed 16-bit PCM in QuickTime.
		binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(sample))
	}

	moovPlaceholder := buildMoov(cfg, 0)
	chunkOffset := uint32(len(ftyp) + len(moovPlaceholder) + 8)
	moov := buildMoov(cfg, chunkOffset)

	out := make([]byte, 0, len(ftyp)+len(moov)+8+len(pcm))
	out = append(out, ftyp...)
	out = append(out, moov...)
	out = append(out, boxHeader("mdat", uint32(8+len(pcm)))...)
	out = append(out, pcm...)
	return out
}

func buildFileTypeBox() []byte {
	payload := make([]byte, 12)
	copy(payload[0:4], "qt  ")
	binary.BigEndian.PutUint32(payload[4:8], 0x00000200)
	copy(payload[8:12], "qt  ")
	return box("ftyp", payload)
}

func box(typ string, payload ...[]byte) []byte {
	total := 8
	for _, p := range payload {
		total += len(p)
	}
	out := make([]byte, 8, total)
	binary.BigEndian.PutUint32(out[0:4], uint32(total))
	copy(out[4:8], typ)
	for _, p := range payload {
		out = append(out, p...)
	}
	return out
}

func boxHeader(typ string, size uint32) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], size)
	copy(out[4:8], typ)
	return out
}

func fullBoxPayload(version byte, flags uint32, body []byte) []byte {
	head := []byte{version, byte(flags >> 16), byte(flags >> 8), byte(flags)}
	return append(head, body...)
}

func appendU16(b []byte, v uint16) []byte {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], v)
	return append(b, buf[:]...)
}

func appendU32(b []byte, v uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	return append(b, buf[:]...)
}

func zeros(n int) []byte { return make([]byte, n) }
