package audit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type y4mHeader struct {
	width       int
	height      int
	chroma      string
	lumaBytes   int64
	chromaBytes int64
}

func parseY4MHeader(line string) (y4mHeader, error) {
	var h y4mHeader
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != "YUV4MPEG2" {
		return h, errors.New("missing YUV4MPEG2 signature")
	}
	h.chroma = "420jpeg"
	for _, field := range fields[1:] {
		if len(field) < 2 {
			continue
		}
		value := field[1:]
		switch field[0] {
		case 'W':
			h.width, _ = strconv.Atoi(value)
		case 'H':
			h.height, _ = strconv.Atoi(value)
		case 'C':
			h.chroma = strings.ToLower(value)
		}
	}
	if h.width <= 0 || h.height <= 0 {
		return h, errors.New("Y4M header is missing valid width or height")
	}
	pixels := int64(h.width) * int64(h.height)
	h.lumaBytes = pixels
	switch h.chroma {
	case "420", "420jpeg", "420mpeg2", "420paldv":
		if h.width%2 != 0 || h.height%2 != 0 {
			return h, errors.New("4:2:0 Y4M requires even width and height")
		}
		h.chromaBytes = pixels / 2
	case "422":
		if h.width%2 != 0 {
			return h, errors.New("4:2:2 Y4M requires even width")
		}
		h.chromaBytes = pixels
	case "444":
		h.chromaBytes = pixels * 2
	case "mono":
		h.chromaBytes = 0
	default:
		return h, fmt.Errorf("unsupported Y4M chroma format %q (8-bit planar formats only)", h.chroma)
	}
	return h, nil
}

func loadY4M(path string, fileBytes int64, maxSamples uint64) (LoadInfo, *Accumulator, error) {
	f, err := os.Open(path)
	if err != nil {
		return LoadInfo{}, nil, err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 64*1024)
	headerLine, err := r.ReadString('\n')
	if err != nil {
		return LoadInfo{}, nil, fmt.Errorf("read Y4M header: %w", err)
	}
	header, err := parseY4MHeader(strings.TrimSpace(headerLine))
	if err != nil {
		return LoadInfo{}, nil, err
	}

	frameBytes := header.lumaBytes + header.chromaBytes + 6
	approxFrames := uint64(1)
	if frameBytes > 0 && fileBytes > int64(len(headerLine)) {
		approxFrames = uint64(fileBytes-int64(len(headerLine))) / uint64(frameBytes)
		if approxFrames == 0 {
			approxFrames = 1
		}
	}
	stride := sampleStride(approxFrames*uint64(header.lumaBytes), maxSamples)
	acc := NewAccumulator()
	yPlane := make([]byte, header.lumaBytes)
	frames := 0
	var lumaIndex uint64

	for {
		frameLine, err := r.ReadString('\n')
		if err == io.EOF && frameLine == "" {
			break
		}
		if err != nil {
			return LoadInfo{}, nil, fmt.Errorf("read frame header: %w", err)
		}
		if !strings.HasPrefix(strings.TrimSpace(frameLine), "FRAME") {
			return LoadInfo{}, nil, errors.New("invalid Y4M frame header")
		}
		if _, err := io.ReadFull(r, yPlane); err != nil {
			return LoadInfo{}, nil, fmt.Errorf("read luma frame %d: %w", frames, err)
		}
		for _, v := range yPlane {
			if lumaIndex%stride == 0 {
				acc.Add(v)
			}
			lumaIndex++
		}
		if header.chromaBytes > 0 {
			if _, err := io.CopyN(io.Discard, r, header.chromaBytes); err != nil {
				return LoadInfo{}, nil, fmt.Errorf("read chroma frame %d: %w", frames, err)
			}
		}
		frames++
	}
	if frames == 0 {
		return LoadInfo{}, nil, errors.New("Y4M contains no frames")
	}
	return LoadInfo{
		Path:      path,
		Format:    "y4m/" + header.chroma,
		Width:     header.width,
		Height:    header.height,
		Frames:    frames,
		Sampled:   acc.Total(),
		FileBytes: fileBytes,
	}, acc, nil
}
