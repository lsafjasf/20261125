package analyze

import (
	"bufio"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
)

// ManifestEntry is one row of a delivery manifest.
type ManifestEntry struct {
	Path string // path as recorded in the manifest
	MD5  string // lowercase hex, empty if unknown
	Size int64  // -1 if unknown
}

// Manifest is a set of entries keyed by cleaned relative path.
type Manifest struct {
	Entries map[string]ManifestEntry
}

// LoadManifest parses a manifest file. Supported formats:
//   - md5sum / md5sum -c output: "<hex>  <path>" (also "*.dpx" binary marker)
//   - CSV with a header row containing at least a path/file column and
//     optionally md5/checksum and size columns.
func LoadManifest(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	first, err := peekLine(path)
	if err != nil {
		return nil, err
	}
	if looksLikeCSVHeader(first) {
		return parseCSVManifest(f)
	}
	return parseMD5Manifest(f)
}

func peekLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if s := strings.TrimSpace(sc.Text()); s != "" {
			return s, nil
		}
	}
	return "", sc.Err()
}

func looksLikeCSVHeader(line string) bool {
	l := strings.ToLower(line)
	if !strings.Contains(l, ",") {
		return false
	}
	for _, key := range []string{"file", "path", "name"} {
		for _, field := range strings.Split(l, ",") {
			if strings.Contains(strings.TrimSpace(field), key) {
				return true
			}
		}
	}
	return false
}

func parseMD5Manifest(f *os.File) (*Manifest, error) {
	m := &Manifest{Entries: map[string]ManifestEntry{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Format: "<32 hex> <space><space|*>path"
		if len(line) < 34 {
			continue
		}
		sum := strings.ToLower(line[:32])
		if !isHex(sum) {
			continue
		}
		rest := strings.TrimLeft(line[32:], " ")
		rest = strings.TrimPrefix(rest, "*")
		if rest == "" {
			continue
		}
		m.Entries[cleanKey(rest)] = ManifestEntry{Path: rest, MD5: sum, Size: -1}
	}
	return m, sc.Err()
}

func parseCSVManifest(f *os.File) (*Manifest, error) {
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &Manifest{Entries: map[string]ManifestEntry{}}, nil
	}
	idxPath, idxMD5, idxSize := -1, -1, -1
	for i, col := range rows[0] {
		switch normalizeCol(col) {
		case "file", "filename", "filepath", "path", "name":
			if idxPath < 0 {
				idxPath = i
			}
		case "md5", "checksum", "md5sum", "hash":
			idxMD5 = i
		case "size", "bytes", "filesize":
			idxSize = i
		}
	}
	m := &Manifest{Entries: map[string]ManifestEntry{}}
	if idxPath < 0 {
		return m, nil
	}
	for _, row := range rows[1:] {
		if idxPath >= len(row) || strings.TrimSpace(row[idxPath]) == "" {
			continue
		}
		e := ManifestEntry{Path: strings.TrimSpace(row[idxPath]), Size: -1}
		if idxMD5 >= 0 && idxMD5 < len(row) {
			e.MD5 = strings.ToLower(strings.TrimSpace(row[idxMD5]))
		}
		if idxSize >= 0 && idxSize < len(row) {
			e.Size = parseInt64(strings.TrimSpace(row[idxSize]))
		}
		m.Entries[cleanKey(e.Path)] = e
	}
	return m, nil
}

func normalizeCol(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '_' || r == '-' {
			return -1
		}
		return r
	}, s)
}

func parseInt64(s string) int64 {
	var n int64 = -1
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		if n < 0 {
			n = 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func isHex(s string) bool {
	for _, c := range s {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return len(s) > 0
}

func cleanKey(p string) string {
	p = strings.TrimPrefix(p, "./")
	return filepath.ToSlash(filepath.Clean(p))
}

// Lookup finds the manifest entry for a scanned file path, tolerating
// leading directory differences by matching on path suffixes.
func (m *Manifest) Lookup(path string) (ManifestEntry, bool) {
	key := cleanKey(path)
	if e, ok := m.Entries[key]; ok {
		return e, true
	}
	base := filepath.Base(key)
	var found ManifestEntry
	count := 0
	for k, e := range m.Entries {
		if strings.HasSuffix(k, "/"+key) || k == key || filepath.Base(k) == base && strings.HasSuffix(key, filepath.Base(k)) {
			found, count = e, count+1
		}
	}
	if count == 1 {
		return found, true
	}
	return ManifestEntry{}, false
}
