package media

import (
	"errors"
	"fmt"
	"io"
	"os"
)

func ReadFile(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 8 {
		return nil, fmt.Errorf("%s: file is too small", path)
	}
	var head [8]byte
	if _, err := f.ReadAt(head[:], 0); err != nil {
		return nil, err
	}
	var mf *File
	switch {
	case isISOBMFFStart(head[:]):
		mf, err = readMP4(f, info.Size())
	case head[0] == 0x1a && head[1] == 0x45 && head[2] == 0xdf && head[3] == 0xa3:
		mf, err = readMatroska(f, info.Size())
	default:
		return nil, fmt.Errorf("%s: unsupported container (want ISO BMFF/QuickTime or Matroska)", path)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	mf.Path = path
	for i := range mf.Tracks {
		if err := mf.Tracks[i].Validate(); err != nil {
			return nil, fmt.Errorf("%s: track %d: %w", path, mf.Tracks[i].ID, err)
		}
	}
	return mf, nil
}

func isISOBMFFStart(head []byte) bool {
	if len(head) < 8 {
		return false
	}
	switch string(head[4:8]) {
	case "ftyp", "moov", "mdat", "free", "wide", "skip", "uuid":
		return true
	default:
		return false
	}
}

func readAtFull(r io.ReaderAt, off, n int64) ([]byte, error) {
	if n < 0 || off < 0 {
		return nil, errors.New("negative read")
	}
	buf := make([]byte, n)
	if n == 0 {
		return buf, nil
	}
	got, err := r.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if int64(got) != n {
		return nil, io.ErrUnexpectedEOF
	}
	return buf, nil
}
