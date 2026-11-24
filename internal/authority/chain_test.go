package authority

import (
	"testing"

	"adlabel/internal/catalog"
)

func ev(source catalog.EvidenceSource, field, raw string) catalog.Evidence {
	return catalog.Evidence{Source: source, Field: field, Raw: raw}
}

func recordWith(evidence map[string][]catalog.Evidence) *catalog.MediaRecord {
	rec := &catalog.MediaRecord{MediaID: "x", Labels: map[string]catalog.Label{}}
	for field, items := range evidence {
		label := catalog.Label{Source: catalog.SourceFilesystem, Value: "stale", Evidence: items}
		rec.Labels[field] = label
	}
	return rec
}

func TestNormalizeLanguage(t *testing.T) {
	cases := map[string]string{
		"eng": "en", "en-US": "en", "en_US": "en", "english": "en",
		"jpn": "ja", "cmn": "zh", "chi": "zh", "fr": "fr", "!!": "",
	}
	for raw, want := range cases {
		got, err := NormalizeLanguage(raw)
		if want == "" {
			if err == nil {
				t.Fatalf("expected error for %q", raw)
			}
			continue
		}
		if err != nil || got != want {
			t.Fatalf("NormalizeLanguage(%q)=%q,%v want %q", raw, got, err, want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]float64{
		"30": 30, "30s": 30, "1m0s": 60, "00:15": 15,
		"01:02:03": 3723, "20.5": 20.5, "1:30.5": 90.5,
	}
	for raw, want := range cases {
		got, err := ParseDuration(raw)
		if err != nil || got != want {
			t.Fatalf("ParseDuration(%q)=%v,%v want %v", raw, got, err, want)
		}
	}
	if _, err := ParseDuration("nope"); err == nil {
		t.Fatal("expected error for unparseable duration")
	}
}

func TestEvaluateAcceptsStrongestSource(t *testing.T) {
	rec := recordWith(map[string][]catalog.Evidence{
		catalog.FieldLanguage: {
			ev(catalog.SourceBitstream, "language", "jpn"),
			ev(catalog.SourceContainer, "language", "ja"),
		},
	})
	corr := catalog.Correction{GroupID: "g", MediaID: "x", Field: "language", Value: "ja"}
	v := Evaluate(corr, rec, DefaultOptions())
	if v.Decision != DecisionAccepted || v.WinningSource != catalog.SourceBitstream {
		t.Fatalf("got %+v", v)
	}
}

func TestEvaluateRejectsFilesystemOnly(t *testing.T) {
	rec := &catalog.MediaRecord{MediaID: "x", Labels: map[string]catalog.Label{
		catalog.FieldLanguage: {Value: "en", Source: catalog.SourceFilesystem},
	}}
	corr := catalog.Correction{GroupID: "g", MediaID: "x", Field: "language", Value: "en"}
	v := Evaluate(corr, rec, DefaultOptions())
	if v.Decision != DecisionManual || v.Reason != ReasonNoAdmissible {
		t.Fatalf("filesystem-only must not authorize, got %+v", v)
	}
}

func TestEvaluateConflictRoutedToManual(t *testing.T) {
	rec := recordWith(map[string][]catalog.Evidence{
		catalog.FieldDuration: {
			ev(catalog.SourceBitstream, "duration", "45.0"),
			ev(catalog.SourceContainer, "duration", "47.0"),
		},
	})
	corr := catalog.Correction{GroupID: "g", MediaID: "x", Field: "duration", Value: "45s"}
	v := Evaluate(corr, rec, DefaultOptions())
	if v.Decision != DecisionManual || v.Reason != ReasonConflict {
		t.Fatalf("expected conflict manual hold, got %+v", v)
	}
}

func TestEvaluateRejectsContradiction(t *testing.T) {
	rec := recordWith(map[string][]catalog.Evidence{
		catalog.FieldDuration: {ev(catalog.SourceBitstream, "duration", "20s")},
	})
	corr := catalog.Correction{GroupID: "g", MediaID: "x", Field: "duration", Value: "99s"}
	v := Evaluate(corr, rec, DefaultOptions())
	if v.Decision != DecisionRejected || v.Reason != ReasonMismatch {
		t.Fatalf("expected mismatch rejection, got %+v", v)
	}
}

func TestEvaluateRejectsImmutableField(t *testing.T) {
	rec := &catalog.MediaRecord{MediaID: "x", Labels: map[string]catalog.Label{}}
	corr := catalog.Correction{GroupID: "g", MediaID: "x", Field: "media_hash", Value: "deadbeef"}
	v := Evaluate(corr, rec, DefaultOptions())
	if v.Decision != DecisionRejected || v.Reason != ReasonNotMutable {
		t.Fatalf("protected field must be rejected, got %+v", v)
	}
}
