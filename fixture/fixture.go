// Package fixture deterministically builds small MP4 files with known ground
// truth sample layouts and applies the three corruption families used to
// validate the repair tool: truncation, duplicate sample records and wrong
// chunk offsets.
package fixture

import (
	"encoding/binary"
	"os"

	"mp4index/bmff"
)

// GoldenSample is the known truth for one complete sample in a built file.
type GoldenSample struct {
	Offset int64
	Size   int
	Key    bool
}

// Golden describes a built fixture.
type Golden struct {
	Path             string
	FileSize         int64
	MdatPayloadStart int64
	Samples          []GoldenSample
	Timescale        uint32
	SampleDuration   uint32
}

// Config controls fixture generation.
type Config struct {
	Path           string
	Frames         int
	GOP            int  // keyframe every GOP frames; frame 0 is always key
	WithAUD        bool // prefix every sample with an AUD NAL
	MoovFirst      bool // faststart layout (moov before mdat)
	Width          uint16
	Height         uint16
	Timescale      uint32
	SampleDuration uint32
	// DupAfter lists 0 based sample indices after which an identical
	// stsz/stco record is duplicated in the old index.
	DupAfter []int
	// RepeatFrames lists frame indices whose payload bytes are an exact
	// repetition of the previous frame (payload level duplicate sample).
	RepeatFrames []int
}

func (c *Config) withDefaults() {
	if c.Frames <= 0 {
		c.Frames = 24
	}
	if c.GOP <= 0 {
		c.GOP = 8
	}
	if c.Width == 0 {
		c.Width, c.Height = 320, 240
	}
	if c.Timescale == 0 {
		c.Timescale = 15000
	}
	if c.SampleDuration == 0 {
		c.SampleDuration = 1000 // 15 fps
	}
}

// sampleBytes builds one deterministic AVCC sample. The VCL slice header
// encodes first_mb_in_slice == 0 so the scanner can find the boundary.
func sampleBytes(index int, key, withAUD bool, totalSize int) []byte {
	var out []byte
	if withAUD {
		aud := []byte{0x09, 0xF0}
		out = appendLengthNAL(out, aud)
	}

	// NAL header + one Exp-Golomb ue(0) bit (0x80) + filler.
	nalLen := totalSize - len(out) - 4
	if nalLen < 4 {
		nalLen = 4
	}
	nal := make([]byte, nalLen)
	if key {
		nal[0] = 0x65 // IDR slice
	} else {
		nal[0] = 0x41 // non-IDR reference slice
	}
	nal[1] = 0x80 // first_mb_in_slice = ue(0) -> leading 1 bit
	for i := 2; i < nalLen; i++ {
		nal[i] = byte(index*31 + i*7)
	}
	out = appendLengthNAL(out, nal)
	return out
}

func appendLengthNAL(out, nal []byte) []byte {
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(nal)))
	out = append(out, l[:]...)
	return append(out, nal...)
}

// Build creates the fixture file and returns its ground truth.
func Build(cfg Config) (*Golden, error) {
	cfg.withDefaults()

	// ---- build mdat payload + truth offsets relative to payload start ----
	payload := []byte{}
	type relSample struct {
		rel  int
		size int
		key  bool
	}
	rels := make([]relSample, 0, cfg.Frames)
	repeatSet := map[int]bool{}
	for _, f := range cfg.RepeatFrames {
		repeatSet[f] = true
	}
	var prevSample []byte
	for i := 0; i < cfg.Frames; i++ {
		key := i%cfg.GOP == 0
		size := 40 + (i%5)*37 // deterministic varied sizes
		before := len(payload)
		s := sampleBytes(i, key, cfg.WithAUD, size)
		if repeatSet[i] && prevSample != nil {
			s = append([]byte(nil), prevSample...)
		}
		payload = append(payload, s...)
		rels = append(rels, relSample{before, len(s), key})
		prevSample = s
	}

	ftyp := buildFtyp()
	stsd := buildSTSD(cfg.Width, cfg.Height)

	// Index record order with optional duplicates.
	recs := make([]indexRec, 0, cfg.Frames+len(cfg.DupAfter))
	dupSet := map[int]bool{}
	for _, d := range cfg.DupAfter {
		dupSet[d] = true
	}
	for i, r := range rels {
		recs = append(recs, indexRec{r.rel, r.size, r.key})
		if dupSet[i] {
			recs = append(recs, indexRec{r.rel, r.size, r.key})
		}
	}

	var moov []byte
	var mdatStart int64
	if cfg.MoovFirst {
		// Two passes: moov length is offset-value independent.
		dummy := buildMoov(cfg, stsd, recs, 0)
		mdatStart = int64(len(ftyp)) + int64(len(dummy)) + 8
		moov = buildMoov(cfg, stsd, recs, mdatStart)
	} else {
		mdatStart = int64(len(ftyp)) + 8
		moov = buildMoov(cfg, stsd, recs, mdatStart)
	}

	mdat := bmff.BuildBox("mdat", payload)

	var file []byte
	if cfg.MoovFirst {
		file = append(file, ftyp...)
		file = append(file, moov...)
		file = append(file, mdat...)
	} else {
		file = append(file, ftyp...)
		file = append(file, mdat...)
		file = append(file, moov...)
	}

	if err := os.WriteFile(cfg.Path, file, 0o644); err != nil {
		return nil, err
	}

	g := &Golden{
		Path:             cfg.Path,
		FileSize:         int64(len(file)),
		Timescale:        cfg.Timescale,
		SampleDuration:   cfg.SampleDuration,
		MdatPayloadStart: mdatStart,
	}
	for _, r := range rels {
		g.Samples = append(g.Samples, GoldenSample{
			Offset: mdatStart + int64(r.rel),
			Size:   r.size,
			Key:    r.key,
		})
	}
	return g, nil
}

func buildFtyp() []byte {
	p := make([]byte, 16)
	copy(p[0:4], "isom")
	p[6] = 2
	copy(p[8:12], "isom")
	copy(p[12:16], "mp41")
	return bmff.BuildBox("ftyp", p)
}
