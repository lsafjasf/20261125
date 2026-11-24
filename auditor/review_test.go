package auditor

import (
	"testing"
	"time"
)

var testTime = time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)

func obs(source Source, field Field, raw string) Observation {
	return Observation{
		MediaID:    "m1",
		Source:     source,
		Field:      field,
		RawValue:   raw,
		ObservedAt: testTime,
		Detail:     string(source) + ":" + string(field),
	}
}

func verdictFor(t *testing.T, report Report, field Field) FieldVerdict {
	for _, verdict := range report.Verdicts {
		if verdict.Field == field {
			return verdict
		}
	}
	t.Fatalf("no verdict for field %s", field)
	return FieldVerdict{}
}

func TestAutomaticHighConfidenceWhenSourcesAgree(t *testing.T) {
	observations := []Observation{
		obs(SourceBitstream, FieldLanguage, "en"),
		obs(SourceContainer, FieldLanguage, "eng"),
		obs(SourceSidecar, FieldLanguage, "en-US"),
		obs(SourceFilesystem, FieldLanguage, "english"),
	}
	report := Review("m1", observations, DefaultPolicy())
	verdict := verdictFor(t, report, FieldLanguage)

	if verdict.Status != StatusAutomatic {
		t.Fatalf("status = %s, want automatic (%s)", verdict.Status, verdict.Reason)
	}
	if verdict.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %q, want high", verdict.Confidence)
	}
	if verdict.Value != "en" {
		t.Fatalf("value = %q, want en", verdict.Value)
	}
	if len(verdict.ValueSources) != 3 {
		t.Fatalf("value sources = %v, want 3 non-filesystem sources", verdict.ValueSources)
	}
	for _, source := range verdict.ValueSources {
		if source == SourceFilesystem {
			t.Fatal("filesystem must never support an automatic value")
		}
	}
	if len(verdict.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", verdict.Conflicts)
	}
}

func TestFilesystemConflictIsRecordedButDoesNotBlock(t *testing.T) {
	observations := []Observation{
		obs(SourceBitstream, FieldDuration, "01:42:03.500"),
		obs(SourceContainer, FieldDuration, "6123.5s"),
		obs(SourceFilesystem, FieldDuration, "3600s"),
	}
	verdict := verdictFor(t, Review("m1", observations, DefaultPolicy()), FieldDuration)

	if verdict.Status != StatusAutomatic {
		t.Fatalf("status = %s, want automatic", verdict.Status)
	}
	if verdict.Value != "6123.5s" {
		t.Fatalf("value = %q, want 6123.5s", verdict.Value)
	}
	if len(verdict.Conflicts) != 2 || !verdict.Conflicts[0].InvolvesFilesystem {
		t.Fatalf("filesystem conflict missing: %+v", verdict.Conflicts)
	}
}

func TestManualWhenAdmissibleSourcesConflict(t *testing.T) {
	observations := []Observation{
		obs(SourceBitstream, FieldLanguage, "en"),
		obs(SourceContainer, FieldLanguage, "ja"),
		obs(SourceSidecar, FieldLanguage, "en"),
	}
	verdict := verdictFor(t, Review("m1", observations, DefaultPolicy()), FieldLanguage)

	if verdict.Status != StatusManual {
		t.Fatalf("status = %s, want manual", verdict.Status)
	}
	if verdict.Value != "" {
		t.Fatalf("manual verdict must not carry a value, got %q", verdict.Value)
	}
	if verdict.Reason != ReasonConflictingEvidence {
		t.Fatalf("reason = %q, want %q", verdict.Reason, ReasonConflictingEvidence)
	}
	if len(verdict.Conflicts) != 2 {
		t.Fatalf("conflicts = %d, want 2", len(verdict.Conflicts))
	}
}

func TestUndeterminedNeverFallsBackToFilesystem(t *testing.T) {
	observations := []Observation{
		obs(SourceFilesystem, FieldLanguage, "fr"),
		obs(SourceFilesystem, FieldDuration, "1200s"),
	}
	report := Review("m1", observations, DefaultPolicy())

	for _, verdict := range report.Verdicts {
		if verdict.Status != StatusUndetermined {
			t.Fatalf("%s status = %s, want undetermined", verdict.Field, verdict.Status)
		}
		if verdict.Value != "" || len(verdict.ValueSources) != 0 {
			t.Fatalf("%s must not adopt a filesystem fallback: %+v", verdict.Field, verdict)
		}
		if verdict.Reason != ReasonNoAdmissibleEvidence {
			t.Fatalf("%s reason = %q", verdict.Field, verdict.Reason)
		}
	}
}

func TestUndeterminedWhenNoEvidenceAtAll(t *testing.T) {
	report := Review("m1", nil, DefaultPolicy())
	for _, verdict := range report.Verdicts {
		if verdict.Status != StatusUndetermined {
			t.Fatalf("%s expected undetermined, got %s", verdict.Field, verdict.Status)
		}
		if len(verdict.Provenance) != 0 {
			t.Fatalf("%s expected empty provenance", verdict.Field)
		}
	}
}

func TestSingleAdmissibleSourceIsLowConfidenceAutomatic(t *testing.T) {
	observations := []Observation{
		obs(SourceBitstream, FieldDuration, "90m"),
	}
	verdict := verdictFor(t, Review("m1", observations, DefaultPolicy()), FieldDuration)

	if verdict.Status != StatusAutomatic || verdict.Confidence != ConfidenceLow {
		t.Fatalf("got status=%s confidence=%s", verdict.Status, verdict.Confidence)
	}
	if verdict.Value != "5400s" {
		t.Fatalf("value = %q, want 5400s", verdict.Value)
	}
}

func TestRequireCorroborationRoutesLoneSourceToManual(t *testing.T) {
	policy := DefaultPolicy()
	policy.RequireCorroboration = true
	observations := []Observation{
		obs(SourceSidecar, FieldLanguage, "de"),
	}
	verdict := verdictFor(t, Review("m1", observations, policy), FieldLanguage)

	if verdict.Status != StatusManual {
		t.Fatalf("status = %s, want manual", verdict.Status)
	}
	if verdict.Reason != ReasonSingleSourceCorroboration {
		t.Fatalf("reason = %q", verdict.Reason)
	}
}

func TestIntraSourceConflictForcesManual(t *testing.T) {
	observations := []Observation{
		obs(SourceSidecar, FieldLanguage, "en"),
		obs(SourceSidecar, FieldLanguage, "fr"),
		obs(SourceBitstream, FieldLanguage, "en"),
	}
	verdict := verdictFor(t, Review("m1", observations, DefaultPolicy()), FieldLanguage)

	if verdict.Status != StatusManual || verdict.Reason != ReasonIntraSourceConflict {
		t.Fatalf("got status=%s reason=%s", verdict.Status, verdict.Reason)
	}
	found := false
	for _, conflict := range verdict.Conflicts {
		if conflict.IntraSource {
			found = true
		}
	}
	if !found {
		t.Fatal("intra-source conflict not reported")
	}
}

func TestDurationEpsilonTolerance(t *testing.T) {
	observations := []Observation{
		obs(SourceBitstream, FieldDuration, "6123.500"),
		obs(SourceContainer, FieldDuration, "6123.505"),
	}
	verdict := verdictFor(t, Review("m1", observations, DefaultPolicy()), FieldDuration)

	if verdict.Status != StatusAutomatic {
		t.Fatalf("within-epsilon values must agree, got %s: %s", verdict.Status, verdict.Reason)
	}
	if len(verdict.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", verdict.Conflicts)
	}
}

func TestInvalidObservationStaysInProvenance(t *testing.T) {
	observations := []Observation{
		obs(SourceBitstream, FieldLanguage, "en"),
		obs(SourceContainer, FieldLanguage, "??"),
	}
	verdict := verdictFor(t, Review("m1", observations, DefaultPolicy()), FieldLanguage)

	if verdict.Status != StatusAutomatic {
		t.Fatalf("unparseable side evidence should not block valid evidence: %s", verdict.Status)
	}
	if len(verdict.Provenance) != 2 {
		t.Fatalf("provenance = %d entries, want 2", len(verdict.Provenance))
	}
	for _, entry := range verdict.Provenance {
		if entry.Source == SourceContainer && entry.Present {
			t.Fatal("invalid observation must be marked absent")
		}
	}
}
