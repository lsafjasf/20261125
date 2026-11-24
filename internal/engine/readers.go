package engine

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// patternReader emits a small deterministic, self-describing blob.
type patternReader struct {
	total   int64
	pos     int64
	filler  byte
	header  []byte
	headerR *bytes.Reader
}

func newPatternReader(jobID, kind string, total int64, filler byte) *io.LimitedReader {
	header := make([]byte, 64)
	copy(header[0:8], []byte("BPREP001"))
	binary.BigEndian.PutUint64(header[8:16], uint64(total))
	copy(header[16:48], []byte(jobID))
	copy(header[48:], []byte(kind))
	pr := &patternReader{
		total:   total,
		filler:  filler,
		header:  header,
		headerR: bytes.NewReader(header),
	}
	return &io.LimitedReader{R: pr, N: total + int64(len(header))}
}

func (r *patternReader) Read(p []byte) (int, error) {
	written := 0
	if r.headerR != nil {
		n, err := r.headerR.Read(p)
		written += n
		if errors.Is(err, io.EOF) {
			r.headerR = nil
		} else if err != nil {
			return written, err
		}
		if written > 0 {
			return written, nil
		}
	}
	if r.pos >= r.total {
		if written > 0 {
			return written, nil
		}
		return 0, io.EOF
	}
	n := len(p) - written
	if remain := r.total - r.pos; int64(n) > remain {
		n = int(remain)
	}
	target := p[written:]
	for i := 0; i < n; i++ {
		target[i] = r.filler + byte((r.pos+int64(i))%37)
	}
	r.pos += int64(n)
	written += n
	if r.pos >= r.total {
		return written, io.EOF
	}
	return written, nil
}

// encodeReader prepends an MP4-like header and transforms render bytes.
type encodeReader struct {
	src      io.Reader
	header   []byte
	headerAt int
}

func newEncodeReader(src io.Reader, jobID string) *encodeReader {
	header := make([]byte, 4096)
	copy(header[0:12], []byte("\x00\x00\x00\x18ftypmp42"))
	copy(header[32:64], []byte(jobID))
	return &encodeReader{src: src, header: header}
}

func (r *encodeReader) Read(p []byte) (int, error) {
	if r.headerAt < len(r.header) {
		n := copy(p, r.header[r.headerAt:])
		r.headerAt += n
		return n, nil
	}
	n, err := r.src.Read(p)
	for i := 0; i < n; i++ {
		p[i] = p[i] ^ 0x5A
	}
	return n, err
}
