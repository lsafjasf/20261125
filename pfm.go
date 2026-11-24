package sdrquant

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
)

func readPFMToken(r *bufio.Reader) (string, error) {
	var token []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == ' ' || b == '\n' || b == '\r' || b == '\t' {
			if b == '\r' {
				if next, err := r.Peek(1); err == nil && next[0] == '\n' {
					_, _ = r.ReadByte()
				}
			}
			if len(token) > 0 {
				return string(token), nil
			}
			continue
		}
		token = append(token, b)
	}
}

func readPFM(r io.Reader) (*FloatImage, error) {
	br := bufio.NewReader(r)
	magic, err := readPFMToken(br)
	if err != nil {
		return nil, fmt.Errorf("read PFM magic: %w", err)
	}
	channels := 0
	switch magic {
	case "Pf":
		channels = 1
	case "PF":
		channels = 3
	default:
		return nil, fmt.Errorf("invalid PFM magic %q", magic)
	}
	widthToken, err := readPFMToken(br)
	if err != nil {
		return nil, fmt.Errorf("read PFM width: %w", err)
	}
	heightToken, err := readPFMToken(br)
	if err != nil {
		return nil, fmt.Errorf("read PFM height: %w", err)
	}
	scaleToken, err := readPFMToken(br)
	if err != nil {
		return nil, fmt.Errorf("read PFM scale: %w", err)
	}
	width, err1 := strconv.Atoi(widthToken)
	height, err2 := strconv.Atoi(heightToken)
	scale, err3 := strconv.ParseFloat(scaleToken, 32)
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, fmt.Errorf("invalid PFM header %q %q %q", widthToken, heightToken, scaleToken)
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid PFM dimensions %dx%d", width, height)
	}
	pixelCount := int64(width) * int64(height) * int64(channels)
	maxInt := int64(^uint(0) >> 1)
	if pixelCount <= 0 || pixelCount > maxInt/4 {
		return nil, fmt.Errorf("PFM raster too large: %dx%d", width, height)
	}

	im, err := NewFloatImage(width, height, channels)
	if err != nil {
		return nil, err
	}
	littleEndian := scale < 0
	for row := 0; row < height; row++ {
		y := height - 1 - row
		for x := 0; x < width; x++ {
			for c := 0; c < channels; c++ {
				bits, err := readUint32(br, littleEndian)
				if err != nil {
					return nil, fmt.Errorf("read PFM raster at row %d: %w", row, err)
				}
				im.Set(x, y, c, math.Float32frombits(bits))
			}
		}
	}
	return im, nil
}

func readUint32(r io.Reader, littleEndian bool) (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	if littleEndian {
		return binary.LittleEndian.Uint32(buf[:]), nil
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

func writeFloat32(w io.Writer, v float32, littleEndian bool) error {
	var buf [4]byte
	bits := math.Float32bits(v)
	if littleEndian {
		binary.LittleEndian.PutUint32(buf[:], bits)
	} else {
		binary.BigEndian.PutUint32(buf[:], bits)
	}
	_, err := w.Write(buf[:])
	return err
}
