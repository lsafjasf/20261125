package repair

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"adlabel/internal/authority"
	"adlabel/internal/catalog"
)

type testSidecar struct {
	MediaID   string                   `json:"media_id"`
	Name      string                   `json:"name"`
	MediaPath string                   `json:"media_path"`
	MediaHash string                   `json:"media_hash"`
	Labels    map[string]catalog.Label `json:"labels"`
	Protected map[string]any           `json:"protected"`
}

func writeMiniLibrary(t *testing.T, root string, readOnly map[string]bool) map[string]string {
	t.Helper()
	mediaDir := filepath.Join(root, "media")
	metaDir := filepath.Join(root, "metadata")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}

	body := func(seed string) []byte {
		b := make([]byte, 256)
		for i := range b {
			b[i] = seed[i%len(seed)]
		}
		return b
	}
	lbl := func(source catalog.EvidenceSource, value string, ev ...catalog.Evidence) catalog.Label {
		return catalog.Label{Value: value, Source: source, Evidence: ev}
	}
	e := func(source catalog.EvidenceSource, field, raw string) catalog.Evidence {
		return catalog.Evidence{Source: source, Field: field, Raw: raw}
	}

	specs := []struct {
		id, name, file, seed string
		labels               map[string]catalog.Label
	}{
		{"m1", "same-name", "m1.bin", "MEDIA-ONE", map[string]catalog.Label{
			catalog.FieldLanguage: lbl(catalog.SourceFilesystem, "en", e(catalog.SourceBitstream, "language", "eng")),
			catalog.FieldDuration: lbl(catalog.SourceFilesystem, "30", e(catalog.SourceBitstream, "duration", "30s")),
		}},
		{"m2", "same-name", "m2.bin", "MEDIA-TWO", map[string]catalog.Label{
			catalog.FieldLanguage: lbl(catalog.SourceFilesystem, "en", e(catalog.SourceBitstream, "language", "jpn")),
			catalog.FieldDuration: lbl(catalog.SourceFilesystem, "15", e(catalog.SourceBitstream, "duration", "15s")),
		}},
	}
	hashes := map[string]string{}
	for _, spec := range specs {
		mediaBytes := body(spec.seed)
		mediaPath := filepath.Join(mediaDir, spec.file)
		if err := os.WriteFile(mediaPath, mediaBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		hashes[spec.id] = catalog.HashBytes(mediaBytes)
		doc := testSidecar{
			MediaID: spec.id, Name: spec.name,
			MediaPath: filepath.ToSlash(filepath.Join("media", spec.file)),
			MediaHash: hashes[spec.id], Labels: spec.labels,
			Protected: map[string]any{"campaign": "keep-me", "n": 42},
		}
		data, _ := json.MarshalIndent(doc, "", "  ")
		sidecar := filepath.Join(metaDir, spec.id+".json")
		mode := os.FileMode(0o644)
		if readOnly[spec.id] {
			mode = 0o444
		}
		if err := os.WriteFile(sidecar, append(data, '\n'), mode); err != nil {
			t.Fatal(err)
		}
	}
	return hashes
}

func goodGroup(id string) []catalog.Correction {
	lang := "en"
	if id == "m2" {
		lang = "ja"
	}
	dur := "30s"
	if id == "m2" {
		dur = "15s"
	}
	return []catalog.Correction{
		{GroupID: id + "-g", MediaID: id, Field: catalog.FieldLanguage, Value: lang},
		{GroupID: id + "-g", MediaID: id, Field: catalog.FieldDuration, Value: dur},
	}
}

func TestApplyKeepsHashAndProtectedFields(t *testing.T) {
	root := t.TempDir()
	hashes := writeMiniLibrary(t, root, nil)
	lib, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	eng := NewEngine(lib, authority.DefaultOptions(), false, false)
	report, err := eng.Run(goodGroup("m1"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Applied != 1 || report.Groups[0].Status != StatusApplied {
		t.Fatalf("expected one applied group, got %+v", report)
	}

	entry := lib.ByID["m1"]
	gotHash, err := catalog.ReadMediaHash(filepath.Join(root, entry.Record.MediaPath))
	if err != nil {
		t.Fatal(err)
	}
	if gotHash != hashes["m1"] {
		t.Fatal("media hash changed")
	}
	var doc testSidecar
	data, _ := os.ReadFile(entry.Record.SidecarPath())
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Protected["campaign"] != "keep-me" || doc.MediaHash != hashes["m1"] {
		t.Fatalf("protected data lost: %+v", doc.Protected)
	}
	if doc.Labels[catalog.FieldLanguage].Value != "en" ||
		doc.Labels[catalog.FieldDuration].Value != "30s" {
		t.Fatalf("labels not canonicalized: %+v", doc.Labels)
	}
}

func TestSameNameVariantsTargetedByID(t *testing.T) {
	root := t.TempDir()
	writeMiniLibrary(t, root, nil)
	lib, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Names["same-name"]) != 2 {
		t.Fatal("expected two same-name variants")
	}
	eng := NewEngine(lib, authority.DefaultOptions(), false, false)
	if _, err := eng.Run(goodGroup("m2")); err != nil {
		t.Fatal(err)
	}
	m2Rec, _, err := catalog.LoadSidecar(lib.ByID["m2"].Record.SidecarPath())
	if err != nil {
		t.Fatal(err)
	}
	m1Rec, _, err := catalog.LoadSidecar(lib.ByID["m1"].Record.SidecarPath())
	if err != nil {
		t.Fatal(err)
	}
	if m2Rec.Labels[catalog.FieldLanguage].Value != "ja" {
		t.Fatalf("m2 language wrong: %s", m2Rec.Labels[catalog.FieldLanguage].Value)
	}
	// m1 must remain untouched despite sharing the display name.
	if m1Rec.Labels[catalog.FieldLanguage].Value != "en" {
		t.Fatalf("m1 was modified by same-name sibling: %+v", m1Rec.Labels)
	}
}

func TestReadOnlySidecarRejectedAndUntouched(t *testing.T) {
	root := t.TempDir()
	writeMiniLibrary(t, root, map[string]bool{"m1": true})
	before, err := os.ReadFile(filepath.Join(root, "metadata", "m1.json"))
	if err != nil {
		t.Fatal(err)
	}
	lib, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	eng := NewEngine(lib, authority.DefaultOptions(), false, false)
	report, err := eng.Run(goodGroup("m1"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Groups[0].Status != StatusRolledBack {
		t.Fatalf("expected rollback, got %+v", report.Groups[0])
	}
	after, _ := os.ReadFile(filepath.Join(root, "metadata", "m1.json"))
	if string(before) != string(after) {
		t.Fatal("read-only sidecar content changed")
	}
}

func TestAllOrNothingRollsBackEarlierGroups(t *testing.T) {
	root := t.TempDir()
	writeMiniLibrary(t, root, nil)
	lib, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	originalM1, _ := os.ReadFile(lib.ByID["m1"].Record.SidecarPath())

	// m2 duration contradicts its bitstream evidence (15s) to force failure.
	bad := append(goodGroup("m1"),
		catalog.Correction{GroupID: "m2-g", MediaID: "m2", Field: catalog.FieldLanguage, Value: "ja"},
		catalog.Correction{GroupID: "m2-g", MediaID: "m2", Field: catalog.FieldDuration, Value: "99s"},
	)
	eng := NewEngine(lib, authority.DefaultOptions(), true, false)
	report, err := eng.Run(bad)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Aborted || report.Summary.Applied != 0 || report.Summary.RolledBack != 1 {
		t.Fatalf("expected full abort, got %+v", report.Summary)
	}
	for _, g := range report.Groups {
		if g.Status != StatusRolledBack {
			t.Fatalf("group %s not marked rolled back", g.GroupID)
		}
	}
	restoredM1, _ := os.ReadFile(lib.ByID["m1"].Record.SidecarPath())
	if string(originalM1) != string(restoredM1) {
		t.Fatal("earlier applied group was not rolled back")
	}
}

func TestGroupShapeEnforced(t *testing.T) {
	if _, _, err := groupCorrections([]catalog.Correction{
		{GroupID: "g", MediaID: "m1", Field: catalog.FieldLanguage, Value: "en"},
	}); err == nil {
		t.Fatal("single-field group must be rejected")
	}
	if _, _, err := groupCorrections([]catalog.Correction{
		{GroupID: "g", MediaID: "m1", Field: catalog.FieldLanguage, Value: "en"},
		{GroupID: "g", MediaID: "m2", Field: catalog.FieldDuration, Value: "30s"},
	}); err == nil {
		t.Fatal("group spanning media ids must be rejected")
	}
	if _, _, err := groupCorrections(append(goodGroup("m1"),
		catalog.Correction{GroupID: "again", MediaID: "m1", Field: catalog.FieldLanguage, Value: "en"},
		catalog.Correction{GroupID: "again", MediaID: "m1", Field: catalog.FieldDuration, Value: "30s"},
	)); err == nil {
		t.Fatal("same media id in two groups must be rejected")
	}
}
