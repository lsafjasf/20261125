package aes3

import "encoding/hex"

type statusBuilder struct {
	segments []StatusSegment
	current  *StatusSegment
}

func buildChannelStatus(sfs []Subframe, gapBefore []bool) ChannelStatusReport {
	left := &statusBuilder{}
	right := &statusBuilder{}

	flushAll := func() {
		left.flush()
		right.flush()
	}

	for i, sf := range sfs {
		if gapBefore[i] {
			flushAll()
		}

		builder := left
		if sf.Channel == "right" {
			builder = right
		}

		if !sf.Trusted {
			builder.flush()
			continue
		}

		aligned := sf.BlockStart
		if !aligned && sf.Channel == "right" && i > 0 && !gapBefore[i] &&
			sfs[i-1].Channel == "left" && sfs[i-1].BlockStart {
			aligned = true
		}
		builder.add(sf, aligned)
	}
	left.flush()
	right.flush()

	leftSegments := left.segments
	rightSegments := right.segments
	if leftSegments == nil {
		leftSegments = []StatusSegment{}
	}
	if rightSegments == nil {
		rightSegments = []StatusSegment{}
	}
	report := ChannelStatusReport{Left: leftSegments, Right: rightSegments}
	report.CompleteLeft192 = firstAlignedBlock(left.segments)
	report.CompleteRight192 = firstAlignedBlock(right.segments)
	return report
}

func (b *statusBuilder) add(sf Subframe, blockAligned bool) {
	if b.current == nil {
		b.current = &StatusSegment{
			Channel:       sf.Channel,
			StartSubframe: sf.Index,
			StartTime:     sf.StartTime,
			BlockAligned:  blockAligned,
			Bits:          []int{},
		}
	}
	b.current.Bits = append(b.current.Bits, boolToInt(sf.ChannelStatusBit))
	b.current.EndSubframe = sf.Index
	b.current.EndTime = sf.EndTime
}

func (b *statusBuilder) flush() {
	if b.current == nil {
		return
	}
	seg := *b.current
	rawBytes := bitsToBytesLSBFirst(seg.Bits)
	seg.BytesLSBFirst = bytesToInts(rawBytes)
	seg.Hex = hex.EncodeToString(rawBytes)
	b.segments = append(b.segments, seg)
	b.current = nil
}

func bitsToBytesLSBFirst(bits []int) []byte {
	out := make([]byte, (len(bits)+7)/8)
	for i, bit := range bits {
		if bit != 0 {
			out[i/8] |= 1 << uint(i%8)
		}
	}
	return out
}

func bytesToInts(in []byte) []int {
	out := make([]int, len(in))
	for i, b := range in {
		out[i] = int(b)
	}
	return out
}

func firstAlignedBlock(segments []StatusSegment) []int {
	for _, seg := range segments {
		if seg.BlockAligned && len(seg.Bits) >= 192 {
			block := make([]int, 192)
			copy(block, seg.Bits[:192])
			return bytesToInts(bitsToBytesLSBFirst(block))
		}
	}
	return nil
}
