package h264scan

import (
	"io"
	"sort"
)

const (
	avcAUD  = 9
	hevcAUD = 35
)

// groupSamples turns the flat NAL list into access units.
func groupSamples(nals []nalInfo, gaps []Gap, cleanEnd int64, codec Codec) []Sample {
	if len(nals) == 0 {
		return nil
	}

	startSet := map[int]bool{0: true}
	pictureStarts := 0

	for i, n := range nals {
		if codec == CodecHEVC {
			if isVCLHEVC(n.typ) && n.firstOK && n.firstMB == 1 {
				startSet[preludeStart(nals, i, codec)] = true
				pictureStarts++
			}
			continue
		}
		if !isVCLAVC(n.typ) {
			continue
		}
		boundary := true
		if n.firstOK {
			boundary = n.firstMB == 0
		}
		if boundary {
			startSet[preludeStart(nals, i, codec)] = true
			pictureStarts++
		}
	}

	// Streams without parseable slice headers still get AUD segmentation.
	if pictureStarts == 0 {
		aud := byte(avcAUD)
		if codec == CodecHEVC {
			aud = hevcAUD
		}
		for i, n := range nals {
			if n.typ == aud {
				startSet[i] = true
			}
		}
	}

	startIdxs := make([]int, 0, len(startSet))
	for idx := range startSet {
		startIdxs = append(startIdxs, idx)
	}
	sort.Ints(startIdxs)

	out := make([]Sample, 0, len(startIdxs))
	for k, si := range startIdxs {
		nextIdx := len(nals)
		if k+1 < len(startIdxs) {
			nextIdx = startIdxs[k+1]
		}
		startOff := nals[si].prefixOff
		endOff := cleanEnd
		if nextIdx < len(nals) {
			endOff = nals[nextIdx].prefixOff
		}

		// A resync gap inside the nominal span cuts the sample short.
		for _, g := range gaps {
			if g.Start > startOff && g.Start < endOff {
				endOff = g.Start
			}
		}

		key := false
		for j := si; j < nextIdx; j++ {
			if isIDR(codec, nals[j].typ) {
				key = true
				break
			}
		}
		out = append(out, Sample{
			Offset:   startOff,
			Size:     endOff - startOff,
			Keyframe: key,
			NALCount: nextIdx - si,
		})
	}
	return out
}

// preludeStart walks backwards over parameter/header NALs (SPS/PPS/SEI/...)
// that belong to the upcoming picture and returns the first NAL index of the
// access unit containing VCL NAL vi.
func preludeStart(nals []nalInfo, vi int, codec Codec) int {
	isVCL := isVCLAVC
	aud := byte(avcAUD)
	if codec == CodecHEVC {
		isVCL = isVCLHEVC
		aud = hevcAUD
	}
	c := vi
	for c > 0 {
		t := nals[c-1].typ
		if isVCL(t) || t == aud {
			break
		}
		c--
	}
	if c > 0 && nals[c-1].typ == aud {
		c--
	}
	return c
}

// resync scans forward byte by byte looking for a plausible length prefixed
// NAL chain. pos is the current reader offset relative to section start; it is
// advanced over consumed garbage bytes.
func resync(ra io.ReaderAt, br bytePeeker, pos *int64, start, end int64, opts Options) (int64, bool) {
	ls := opts.LengthSize
	need := ls + 1
	for start+*pos+int64(need) <= end {
		peek, err := br.Peek(need)
		if len(peek) < need {
			if err != nil {
				return 0, false
			}
			if _, e := br.ReadByte(); e != nil {
				return 0, false
			}
			*pos++
			continue
		}
		off := start + *pos
		if cheapCandidate(peek, off, end, opts) && candidateValid(ra, off, end, opts) {
			return off, true
		}
		b, err := br.ReadByte()
		_ = b
		if err != nil {
			return 0, false
		}
		*pos++
	}
	return 0, false
}

// cheapCandidate filters byte positions from buffered bytes before doing any
// random access reads for chain validation.
func cheapCandidate(peek []byte, off, end int64, opts Options) bool {
	ls := opts.LengthSize
	nalLen := decodeLength(peek[:ls])
	if nalLen < 1 || off+int64(ls)+nalLen > end {
		return false
	}
	h := peek[ls]
	if h&0x80 != 0 {
		return false
	}
	if opts.Codec == CodecHEVC {
		return (h>>1)&0x3f <= 40
	}
	t := h & 0x1f
	return t > 0 && t <= 23
}

type bytePeeker interface {
	ReadByte() (byte, error)
	Peek(n int) ([]byte, error)
}

// candidateValid checks that off looks like a length prefix followed by a sane
// NAL header and another valid NAL (or exact end of section).
func candidateValid(ra io.ReaderAt, off, end int64, opts Options) bool {
	ls := opts.LengthSize
	var hdr [8]byte
	if _, err := ra.ReadAt(hdr[:ls+1], off); err != nil {
		return false
	}
	nalLen := decodeLength(hdr[:ls])
	if nalLen < 1 || off+int64(ls)+nalLen > end {
		return false
	}
	h := hdr[ls]
	if h&0x80 != 0 { // forbidden_zero_bit
		return false
	}
	if opts.Codec == CodecHEVC {
		t := (h >> 1) & 0x3f
		if t > 40 {
			return false
		}
	} else {
		t := h & 0x1f
		if t == 0 || t > 23 {
			return false
		}
	}

	next := off + int64(ls) + nalLen
	if next == end {
		return true
	}
	if next+int64(ls)+1 > end {
		return false
	}
	var hdr2 [8]byte
	if _, err := ra.ReadAt(hdr2[:ls+1], next); err != nil {
		return false
	}
	nalLen2 := decodeLength(hdr2[:ls])
	if nalLen2 < 1 || next+int64(ls)+nalLen2 > end {
		return false
	}
	h2 := hdr2[ls]
	return h2&0x80 == 0
}

// decodeUE decodes one unsigned Exp-Golomb value from the start of buf.
func decodeUE(buf []byte) (uint32, bool) {
	bit := uint(0)
	byteLen := uint(len(buf)) * 8
	readBit := func() (uint32, bool) {
		if bit >= byteLen {
			return 0, false
		}
		v := uint32(buf[bit/8]>>(7-bit%8)) & 1
		bit++
		return v, true
	}

	zeros := 0
	for {
		b, ok := readBit()
		if !ok {
			return 0, false
		}
		if b == 1 {
			break
		}
		zeros++
		if zeros > 25 {
			return 0, false
		}
	}
	value := uint32(1)
	for i := 0; i < zeros; i++ {
		b, ok := readBit()
		if !ok {
			return 0, false
		}
		value = value<<1 | b
	}
	return value - 1, true
}
