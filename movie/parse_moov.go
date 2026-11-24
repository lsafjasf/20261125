package movie

import (
	"io"

	"mp4index/bmff"
)

func parseMoov(ra io.ReaderAt, moov bmff.Box, info *Info) error {
	if mvhd, ok, err := bmff.FindChild(ra, moov, "mvhd"); err == nil && ok {
		if payload, err := bmff.ReadPayload(ra, mvhd); err == nil {
			info.MovieTimeScale = parseMVHDTimescale(payload)
		}
	}

	return bmff.IterateChildren(ra, moov, func(child bmff.Box) error {
		if child.Type != "trak" {
			return nil
		}
		t, err := parseTrak(ra, child)
		if err != nil {
			return err
		}
		if t != nil {
			if raw, rerr := bmff.ReadBoxBytes(ra, child); rerr == nil {
				t.RawTrak = raw
			}
			info.Tracks = append(info.Tracks, t)
			if t.Handler == "vide" && info.Video == nil {
				info.Video = t
			}
		}
		return nil
	})
}

func parseMVHDTimescale(p []byte) uint32 {
	if len(p) < 4 {
		return 0
	}
	version := p[0]
	if version == 1 {
		// version(1)+flags(3), creation(8)+modification(8)
		if len(p) >= 4+20+4 {
			return bmff.U32(p, 4+16)
		}
		return 0
	}
	// creation(4)+modification(4)
	if len(p) >= 4+8+4 {
		return bmff.U32(p, 4+8)
	}
	return 0
}

func parseTrak(ra io.ReaderAt, trak bmff.Box) (*Track, error) {
	t := &Track{LengthSize: 4}

	if tkhd, ok, err := bmff.FindChild(ra, trak, "tkhd"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, tkhd); err == nil {
			parseTKHD(p, t)
		}
	}
	mdia, ok, err := bmff.FindChild(ra, trak, "mdia")
	if err != nil || !ok {
		return nil, nil
	}
	if mdhd, ok, err := bmff.FindChild(ra, mdia, "mdhd"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, mdhd); err == nil {
			t.TimeScale, t.Duration = parseMDHD(p)
		}
	}
	if hdlr, ok, err := bmff.FindChild(ra, mdia, "hdlr"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, hdlr); err == nil && len(p) >= 12 {
			t.Handler = string(p[8:12])
		}
	}

	minf, ok, err := bmff.FindChild(ra, mdia, "minf")
	if err != nil || !ok {
		return t, nil
	}
	stbl, ok, err := bmff.FindChild(ra, minf, "stbl")
	if err != nil || !ok {
		return t, nil
	}

	if stsd, ok, err := bmff.FindChild(ra, stbl, "stsd"); err == nil && ok {
		if raw, err := bmff.ReadBoxBytes(ra, stsd); err == nil {
			t.StsdBox = raw
			t.Codec, t.Width, t.Height = parseSTSD(raw)
			if ls := parseAvcLengthSize(raw); ls > 0 {
				t.LengthSize = ls
			}
		}
	}

	t.Index = &OldIndex{}
	if stts, ok, err := bmff.FindChild(ra, stbl, "stts"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, stts); err == nil {
			t.Index.Stts = parseStts(p)
		}
	}
	if stsc, ok, err := bmff.FindChild(ra, stbl, "stsc"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, stsc); err == nil {
			t.Index.Stsc = parseStsc(p)
		}
	}
	if stsz, ok, err := bmff.FindChild(ra, stbl, "stsz"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, stsz); err == nil {
			parseStsz(p, t.Index)
		}
	}
	if stco, ok, err := bmff.FindChild(ra, stbl, "stco"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, stco); err == nil {
			t.Index.Offsets = parseStco(p, false)
		}
	} else if co64, ok, err := bmff.FindChild(ra, stbl, "co64"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, co64); err == nil {
			t.Index.Offsets = parseStco(p, true)
			t.Index.Offsets64 = true
		}
	}
	if stss, ok, err := bmff.FindChild(ra, stbl, "stss"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, stss); err == nil {
			t.Index.SyncSamples = parseStss(p)
		}
	}
	if ctts, ok, err := bmff.FindChild(ra, stbl, "ctts"); err == nil && ok {
		if p, err := bmff.ReadPayload(ra, ctts); err == nil {
			t.Ctts = parseCtts(p)
		}
	}
	return t, nil
}

// parseCtts expands a composition-offset table into per-sample signed offsets.
func parseCtts(p []byte) []int32 {
	if len(p) < 8 {
		return nil
	}
	version := p[0]
	count := bmff.U32(p, 4)
	avail := (len(p) - 8) / 8
	n := min32(count, uint32(avail))
	const maxExpand = 5_000_000
	out := make([]int32, 0, n)
	for i := uint32(0); i < n; i++ {
		off := 8 + int(i)*8
		runCount := bmff.U32(p, off)
		raw := bmff.U32(p, off+4)
		delta := int32(raw)
		if version == 0 {
			delta = int32(raw) // non-negative in v0; same bit pattern
		}
		for j := uint32(0); j < runCount && len(out) < maxExpand; j++ {
			out = append(out, delta)
		}
	}
	return out
}

func parseTKHD(p []byte, t *Track) {
	if len(p) < 4 {
		return
	}
	version := p[0]
	if version == 1 {
		// flags3 + creation8 + mod8 + trackID4 ...
		if len(p) >= 4+16+4+4+4+8 {
			t.TrackID = bmff.U32(p, 4+16)
		}
	} else if len(p) >= 4+8+4 {
		t.TrackID = bmff.U32(p, 4+8)
	}
	// width/height are the last 8 bytes: 16.16 fixed point.
	if len(p) >= 8 {
		t.Width = uint32(bmff.U32(p, len(p)-8) >> 16)
		t.Height = uint32(bmff.U32(p, len(p)-4) >> 16)
	}
}

func parseMDHD(p []byte) (uint32, uint64) {
	if len(p) < 4 {
		return 0, 0
	}
	if p[0] == 1 {
		// flags3, creation8, mod8, timescale4, duration8
		if len(p) >= 4+16+4+8 {
			ts := bmff.U32(p, 4+16)
			dur := uint64(bmff.U32(p, 4+20))<<32 | uint64(bmff.U32(p, 4+24))
			return ts, dur
		}
		return 0, 0
	}
	// flags3, creation4, mod4, timescale4, duration4
	if len(p) >= 4+4+4+4+4 {
		return bmff.U32(p, 12), uint64(bmff.U32(p, 16))
	}
	return 0, 0
}

func parseStts(p []byte) []SttsEntry {
	if len(p) < 8 {
		return nil
	}
	count := bmff.U32(p, 4)
	out := make([]SttsEntry, 0, min32(count, uint32((len(p)-8)/8)))
	for i := uint32(0); i < count && 8+int(i)*8+8 <= len(p); i++ {
		off := 8 + int(i)*8
		out = append(out, SttsEntry{Count: bmff.U32(p, off), Duration: bmff.U32(p, off+4)})
	}
	return out
}

func parseStsc(p []byte) []StscEntry {
	if len(p) < 8 {
		return nil
	}
	count := bmff.U32(p, 4)
	out := make([]StscEntry, 0, min32(count, uint32((len(p)-8)/12)))
	for i := uint32(0); i < count && 8+int(i)*12+12 <= len(p); i++ {
		off := 8 + int(i)*12
		out = append(out, StscEntry{
			FirstChunk:      bmff.U32(p, off),
			SamplesPerChunk: bmff.U32(p, off+4),
			DescIndex:       bmff.U32(p, off+8),
		})
	}
	return out
}

func parseStsz(p []byte, idx *OldIndex) {
	if len(p) < 12 {
		return
	}
	idx.UniformSize = bmff.U32(p, 4)
	idx.SampleCount = bmff.U32(p, 8)
	if idx.UniformSize != 0 {
		return
	}
	n := min32(idx.SampleCount, uint32((len(p)-12)/4))
	idx.Sizes = make([]uint32, n)
	for i := uint32(0); i < n; i++ {
		idx.Sizes[i] = bmff.U32(p, 12+int(i)*4)
	}
}

func parseStco(p []byte, is64 bool) []uint64 {
	if len(p) < 8 {
		return nil
	}
	count := bmff.U32(p, 4)
	if is64 {
		n := min32(count, uint32((len(p)-8)/8))
		out := make([]uint64, n)
		for i := uint32(0); i < n; i++ {
			off := 8 + int(i)*8
			out[i] = uint64(bmff.U32(p, off))<<32 | uint64(bmff.U32(p, off+4))
		}
		return out
	}
	n := min32(count, uint32((len(p)-8)/4))
	out := make([]uint64, n)
	for i := uint32(0); i < n; i++ {
		out[i] = uint64(bmff.U32(p, 8+int(i)*4))
	}
	return out
}

func parseStss(p []byte) []uint32 {
	if len(p) < 8 {
		return nil
	}
	count := bmff.U32(p, 4)
	n := min32(count, uint32((len(p)-8)/4))
	out := make([]uint32, n)
	for i := uint32(0); i < n; i++ {
		out[i] = bmff.U32(p, 8+int(i)*4)
	}
	return out
}

func min32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
