package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Entry is one loaded sidecar plus the raw fields needed to guarantee
// protected-field preservation during a write.
type Entry struct {
	Record    *MediaRecord
	Protected map[string]json.RawMessage

	// OriginalMode is the sidecar permission captured at load time.
	OriginalMode os.FileMode
	// Writable reports whether the process can currently replace the file.
	Writable bool
	// MediaExists reports whether the referenced media file is present.
	MediaExists bool
	// HashOK reports whether the on-disk media SHA-256 matches media_hash.
	HashOK bool
}

// Library is the fully indexed media library under one root directory.
type Library struct {
	Root string

	// ByID indexes every record by its unique media_id.
	ByID map[string]*Entry
	// Names maps a display name to every record sharing it. More than one
	// entry means same-name multi-language variants, which must never be
	// conflated: corrections always target media_id, never the name.
	Names map[string][]*Entry

	entries []*Entry
}

// Entries returns all loaded entries in stable sidecar-path order.
func (l *Library) Entries() []*Entry { return l.entries }

// Load walks root, loads every *.json sidecar, and indexes the library.
// Media hashes are verified eagerly so a tampered file aborts before any
// write is attempted. Duplicate media_id values are rejected outright.
func Load(root string) (*Library, error) {
	lib := &Library{
		Root:  root,
		ByID:  map[string]*Entry{},
		Names: map[string][]*Entry{},
	}

	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		// Only files under a metadata directory are sidecars; sibling
		// directories (batches, reports ...) may contain arbitrary JSON.
		if filepath.Ext(path) == ".json" && filepath.Base(filepath.Dir(path)) == "metadata" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk library: %w", err)
	}

	for _, path := range paths {
		entry, err := loadEntry(root, path)
		if err != nil {
			return nil, err
		}
		if _, dup := lib.ByID[entry.Record.MediaID]; dup {
			return nil, fmt.Errorf("duplicate media_id %q in %s", entry.Record.MediaID, path)
		}
		lib.ByID[entry.Record.MediaID] = entry
		lib.Names[entry.Record.Name] = append(lib.Names[entry.Record.Name], entry)
		lib.entries = append(lib.entries, entry)
	}
	return lib, nil
}

func loadEntry(root, path string) (*Entry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	record, protected, err := LoadSidecar(path)
	if err != nil {
		return nil, err
	}

	entry := &Entry{
		Record:       record,
		Protected:    protected,
		OriginalMode: info.Mode().Perm(),
		Writable:     info.Mode().Perm()&0o200 != 0,
	}

	mediaPath := record.MediaPath
	if !filepath.IsAbs(mediaPath) {
		mediaPath = filepath.Join(root, mediaPath)
	}
	if mediaInfo, statErr := os.Stat(mediaPath); statErr == nil && !mediaInfo.IsDir() {
		entry.MediaExists = true
		if actual, hashErr := ReadMediaHash(mediaPath); hashErr == nil {
			entry.HashOK = actual == record.MediaHash
		}
	}
	return entry, nil
}

// SameNameVariants returns the records sharing name with the given entry,
// including itself. A slice longer than one signals multi-language variants.
func (l *Library) SameNameVariants(entry *Entry) []*Entry {
	return l.Names[entry.Record.Name]
}
