// Package fixture builds a deterministic ad media library and correction
// batches for demos and acceptance tests. Generated media files are small
// deterministic byte payloads; their real SHA-256 hashes are recorded in the
// sidecars so the repair tool can prove the media bytes never change.
package fixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"adlabel/internal/catalog"
)

// fixtureRecord describes one generated ad in plain data terms.
type fixtureRecord struct {
	id        string
	name      string
	mediaFile string
	mediaBody []byte
	createdAt time.Time
	labels    map[string]catalog.Label
	readOnly  bool
	extra     map[string]json.RawMessage
}

// Generate writes the full fixture tree under out and returns no error only
// when every file is in place.
func Generate(out string) error {
	mediaDir := filepath.Join(out, "media")
	metaDir := filepath.Join(out, "metadata")
	batchDir := filepath.Join(out, "batches")
	for _, dir := range []string{mediaDir, metaDir, batchDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	records := buildRecords()
	paths := map[string]string{}
	for _, rec := range records {
		mediaPath := filepath.Join(mediaDir, rec.mediaFile)
		if err := os.WriteFile(mediaPath, rec.mediaBody, 0o644); err != nil {
			return err
		}
		hash := catalog.HashBytes(rec.mediaBody)

		doc := map[string]any{
			"media_id":   rec.id,
			"name":       rec.name,
			"media_path": filepath.ToSlash(filepath.Join("media", rec.mediaFile)),
			"media_hash": hash,
			"created_at": rec.createdAt.UTC().Format(time.RFC3339),
			"labels":     rec.labels,
		}
		for key, blob := range rec.extra {
			doc[key] = blob
		}
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		sidecarPath := filepath.Join(metaDir, rec.id+".json")
		mode := os.FileMode(0o644)
		if rec.readOnly {
			mode = 0o444
		}
		if err := os.WriteFile(sidecarPath, data, mode); err != nil {
			return err
		}
		paths[rec.id] = sidecarPath
	}

	if err := writeBatches(batchDir); err != nil {
		return err
	}

	manifest := map[string]any{
		"generated_at": time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"library_root": out,
		"records":      recordSummaries(records),
		"read_only":    readOnlyIDs(records),
	}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	return os.WriteFile(filepath.Join(out, "manifest.json"), append(manifestData, '\n'), 0o644)
}

func buildRecords() []fixtureRecord {
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	at := func(n int) time.Time { return base.Add(time.Duration(n) * time.Hour) }

	label := func(source catalog.EvidenceSource, value string, ev ...catalog.Evidence) catalog.Label {
		return catalog.Label{Value: value, Source: source, Evidence: ev}
	}
	ev := func(source catalog.EvidenceSource, field, raw string, n int) catalog.Evidence {
		return catalog.Evidence{Source: source, Field: field, Raw: raw, At: at(n)}
	}

	extraProtected, _ := json.Marshal(map[string]any{
		"campaign": "winter-launch",
		"owner":    "brand-team",
		"tags":     []string{"q4", "hero-slot"},
	})

	return []fixtureRecord{
		{
			id: "ad-001", name: "spring-sale", mediaFile: "spring-sale-en.mp4",
			mediaBody: pad("AD001 spring sale master", 4096),
			createdAt: at(1),
			labels: map[string]catalog.Label{
				catalog.FieldLanguage: label(catalog.SourceFilesystem, "en-US",
					ev(catalog.SourceBitstream, "language", "eng", 1),
					ev(catalog.SourceContainer, "language", "en_US", 1)),
				catalog.FieldDuration: label(catalog.SourceFilesystem, "00:30",
					ev(catalog.SourceBitstream, "duration", "30.0", 1),
					ev(catalog.SourceContainer, "duration", "30s", 1)),
			},
			extra: map[string]json.RawMessage{"campaign_meta": extraProtected},
		},
		{
			// Same display name as ad-003, different language variant.
			id: "ad-002", name: "holiday-cheer", mediaFile: "holiday-cheer-en.mp4",
			mediaBody: pad("AD002 holiday cheer english cut", 4096),
			createdAt: at(2),
			labels: map[string]catalog.Label{
				catalog.FieldLanguage: label(catalog.SourceFilesystem, "english",
					ev(catalog.SourceBitstream, "language", "en", 2)),
				catalog.FieldDuration: label(catalog.SourceFilesystem, "15",
					ev(catalog.SourceBitstream, "duration", "15.000", 2),
					ev(catalog.SourceContainer, "duration", "00:15", 2)),
			},
		},
		{
			// Same display name, Japanese variant: must be targeted by id.
			id: "ad-003", name: "holiday-cheer", mediaFile: "holiday-cheer-ja.mp4",
			mediaBody: pad("AD003 holiday cheer japanese cut", 4096),
			createdAt: at(3),
			labels: map[string]catalog.Label{
				catalog.FieldLanguage: label(catalog.SourceFilesystem, "en",
					ev(catalog.SourceBitstream, "language", "jpn", 3),
					ev(catalog.SourceContainer, "language", "ja", 3)),
				catalog.FieldDuration: label(catalog.SourceFilesystem, "00:15",
					ev(catalog.SourceBitstream, "duration", "15.0", 3)),
			},
		},
		{
			// Read-only sidecar: valid group must be held back cleanly.
			id: "ad-004", name: "brand-anthem", mediaFile: "brand-anthem.mp4",
			mediaBody: pad("AD004 brand anthem long form", 8192),
			createdAt: at(4), readOnly: true,
			labels: map[string]catalog.Label{
				catalog.FieldLanguage: label(catalog.SourceFilesystem, "fr",
					ev(catalog.SourceBitstream, "language", "fra", 4)),
				catalog.FieldDuration: label(catalog.SourceFilesystem, "60",
					ev(catalog.SourceBitstream, "duration", "60.0", 4),
					ev(catalog.SourceContainer, "duration", "1m0s", 4)),
			},
		},
		{
			// Conflicting evidence: duration sources disagree -> manual hold.
			id: "ad-005", name: "product-demo", mediaFile: "product-demo.mp4",
			mediaBody: pad("AD005 product demo walkthrough", 4096),
			createdAt: at(5),
			labels: map[string]catalog.Label{
				catalog.FieldLanguage: label(catalog.SourceFilesystem, "de",
					ev(catalog.SourceBitstream, "language", "deu", 5)),
				catalog.FieldDuration: label(catalog.SourceFilesystem, "45",
					ev(catalog.SourceBitstream, "duration", "45.0", 5),
					ev(catalog.SourceContainer, "duration", "47.0", 5)),
			},
		},
		{
			// Healthy second apply target with Go-duration notation.
			id: "ad-006", name: "local-offer", mediaFile: "local-offer-es.mp4",
			mediaBody: pad("AD006 local offer spanish cut", 2048),
			createdAt: at(6),
			labels: map[string]catalog.Label{
				catalog.FieldLanguage: label(catalog.SourceFilesystem, "spanish",
					ev(catalog.SourceBitstream, "language", "spa", 6),
					ev(catalog.SourceSidecar, "language", "es", 6)),
				catalog.FieldDuration: label(catalog.SourceFilesystem, "20s",
					ev(catalog.SourceBitstream, "duration", "20s", 6)),
			},
		},
	}
}

func writeBatches(batchDir string) error {
	good := catalog.CorrectionFile{
		GeneratedAt: time.Date(2026, 11, 25, 1, 0, 0, 0, time.UTC),
		Corrections: []catalog.Correction{
			{GroupID: "g1", MediaID: "ad-001", Field: "language", Value: "eng", Reason: "normalize to ISO 639-1"},
			{GroupID: "g1", MediaID: "ad-001", Field: "duration", Value: "30s"},
			{GroupID: "g2", MediaID: "ad-002", Field: "language", Value: "en"},
			{GroupID: "g2", MediaID: "ad-002", Field: "duration", Value: "00:15"},
			{GroupID: "g3", MediaID: "ad-003", Field: "language", Value: "ja"},
			{GroupID: "g3", MediaID: "ad-003", Field: "duration", Value: "15"},
			{GroupID: "g4", MediaID: "ad-006", Field: "language", Value: "es-ES"},
			{GroupID: "g4", MediaID: "ad-006", Field: "duration", Value: "20.0"},
		},
	}
	if err := writeBatch(filepath.Join(batchDir, "good.json"), good); err != nil {
		return err
	}

	// Bad batch: read-only target, an evidence conflict, a malformed language,
	// a value that contradicts the authority chain, and a missing field.
	bad := catalog.CorrectionFile{
		GeneratedAt: time.Date(2026, 11, 25, 2, 0, 0, 0, time.UTC),
		Corrections: []catalog.Correction{
			{GroupID: "b1", MediaID: "ad-004", Field: "language", Value: "fr"},
			{GroupID: "b1", MediaID: "ad-004", Field: "duration", Value: "60s"},
			{GroupID: "b2", MediaID: "ad-005", Field: "language", Value: "de"},
			{GroupID: "b2", MediaID: "ad-005", Field: "duration", Value: "45s"},
			{GroupID: "b3", MediaID: "ad-001", Field: "language", Value: "!!!"},
			{GroupID: "b3", MediaID: "ad-001", Field: "duration", Value: "30s"},
			{GroupID: "b4", MediaID: "ad-006", Field: "language", Value: "es"},
			{GroupID: "b4", MediaID: "ad-006", Field: "duration", Value: "99s"},
		},
	}
	if err := writeBatch(filepath.Join(batchDir, "bad.json"), bad); err != nil {
		return err
	}

	// Malformed group shape: only one field, must be rejected before any write.
	malformed := catalog.CorrectionFile{
		GeneratedAt: time.Date(2026, 11, 25, 3, 0, 0, 0, time.UTC),
		Corrections: []catalog.Correction{
			{GroupID: "m1", MediaID: "ad-001", Field: "language", Value: "en"},
		},
	}
	return writeBatch(filepath.Join(batchDir, "malformed-group.json"), malformed)
}

func writeBatch(path string, batch catalog.CorrectionFile) error {
	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func recordSummaries(records []fixtureRecord) []map[string]string {
	out := make([]map[string]string, 0, len(records))
	for _, rec := range records {
		out = append(out, map[string]string{
			"media_id": rec.id, "name": rec.name, "media": rec.mediaFile,
		})
	}
	return out
}

func readOnlyIDs(records []fixtureRecord) []string {
	var ids []string
	for _, rec := range records {
		if rec.readOnly {
			ids = append(ids, rec.id)
		}
	}
	return ids
}

func pad(seed string, size int) []byte {
	body := make([]byte, size)
	for i := range body {
		body[i] = seed[i%len(seed)]
	}
	return body
}
