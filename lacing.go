package matroska

import "fmt"

const defaultTimestampScale int64 = 1_000_000

// ParseBlock parses the payload of a Block (0xA1) or SimpleBlock (0xA3).
// PayloadOffset and ElementOffset are absolute file offsets; timestamps are
// expressed in Matroska ticks and nanoseconds.
func ParseBlock(payload []byte, opts BlockOptions) (*Block, error) {
	if len(payload) == 0 {
		return nil, parseError(ErrTruncated, opts.PayloadOffset, -1, "empty Block payload")
	}
	scale := opts.TimestampScale
	if scale <= 0 {
		scale = defaultTimestampScale
	}

	track, trackWidth, err := readVINT(payload, 0)
	if err != nil {
		return nil, blockError(err, opts.PayloadOffset, -1)
	}
	if track == 0 {
		return nil, parseError(ErrInvalidBlock, opts.PayloadOffset, -1,
			fmt.Sprintf("invalid track number %d", track))
	}
	if len(payload) < trackWidth+3 {
		return nil, parseError(ErrTruncated, opts.PayloadOffset, -1,
			"Block header is shorter than track number, timecode, and flags")
	}

	relative := int16(uint16(payload[trackWidth])<<8 | uint16(payload[trackWidth+1]))
	flags := payload[trackWidth+2]
	pos := trackWidth + 3
	lacing := Lacing(flags & 0x06)

	frameCount := 1
	if lacing != LacingNone {
		if pos >= len(payload) {
			return nil, parseError(ErrTruncated, opts.PayloadOffset+int64(pos), -1,
				"missing lacing frame count")
		}
		frameCount = int(payload[pos]) + 1
		pos++
	}

	sizes, dataStart, err := parseLaceSizes(payload, pos, frameCount, lacing, opts.PayloadOffset)
	if err != nil {
		return nil, err
	}
	if len(sizes) != frameCount {
		return nil, parseError(ErrInvalidBlock, opts.PayloadOffset, -1,
			fmt.Sprintf("decoded %d frame sizes for %d frames", len(sizes), frameCount))
	}

	blockTicks := opts.ClusterTimecode + int64(relative)
	blockNanos := blockTicks * scale
	frames := make([]Frame, frameCount)
	cursor := opts.PayloadOffset + int64(dataStart)
	for i, size := range sizes {
		if size <= 0 {
			kind := ErrFrameSize
			if i == 0 {
				kind = ErrFirstFrameSize
			}
			return nil, parseError(kind, cursor, i, "frame length must be positive")
		}
		timeNanos := blockNanos
		timecode := blockTicks
		if opts.DefaultFrameDuration > 0 {
			timeNanos += int64(i) * opts.DefaultFrameDuration
			timecode += int64(i) * opts.DefaultFrameDuration / scale
		}
		frames[i] = Frame{
			Index:     i,
			Start:     cursor,
			End:       cursor + int64(size),
			Timecode:  timecode,
			TimeNanos: timeNanos,
		}
		cursor += int64(size)
	}
	if cursor != opts.PayloadOffset+int64(len(payload)) {
		return nil, parseError(ErrFrameSize, cursor, -1,
			fmt.Sprintf("frames cover %d bytes, Block payload has %d", cursor-opts.PayloadOffset, len(payload)))
	}

	return &Block{
		TrackNumber:      track,
		RelativeTimecode: relative,
		Timecode:         blockTicks,
		TimeNanos:        blockNanos,
		Flags:            flags,
		Lacing:           lacing,
		FrameCount:       frameCount,
		ElementOffset:    opts.ElementOffset,
		DataOffset:       opts.PayloadOffset + int64(dataStart),
		Frames:           frames,
	}, nil
}

func parseLaceSizes(payload []byte, pos, count int, lacing Lacing, base int64) ([]int, int, error) {
	switch lacing {
	case LacingNone:
		size := len(payload) - pos
		if size <= 0 {
			return nil, 0, parseError(ErrFrameSize, base+int64(pos), 0, "empty frame")
		}
		return []int{size}, pos, nil
	case LacingFixed:
		return fixedLaceSizes(payload, pos, count, base)
	case LacingXiph:
		return xiphLaceSizes(payload, pos, count, base)
	case LacingEBML:
		return ebmlLaceSizes(payload, pos, count, base)
	default:
		return nil, 0, parseError(ErrUnsupportedLacing, base+int64(pos), -1,
			fmt.Sprintf("lacing bits 0x%02x", byte(lacing)))
	}
}

func fixedLaceSizes(payload []byte, pos, count int, base int64) ([]int, int, error) {
	remaining := len(payload) - pos
	if count <= 0 || remaining%count != 0 {
		return nil, 0, parseError(ErrFrameSize, base+int64(pos), -1,
			fmt.Sprintf("%d data bytes cannot be split into %d fixed frames", remaining, count))
	}
	size := remaining / count
	if size == 0 {
		return nil, 0, parseError(ErrFirstFrameSize, base+int64(pos), 0, "empty fixed-laced frame")
	}
	sizes := make([]int, count)
	for i := range sizes {
		sizes[i] = size
	}
	return sizes, pos, nil
}

func blockError(err error, offset int64, frame int) error {
	if err == nil {
		return nil
	}
	kind := ErrInvalidBlock
	if ce, ok := err.(*codecError); ok {
		kind = ce.kind
	}
	return &ParseError{Kind: kind, Offset: offset, Frame: frame, Detail: err.Error(), Cause: err}
}
