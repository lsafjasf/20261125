package cbarcheck

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ReadRaw444 reads one planar yuv444p frame. 8-bit frames use one byte per
// sample; 10- and 12-bit frames use little-endian uint16 words.
func ReadRaw444(r io.Reader, width, height int, depth BitDepth) (*Frame, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid frame size %dx%d", width, height)
	}
	if err := (Config{Range: RangeLimited, Matrix: MatrixBT709, Depth: depth}).Validate(); err != nil {
		return nil, err
	}
	n := width * height
	f := &Frame{
		Width: width, Height: height, Depth: depth,
		Y: make([]uint16, n), Cb: make([]uint16, n), Cr: make([]uint16, n),
	}
	planes := []struct {
		name string
		dst  []uint16
	}{{"Y", f.Y}, {"Cb", f.Cb}, {"Cr", f.Cr}}
	for _, p := range planes {
		if err := readPlane(r, p.name, p.dst, depth); err != nil {
			return nil, err
		}
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return f, nil
}

func readPlane(r io.Reader, name string, dst []uint16, depth BitDepth) error {
	if depth == Depth8 {
		buf := make([]byte, len(dst))
		if _, err := io.ReadFull(r, buf); err != nil {
			return fmt.Errorf("read %s plane: %w", name, err)
		}
		for i, v := range buf {
			dst[i] = uint16(v)
		}
		return nil
	}
	buf := make([]byte, len(dst)*2)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("read %s plane: %w", name, err)
	}
	max := (1 << depth) - 1
	for i := range dst {
		v := binary.LittleEndian.Uint16(buf[i*2:])
		if int(v) > max {
			return fmt.Errorf("%s[%d]=%d exceeds %d-bit range", name, i, v, depth)
		}
		dst[i] = v
	}
	return nil
}

// WriteRaw444 writes one planar yuv444p frame in the same representation read
// by ReadRaw444.
func WriteRaw444(w io.Writer, f *Frame) error {
	if err := f.Validate(); err != nil {
		return err
	}
	planes := []struct {
		name string
		src  []uint16
	}{{"Y", f.Y}, {"Cb", f.Cb}, {"Cr", f.Cr}}
	for _, p := range planes {
		if f.Depth == Depth8 {
			buf := make([]byte, len(p.src))
			for i, v := range p.src {
				buf[i] = byte(v)
			}
			if _, err := w.Write(buf); err != nil {
				return fmt.Errorf("write %s plane: %w", p.name, err)
			}
			continue
		}
		buf := make([]byte, len(p.src)*2)
		for i, v := range p.src {
			binary.LittleEndian.PutUint16(buf[i*2:], v)
		}
		if _, err := w.Write(buf); err != nil {
			return fmt.Errorf("write %s plane: %w", p.name, err)
		}
	}
	return nil
}

// Raw444Size is the number of bytes in one frame.
func Raw444Size(width, height int, depth BitDepth) int64 {
	bytesPerSample := int64(1)
	if depth == Depth10 || depth == Depth12 {
		bytesPerSample = 2
	}
	return int64(width) * int64(height) * 3 * bytesPerSample
}
