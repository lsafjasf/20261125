package matroska

import "fmt"

func xiphLaceSizes(payload []byte, pos, count int, base int64) ([]int, int, error) {
	sizes := make([]int, count)
	encodedAt := pos
	sum := 0
	for i := 0; i < count-1; i++ {
		size := 0
		for {
			if encodedAt >= len(payload) {
				return nil, 0, lacingTruncated(base+int64(encodedAt), i,
					"Xiph length chain reaches end of Block")
			}
			part := int(payload[encodedAt])
			encodedAt++
			size += part
			if part != 255 {
				break
			}
		}
		if i == 0 && size == 0 {
			return nil, 0, parseError(ErrFirstFrameSize, base+int64(pos), 0,
				"Xiph first frame length is zero")
		}
		if size > len(payload)-encodedAt {
			if i == 0 {
				return nil, 0, firstSizeOverflow(base+int64(pos), i, size, len(payload)-encodedAt)
			}
			return nil, 0, parseError(ErrFrameSize, base+int64(pos), i,
				fmt.Sprintf("Xiph frame length %d exceeds %d remaining bytes", size, len(payload)-encodedAt))
		}
		sizes[i] = size
		sum += size
	}
	last := len(payload) - encodedAt - sum
	if last < 0 {
		return nil, 0, parseError(ErrFrameSize, base+int64(encodedAt), count-1,
			"Xiph frame lengths exceed Block payload")
	}
	if last == 0 {
		return nil, 0, parseError(ErrFrameSize, base+int64(encodedAt+sum), count-1,
			"Xiph last frame is empty")
	}
	sizes[count-1] = last
	return sizes, encodedAt, nil
}

func ebmlLaceSizes(payload []byte, pos, count int, base int64) ([]int, int, error) {
	sizes := make([]int, count)
	first, width, err := readVINT(payload, pos)
	if err != nil {
		return nil, 0, firstLaceError(base+int64(pos), err)
	}
	cursor := pos + width
	if first == 0 {
		return nil, 0, parseError(ErrFirstFrameSize, base+int64(pos), 0,
			"EBML first frame length is zero")
	}
	if first > uint64(len(payload)-cursor) {
		return nil, 0, firstSizeOverflow(base+int64(pos), 0, int(first), len(payload)-cursor)
	}
	previous := int64(first)
	sizes[0] = int(first)
	sum := int(first)

	for i := 1; i < count-1; i++ {
		delta, deltaWidth, err := readSignedLacingVINT(payload, cursor)
		if err != nil {
			return nil, 0, blockError(err, base+int64(cursor), i)
		}
		next := previous + delta
		if next <= 0 {
			return nil, 0, &ParseError{
				Kind:   ErrNegativeDelta,
				Offset: base + int64(cursor),
				Frame:  i,
				Detail: fmt.Sprintf("delta %d changes frame length from %d to %d", delta, previous, next),
			}
		}
		if next > int64(len(payload)-cursor) {
			return nil, 0, parseError(ErrFrameSize, base+int64(cursor), i,
				fmt.Sprintf("frame length %d exceeds remaining payload", next))
		}
		cursor += deltaWidth
		previous = next
		sizes[i] = int(next)
		sum += int(next)
	}

	last := len(payload) - cursor - sum
	if last < 0 {
		return nil, 0, parseError(ErrFrameSize, base+int64(cursor), count-1,
			"EBML frame lengths exceed Block payload")
	}
	if last == 0 {
		return nil, 0, parseError(ErrFrameSize, base+int64(cursor+sum), count-1,
			"EBML last frame is empty")
	}
	sizes[count-1] = last
	return sizes, cursor, nil
}

func firstLaceError(offset int64, cause error) *ParseError {
	pe := &ParseError{
		Kind:   ErrFirstFrameSize,
		Offset: offset,
		Frame:  0,
		Detail: "cannot decode first frame length",
		Cause:  cause,
	}
	return pe
}

func firstSizeOverflow(offset int64, frame, size, remaining int) *ParseError {
	return &ParseError{
		Kind:   ErrFirstFrameSize,
		Offset: offset,
		Frame:  frame,
		Detail: fmt.Sprintf("first frame length %d exceeds %d remaining bytes", size, remaining),
		Cause:  codecErr(ErrTruncated, "first frame length overruns Block payload"),
	}
}

func lacingTruncated(offset int64, frame int, detail string) *ParseError {
	pe := &ParseError{Kind: ErrTruncated, Offset: offset, Frame: frame, Detail: detail}
	if frame == 0 {
		pe.Kind = ErrFirstFrameSize
		pe.Cause = codecErr(ErrTruncated, detail)
	}
	return pe
}
