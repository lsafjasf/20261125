package fixture

func buildMDIA(cfg Config, chunkOffset uint32) []byte {
	return box("mdia", buildMDHD(cfg), buildHDLR(), buildMINF(cfg, chunkOffset))
}

func buildMDHD(cfg Config) []byte {
	p := make([]byte, 24)
	p[0] = 0
	putU32(p[12:16], cfg.MediaTimescale)
	putU32(p[16:20], cfg.MediaDuration)
	putU16(p[20:22], 0x55c4) // 'und' packed language
	return box("mdhd", p)
}

func buildHDLR() []byte {
	p := make([]byte, 0, 37)
	p = append(p, 0, 0, 0, 0)        // version + flags
	p = appendU32(p, 0)              // pre_defined
	p = append(p, "soun"...)         // handler_type
	p = append(p, zeros(12)...)      // reserved
	p = append(p, "SoundHandler"...) // name
	p = append(p, 0)
	return box("hdlr", p)
}

func buildMINF(cfg Config, chunkOffset uint32) []byte {
	smhd := box("smhd", make([]byte, 8))

	urlEntry := box("url ", fullBoxPayload(0, 1, nil))
	drefBody := appendU32(nil, 1)
	drefBody = append(drefBody, urlEntry...)
	dinf := box("dinf", box("dref", fullBoxPayload(0, 0, drefBody)))

	return box("minf", smhd, dinf, buildSTBL(cfg, chunkOffset))
}

func buildSTBL(cfg Config, chunkOffset uint32) []byte {
	entry := buildSOWTEntry(cfg.MediaTimescale)
	stsdBody := appendU32(nil, 1)
	stsdBody = append(stsdBody, entry...)
	stsd := box("stsd", fullBoxPayload(0, 0, stsdBody))

	sttsBody := appendU32(nil, 1)
	sttsBody = appendU32(sttsBody, uint32(cfg.PCMFrames))
	sttsBody = appendU32(sttsBody, 1)
	stts := box("stts", fullBoxPayload(0, 0, sttsBody))

	stscBody := appendU32(nil, 1)
	stscBody = appendU32(stscBody, 1)
	stscBody = appendU32(stscBody, uint32(cfg.PCMFrames))
	stscBody = appendU32(stscBody, 1)
	stsc := box("stsc", fullBoxPayload(0, 0, stscBody))

	stszBody := appendU32(nil, 2)
	stszBody = appendU32(stszBody, uint32(cfg.PCMFrames))
	stsz := box("stsz", fullBoxPayload(0, 0, stszBody))

	stcoBody := appendU32(nil, 1)
	stcoBody = appendU32(stcoBody, chunkOffset)
	stco := box("stco", fullBoxPayload(0, 0, stcoBody))

	return box("stbl", stsd, stts, stsc, stsz, stco)
}

func buildSOWTEntry(sampleRate uint32) []byte {
	p := make([]byte, 0, 28)
	p = append(p, zeros(6)...)
	p = appendU16(p, 1)
	p = appendU16(p, 0)
	p = appendU16(p, 0)
	p = appendU32(p, 0)
	p = appendU16(p, 1)
	p = appendU16(p, 16)
	p = appendU16(p, 0)
	p = appendU16(p, 0)
	p = appendU32(p, sampleRate<<16)
	return box("sowt", p)
}

func putU16(b []byte, v uint16) {
	b[0] = byte(v >> 8)
	b[1] = byte(v)
}

func putU32(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}
