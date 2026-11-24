package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DirFSync fsyncs a directory so that recent rename/unlink entries are
// durable across a crash. Errors are tolerated on filesystems that do not
// support fsync on directories.
func DirFSync(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return nil // not supported everywhere; rename itself is atomic
	}
	return nil
}

// AtomicWriteReader streams r into a unique temp file in the same directory
// as target, fsyncs it and renames it over target. It returns size and
// SHA-256 of the written bytes. Readers/writers never observe a truncated
// target because replacement is atomic.
func AtomicWriteReader(target string, r io.Reader, perm os.FileMode) (int64, string, error) {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, "", err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(target)+"-*")
	if err != nil {
		return 0, "", fmt.Errorf("create temp for %s: %w", target, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }

	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hasher), r)
	if err != nil {
		cleanup()
		return 0, "", fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return 0, "", fmt.Errorf("fsync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return 0, "", err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		_ = os.Remove(tmpName)
		return 0, "", err
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return 0, "", fmt.Errorf("rename -> %s: %w", target, err)
	}
	_ = DirFSync(dir)
	return n, hex.EncodeToString(hasher.Sum(nil)), nil
}

// WriteJSONAtomic marshals v and atomically writes it to path.
func WriteJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, _, err = AtomicWriteReader(path, byteReader(data), 0o644)
	return err
}

type bytesReader struct {
	b []byte
	i int
}

func byteReader(b []byte) io.Reader { return &bytesReader{b: b} }

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

// ReadJSON reads and unmarshals a JSON document.
func ReadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// FileSHA256 returns size and hex SHA-256 of path.
func FileSHA256(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// Quarantine moves a suspicious (truncated / corrupt / legacy) artifact
// aside instead of deleting user bytes outright.
func (l *Layout) Quarantine(path, reason string) (string, error) {
	base := filepath.Base(path)
	dest := filepath.Join(l.QuarantineDir,
		fmt.Sprintf("%s.%s.%s", base, reason, timestampForName()))
	if err := os.MkdirAll(l.QuarantineDir, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(path, dest); err != nil {
		return "", err
	}
	_ = DirFSync(l.QuarantineDir)
	return dest, nil
}
