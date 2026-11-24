package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEncodePreservesProtectedFields(t *testing.T) {
	src := `{
  "media_id": "m1",
  "name": "spring",
  "media_path": "media/m1.bin",
  "media_hash": "abc123",
  "created_at": "2026-11-01T10:00:00Z",
  "labels": {
    "language": {"value": "en-US", "source": "filesystem"},
    "duration": {"value": "30", "source": "filesystem"}
  },
  "campaign_meta": {"campaign": "keep", "tags": ["a", "b"]},
  "region_override": "apac"
}`
	record, protected, err := Decode([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	edited := map[string]Label{
		FieldLanguage: {Value: "en", Source: SourceBitstream},
		FieldDuration: {Value: "30s", Source: SourceBitstream},
	}
	out, err := Encode(record, protected, edited)
	if err != nil {
		t.Fatal(err)
	}

	var round map[string]json.RawMessage
	if err := json.Unmarshal(out, &round); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"campaign_meta", "region_override", "media_hash", "media_id", "name", "media_path"} {
		if _, ok := round[key]; !ok {
			t.Fatalf("protected field %q dropped", key)
		}
	}
	var hash string
	_ = json.Unmarshal(round["media_hash"], &hash)
	if hash != "abc123" {
		t.Fatalf("media_hash changed: %s", hash)
	}

	// Re-decode via the typed path and confirm only the two labels changed.
	rec2, _, err := Decode(out)
	if err != nil {
		t.Fatal(err)
	}
	if rec2.Labels[FieldLanguage].Value != "en" || rec2.Labels[FieldDuration].Value != "30s" {
		t.Fatalf("labels not applied: %+v", rec2.Labels)
	}
}

func TestRejectsMissingMediaID(t *testing.T) {
	if _, _, err := Decode([]byte(`{"name":"x"}`)); err == nil {
		t.Fatal("sidecar without media_id must be rejected")
	}
}

func TestWriteSidecarAtomicReplacesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteSidecarAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("got %q", got)
	}
}
