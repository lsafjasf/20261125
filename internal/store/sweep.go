package store

import (
	"os"
	"path/filepath"
	"strings"
)

// SweepTempFiles removes stale atomic-write temp files (".tmp-*") left by a
// crashed previous process. It must only be called when no other batch
// process is writing to OutDir. Quarantine contents are preserved.
func (l *Layout) SweepTempFiles() ([]string, error) {
	var removed []string
	err := filepath.Walk(l.OutDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // disappear/permission races are non-fatal
		}
		if info.IsDir() {
			if path == l.QuarantineDir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), ".tmp-") {
			if rmErr := os.Remove(path); rmErr == nil {
				removed = append(removed, path)
			}
		}
		return nil
	})
	return removed, err
}
