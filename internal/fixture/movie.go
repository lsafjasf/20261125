package fixture

import "encoding/binary"

func buildMoov(cfg Config, chunkOffset uint32) []byte {
	trak := buildTrak(cfg, chunkOffset)
	return box("moov", buildMVHD(cfg), trak)
}

func buildMVHD(cfg Config) []byte {
	p := make([]byte, 100)
	p[0] = 0
	binary.BigEndian.PutUint32(p[12:16], cfg.MovieTimescale)
	binary.BigEndian.PutUint32(p[16:20], cfg.MovieDuration)
	binary.BigEndian.PutUint32(p[20:24], 0x00010000)
	binary.BigEndian.PutUint16(p[24:26], 0x0100)
	putIdentityMatrix(p[36:72])
	binary.BigEndian.PutUint32(p[96:100], cfg.TrackID+1)
	return box("mvhd", p)
}

func buildTrak(cfg Config, chunkOffset uint32) []byte {
	return box("trak", buildTKHD(cfg), buildEDTS(cfg), buildMDIA(cfg, chunkOffset))
}

func buildTKHD(cfg Config) []byte {
	p := make([]byte, 84)
	p[3] = 0x07 // enabled, in movie, in preview
	binary.BigEndian.PutUint32(p[12:16], cfg.TrackID)
	binary.BigEndian.PutUint32(p[20:24], cfg.TrackDuration)
	binary.BigEndian.PutUint16(p[36:38], 0x0100)
	putIdentityMatrix(p[40:76])
	return box("tkhd", p)
}

func buildEDTS(cfg Config) []byte {
	if len(cfg.Edits) == 0 {
		return nil
	}
	p := make([]byte, 8+12*len(cfg.Edits))
	binary.BigEndian.PutUint32(p[4:8], uint32(len(cfg.Edits)))
	pos := 8
	for _, edit := range cfg.Edits {
		binary.BigEndian.PutUint32(p[pos:pos+4], edit.Duration)
		binary.BigEndian.PutUint32(p[pos+4:pos+8], uint32(edit.MediaTime))
		binary.BigEndian.PutUint32(p[pos+8:pos+12], edit.Rate)
		pos += 12
	}
	return box("edts", box("elst", p))
}

func putIdentityMatrix(m []byte) {
	binary.BigEndian.PutUint32(m[0:4], 0x00010000)
	binary.BigEndian.PutUint32(m[16:20], 0x00010000)
	binary.BigEndian.PutUint32(m[32:36], 0x40000000)
}
