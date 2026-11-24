// Package framemd5 reads the textual per-frame hash manifests emitted by
// FFmpeg's framemd5/framecrc muxers. It does not invoke FFmpeg.
package framemd5

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Entry struct {
	Stream   int
	DTS      float64
	PTS      float64
	Duration float64
	Size     int
	Hash     string
}

type Manifest struct {
	Path    string
	Hash    string
	Entries []Entry
}

func ParseFile(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := &Manifest{Path: path}
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#hash:") {
				m.Hash = strings.TrimSpace(strings.TrimPrefix(line, "#hash:"))
			}
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 6 {
			return nil, fmt.Errorf("%s:%d: expected at least 6 comma-separated fields", path, lineNo)
		}
		entry := Entry{Hash: strings.ToLower(strings.TrimSpace(fields[len(fields)-1]))}
		var err error
		if entry.Stream, err = strconv.Atoi(strings.TrimSpace(fields[0])); err != nil {
			return nil, fmt.Errorf("%s:%d: stream: %w", path, lineNo, err)
		}
		if entry.DTS, err = strconv.ParseFloat(strings.TrimSpace(fields[1]), 64); err != nil {
			return nil, fmt.Errorf("%s:%d: dts: %w", path, lineNo, err)
		}
		if entry.PTS, err = strconv.ParseFloat(strings.TrimSpace(fields[2]), 64); err != nil {
			return nil, fmt.Errorf("%s:%d: pts: %w", path, lineNo, err)
		}
		if entry.Duration, err = strconv.ParseFloat(strings.TrimSpace(fields[3]), 64); err != nil {
			return nil, fmt.Errorf("%s:%d: duration: %w", path, lineNo, err)
		}
		sizeField := strings.TrimSpace(fields[len(fields)-2])
		if sizeField != "" {
			if entry.Size, err = strconv.Atoi(sizeField); err != nil {
				// Some manifests omit the size or use a different column.
				entry.Size = 0
			}
		}
		if entry.Hash == "" {
			return nil, fmt.Errorf("%s:%d: empty hash", path, lineNo)
		}
		m.Entries = append(m.Entries, entry)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(m.Entries) == 0 {
		return nil, fmt.Errorf("%s: no frame hash entries", path)
	}
	return m, nil
}

// Hashes returns hashes for the requested stream, preserving file order.
func (m *Manifest) Hashes(stream int) []string {
	out := make([]string, 0, len(m.Entries))
	for _, e := range m.Entries {
		if e.Stream == stream {
			out = append(out, e.Hash)
		}
	}
	return out
}
