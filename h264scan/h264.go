// Package h264scan rebuilds access-unit (sample) boundaries by scanning the
// raw NAL unit stream inside mdat payloads.
//
// It understands AVCC style length prefixed NAL units (the MP4 default) and
// uses slice headers / AUD NALs to locate complete sample boundaries. The
// scanner never modifies media bytes; it only reports offsets and sizes.
package h264scan

import (
	"bufio"
	"encoding/binary"
	"io"
)

// Codec selects NAL classification rules.
type Codec int

const (
	CodecAVC  Codec = iota // H.264 / AVC (default)
	CodecHEVC              // H.265 / HEVC
)

// Options controls a scan.
type Options struct {
	Codec      Codec
	LengthSize int // NAL length prefix width: 1, 2 or 4 (0 => 4)
}

// Sample is one recovered access unit. Offsets are absolute in the file that
// backs the ReaderAt passed to ScanRange.
type Sample struct {
	Offset   int64 // absolute offset of the first length prefix byte
	Size     int64 // complete sample size on disk
	Keyframe bool
	NALCount int
}

// Gap marks bytes that could not be assigned to a complete sample.
type Gap struct {
	Start  int64
	End    int64
	Reason string
}

// Result is the scan outcome for one contiguous media section.
type Result struct {
	Samples  []Sample
	Gaps     []Gap
	CleanEnd int64 // absolute offset immediately after the last complete NAL
}

type nalInfo struct {
	prefixOff int64
	totalWire int64 // length prefix + NAL bytes
	nalLen    int64
	typ       byte
	firstMB   uint32
	firstOK   bool
}

// ScanRange scans one contiguous AVCC NAL region [start,end).
func ScanRange(ra io.ReaderAt, start, end int64, opts Options) (*Result, error) {
	if opts.LengthSize == 0 {
		opts.LengthSize = 4
	}
	section := io.NewSectionReader(ra, start, end-start)
	br := bufio.NewReaderSize(section, 1<<16)

	var nals []nalInfo
	var gaps []Gap
	pos := int64(0) // relative to start

	abs := func(rel int64) int64 { return start + rel }

	for pos < end-start {
		remain := end - abs(pos)
		if remain < int64(opts.LengthSize) {
			gaps = append(gaps, Gap{Start: abs(pos), End: end, Reason: "length prefix truncated at EOF"})
			pos = end - start
			break
		}

		lbuf := make([]byte, opts.LengthSize)
		if _, err := io.ReadFull(br, lbuf); err != nil {
			gaps = append(gaps, Gap{Start: abs(pos), End: end, Reason: "short length prefix read"})
			pos = end - start
			break
		}
		pos += int64(opts.LengthSize)
		nalLen := decodeLength(lbuf)

		if nalLen <= 0 || int64(opts.LengthSize)+nalLen > remain {
			badStart := abs(pos) - int64(opts.LengthSize)
			candidate, ok := resync(ra, br, &pos, start, end, opts)
			if !ok {
				gaps = append(gaps, Gap{Start: badStart, End: end, Reason: "NAL length exceeds available bytes (truncated sample)"})
				pos = end - start
				break
			}
			gaps = append(gaps, Gap{Start: badStart, End: candidate, Reason: "invalid NAL length, skipped to next candidate"})
			continue
		}

		info := nalInfo{
			prefixOff: abs(pos) - int64(opts.LengthSize),
			totalWire: int64(opts.LengthSize) + nalLen,
			nalLen:    nalLen,
		}

		headN := nalLen
		if headN > 16 {
			headN = 16
		}
		head := make([]byte, headN)
		if _, err := io.ReadFull(br, head); err != nil {
			gaps = append(gaps, Gap{Start: info.prefixOff, End: end, Reason: "NAL header truncated"})
			pos = end - start
			break
		}
		pos += headN
		discard := nalLen - headN
		if discard > 0 {
			n, err := br.Discard(int(discard))
			pos += int64(n)
			if err != nil {
				gaps = append(gaps, Gap{Start: info.prefixOff, End: end, Reason: "NAL payload truncated"})
				pos = end - start
				break
			}
		}

		if len(head) > 0 {
			if opts.Codec == CodecHEVC {
				info.typ = (head[0] >> 1) & 0x3f
				if isVCLHEVC(info.typ) && len(head) >= 3 {
					info.firstOK = true
					info.firstMB = uint32((head[2] >> 7) & 1) // first_slice_segment_in_pic_flag
				}
			} else {
				info.typ = head[0] & 0x1f
				if isVCLAVC(info.typ) && len(head) > 1 {
					mb, ok := decodeUE(head[1:])
					info.firstMB, info.firstOK = mb, ok
				}
			}
		}
		nals = append(nals, info)
	}

	cleanEnd := end
	if len(nals) > 0 {
		last := nals[len(nals)-1]
		cleanEnd = last.prefixOff + last.totalWire
	}

	samples := groupSamples(nals, gaps, cleanEnd, opts.Codec)
	return &Result{Samples: samples, Gaps: gaps, CleanEnd: cleanEnd}, nil
}

func decodeLength(b []byte) int64 {
	switch len(b) {
	case 1:
		return int64(b[0])
	case 2:
		return int64(binary.BigEndian.Uint16(b))
	case 4:
		return int64(binary.BigEndian.Uint32(b))
	default:
		var v uint64
		for _, x := range b {
			v = v<<8 | uint64(x)
		}
		return int64(v)
	}
}

func isVCLAVC(t byte) bool { return t >= 1 && t <= 5 }

// HEVC VCL NAL units are types 0..31; types 32+ are non-VCL (VPS/SPS/PPS/...).
func isVCLHEVC(t byte) bool { return t < 32 }

func isIDR(codec Codec, t byte) bool {
	if codec == CodecHEVC {
		return t == 19 || t == 20
	}
	return t == 5
}
