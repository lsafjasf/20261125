package auditor

import (
	"fmt"
	"sort"
	"time"
)

// Machine-readable manual/undetermined reasons.
const (
	ReasonConflictingEvidence       = "conflicting_evidence"
	ReasonIntraSourceConflict       = "intra_source_conflict"
	ReasonNoAdmissibleEvidence      = "no_admissible_evidence"
	ReasonSingleSourceCorroboration = "single_source_requires_corroboration"
)

type normalized struct {
	obs       Observation
	canonical string
	seconds   float64
	err       error
}

// Review reconciles all observations for one media item and returns a report
// containing one verdict per field. Every observation is reflected in the
// provenance; every disagreement is reflected as a conflict path.
func Review(mediaID string, observations []Observation, policy Policy) Report {
	if len(policy.AdmissibleSources) == 0 || policy.DurationEpsilon <= 0 {
		policy = DefaultPolicy()
	}

	byField := map[Field][]normalized{}
	for _, obs := range observations {
		entry := normalized{obs: obs}
		switch obs.Field {
		case FieldLanguage:
			entry.canonical, entry.err = NormalizeLanguage(obs.RawValue)
		case FieldDuration:
			entry.seconds, entry.err = ParseDuration(obs.RawValue)
			if entry.err == nil {
				entry.canonical = CanonicalDuration(entry.seconds)
			}
		default:
			entry.err = fmt.Errorf("unknown field %q", obs.Field)
		}
		byField[obs.Field] = append(byField[obs.Field], entry)
	}

	report := Report{
		MediaID:     mediaID,
		GeneratedAt: time.Now().UTC(),
	}
	for _, field := range []Field{FieldLanguage, FieldDuration} {
		report.Verdicts = append(report.Verdicts, reviewField(field, byField[field], policy))
	}
	return report
}

func reviewField(field Field, entries []normalized, policy Policy) FieldVerdict {
	sort.SliceStable(entries, func(i, j int) bool {
		if trustRank[entries[i].obs.Source] != trustRank[entries[j].obs.Source] {
			return trustRank[entries[i].obs.Source] < trustRank[entries[j].obs.Source]
		}
		if entries[i].obs.Source != entries[j].obs.Source {
			return entries[i].obs.Source < entries[j].obs.Source
		}
		return entries[i].obs.RawValue < entries[j].obs.RawValue
	})

	verdict := FieldVerdict{
		Field:      field,
		Provenance: []SourceValue{},
		Conflicts:  []Conflict{},
	}
	verdict.Provenance = buildProvenance(entries, policy)
	verdict.Conflicts = buildConflicts(field, entries, policy)

	valid := make([]normalized, 0, len(entries))
	for _, entry := range entries {
		if entry.err == nil && policy.admissible(entry.obs.Source) {
			valid = append(valid, entry)
		}
	}

	if hasIntraSourceConflict(verdict.Conflicts) {
		verdict.Status = StatusManual
		verdict.Reason = ReasonIntraSourceConflict
		return verdict
	}
	if len(valid) == 0 {
		// No trustworthy evidence: the verdict stays empty on purpose.
		// The file system is never promoted into a fallback value.
		verdict.Status = StatusUndetermined
		verdict.Reason = ReasonNoAdmissibleEvidence
		return verdict
	}

	groups := groupAgreeing(field, valid, policy)
	if len(groups) > 1 {
		verdict.Status = StatusManual
		verdict.Reason = ReasonConflictingEvidence
		return verdict
	}

	group := groups[0]
	sources := distinctSources(group)
	if policy.RequireCorroboration && len(sources) == 1 {
		verdict.Status = StatusManual
		verdict.Reason = ReasonSingleSourceCorroboration
		return verdict
	}

	verdict.Status = StatusAutomatic
	verdict.Value = group[0].canonical
	verdict.ValueSources = sources
	if len(sources) == 1 {
		verdict.Confidence = ConfidenceLow
	} else {
		verdict.Confidence = ConfidenceHigh
	}
	return verdict
}
