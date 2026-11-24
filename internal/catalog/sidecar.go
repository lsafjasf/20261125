package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Decode reads a sidecar document into a MediaRecord while retaining every
// top-level field that is not part of the typed model. The retained blob is
// re-emitted by Encode so protected/unknown fields are never dropped.
func Decode(data []byte) (*MediaRecord, map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse sidecar: %w", err)
	}

	record := &MediaRecord{}
	if blob, ok := raw["media_id"]; ok {
		if err := json.Unmarshal(blob, &record.MediaID); err != nil {
			return nil, nil, fmt.Errorf("parse media_id: %w", err)
		}
	}
	if record.MediaID == "" {
		return nil, nil, fmt.Errorf("sidecar is missing media_id")
	}
	if blob, ok := raw["name"]; ok {
		_ = json.Unmarshal(blob, &record.Name)
	}
	if blob, ok := raw["media_path"]; ok {
		_ = json.Unmarshal(blob, &record.MediaPath)
	}
	if blob, ok := raw["media_hash"]; ok {
		_ = json.Unmarshal(blob, &record.MediaHash)
	}
	if blob, ok := raw["created_at"]; ok {
		_ = json.Unmarshal(blob, &record.CreatedAt)
	}
	if blob, ok := raw["labels"]; ok {
		if err := json.Unmarshal(blob, &record.Labels); err != nil {
			return nil, nil, fmt.Errorf("parse labels: %w", err)
		}
	}
	if record.Labels == nil {
		record.Labels = map[string]Label{}
	}

	protected := map[string]json.RawMessage{}
	for key, blob := range raw {
		if key == "labels" {
			continue
		}
		protected[key] = blob
	}
	return record, protected, nil
}

// Encode serialises a record after applying label edits. Typed protected
// fields are rendered from the model; any extra unknown top-level fields are
// carried over untouched. Key order is deterministic for stable diffs.
func Encode(record *MediaRecord, protected map[string]json.RawMessage, edited map[string]Label) ([]byte, error) {
	out := map[string]json.RawMessage{}

	put := func(key string, value any) error {
		blob, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode %s: %w", key, err)
		}
		out[key] = blob
		return nil
	}

	if err := put("media_id", record.MediaID); err != nil {
		return nil, err
	}
	if err := put("name", record.Name); err != nil {
		return nil, err
	}
	if err := put("media_path", record.MediaPath); err != nil {
		return nil, err
	}
	if err := put("media_hash", record.MediaHash); err != nil {
		return nil, err
	}
	if !record.CreatedAt.IsZero() {
		if err := put("created_at", record.CreatedAt); err != nil {
			return nil, err
		}
	}

	mergedLabels := map[string]Label{}
	for field, label := range record.Labels {
		mergedLabels[field] = label
	}
	for field, label := range edited {
		mergedLabels[field] = label
	}
	if err := put("labels", mergedLabels); err != nil {
		return nil, err
	}

	for key, blob := range protected {
		if _, exists := out[key]; exists {
			continue
		}
		out[key] = blob
	}

	keys := make([]string, 0, len(out))
	for key := range out {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBlob, _ := json.Marshal(key)
		buf.Write(keyBlob)
		buf.WriteByte(':')
		buf.Write(out[key])
	}
	buf.WriteByte('}')
	buf.WriteByte('\n')

	var compact bytes.Buffer
	if err := json.Indent(&compact, buf.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("re-indent sidecar: %w", err)
	}
	return compact.Bytes(), nil
}

// LoadSidecar reads and decodes one sidecar file.
func LoadSidecar(path string) (*MediaRecord, map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	record, protected, err := Decode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	record.SetSidecarPath(path)
	return record, protected, nil
}

// WriteSidecarAtomic replaces path with data using a same-directory temp
// file and rename, so readers never observe a half-written sidecar.
func WriteSidecarAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sidecar-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// ReadMediaHash streams a media file and returns its lowercase hex SHA-256.
func ReadMediaHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return hashReader(f)
}

func hashReader(r io.Reader) (string, error) {
	return sha256Hex(r)
}
