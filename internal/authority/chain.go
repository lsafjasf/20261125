package authority

import (
	"fmt"
	"sort"

	"adlabel/internal/catalog"
)

// Decision outcomes for one proposed correction.
const (
	// DecisionAccepted means the strongest admissible evidence supports the
	// proposed canonical value.
	DecisionAccepted = "accepted"
	// DecisionRejected means admissible evidence contradicts the proposal or
	// the value cannot be normalized. The group containing it must not apply.
	DecisionRejected = "rejected"
	// DecisionManual means evidence is missing or internally conflicting, so
	// a human must rule; the group is held back rather than guessed.
	DecisionManual = "manual"
)

// Machine-readable rejection/manual reasons.
const (
	ReasonNotMutable          = "field_not_mutable"
	ReasonInvalidValue        = "invalid_value"
	ReasonNoAdmissible        = "no_admissible_evidence"
	ReasonConflict            = "authority_conflict"
	ReasonIntraSourceConflict = "intra_source_conflict"
	ReasonMismatch            = "evidence_disagrees"
)

// trustRank orders evidence sources: lower number is stronger. The file
// system is the weakest and is never admissible for automatic decisions.
var trustRank = map[catalog.EvidenceSource]int{
	catalog.SourceBitstream:  0,
	catalog.SourceContainer:  1,
	catalog.SourceSidecar:    2,
	catalog.SourceFilesystem: 3,
}

// admissible reports whether a source may author an automatic decision.
func admissible(source catalog.EvidenceSource) bool {
	return source != catalog.SourceFilesystem
}

// FieldObservation is one canonicalized piece of evidence for one field.
type FieldObservation struct {
	Source    catalog.EvidenceSource
	Raw       string
	Canonical string
}

// Verdict explains what the authority chain decided for one correction.
type Verdict struct {
	MediaID        string
	GroupID        string
	Field          string
	Proposed       string
	Canonical      string
	Decision       string
	Reason         string
	WinningSource  catalog.EvidenceSource
	AuthorityValue string
	Detail         string
}

// Options tunes chain evaluation.
type Options struct {
	// DurationEpsilon is the max absolute second difference for two duration
	// observations to count as equal.
	DurationEpsilon float64
}

// DefaultOptions returns the conservative defaults.
func DefaultOptions() Options {
	return Options{DurationEpsilon: 0.01}
}

// canonicalize normalizes one evidence observation for a field.
func canonicalize(field, raw string) (string, float64, error) {
	switch field {
	case catalog.FieldLanguage:
		canonical, err := NormalizeLanguage(raw)
		return canonical, 0, err
	case catalog.FieldDuration:
		seconds, err := ParseDuration(raw)
		if err != nil {
			return "", 0, err
		}
		return CanonicalDuration(seconds), seconds, nil
	default:
		return "", 0, fmt.Errorf("%s: %s", ReasonNotMutable, field)
	}
}

// Evaluate runs the authority chain for a single correction against all
// evidence currently attached to the record. It never mutates anything.
func Evaluate(corr catalog.Correction, record *catalog.MediaRecord, opts Options) Verdict {
	v := Verdict{
		MediaID:  corr.MediaID,
		GroupID:  corr.GroupID,
		Field:    corr.Field,
		Proposed: corr.Value,
		Decision: DecisionRejected,
	}
	if opts.DurationEpsilon <= 0 {
		opts = DefaultOptions()
	}
	if !catalog.MutableFields[corr.Field] {
		v.Reason = ReasonNotMutable
		v.Detail = fmt.Sprintf("field %q is not in the mutable allow-list", corr.Field)
		return v
	}

	proposedCanonical, proposedSeconds, err := canonicalize(corr.Field, corr.Value)
	if err != nil {
		v.Reason = ReasonInvalidValue
		v.Detail = err.Error()
		return v
	}
	v.Canonical = proposedCanonical

	observations := collectObservations(corr.Field, record)
	if len(observations) == 0 {
		v.Decision = DecisionManual
		v.Reason = ReasonNoAdmissible
		v.Detail = "record carries no evidence for this field"
		return v
	}

	// Group evidence by source so an intra-source conflict can be detected.
	bySource := map[catalog.EvidenceSource][]FieldObservation{}
	for _, obs := range observations {
		bySource[obs.Source] = append(bySource[obs.Source], obs)
	}
	sources := make([]catalog.EvidenceSource, 0, len(bySource))
	for source := range bySource {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool {
		return trustRank[sources[i]] < trustRank[sources[j]]
	})

	winning := sources[0]
	winningObs := bySource[winning]
	canonicalValues := map[string]bool{}
	for _, obs := range winningObs {
		canonicalValues[obs.Canonical] = true
	}
	if len(canonicalValues) > 1 {
		v.Decision = DecisionManual
		v.Reason = ReasonIntraSourceConflict
		v.WinningSource = winning
		v.Detail = fmt.Sprintf("source %q disagrees with itself", winning)
		return v
	}

	authorityCanonical := winningObs[0].Canonical
	v.WinningSource = winning
	v.AuthorityValue = authorityCanonical

	matches := authorityCanonical == proposedCanonical
	if corr.Field == catalog.FieldDuration {
		authoritySeconds, err := ParseDuration(authorityCanonical)
		if err == nil {
			matches = DurationEqual(authoritySeconds, proposedSeconds, opts.DurationEpsilon)
		}
	}
	if !matches {
		v.Reason = ReasonMismatch
		v.Detail = fmt.Sprintf("proposed %q != authority(%s) %q",
			proposedCanonical, winning, authorityCanonical)
		return v
	}

	// Lower-ranked admissible sources that disagree surface as a conflict so
	// the discrepancy stays auditable, even though the strongest source wins.
	for _, source := range sources[1:] {
		for _, obs := range bySource[source] {
			same := obs.Canonical == authorityCanonical
			if corr.Field == catalog.FieldDuration {
				obsSeconds, _ := ParseDuration(obs.Canonical)
				authSeconds, _ := ParseDuration(authorityCanonical)
				same = DurationEqual(obsSeconds, authSeconds, opts.DurationEpsilon)
			}
			if !same {
				v.Decision = DecisionManual
				v.Reason = ReasonConflict
				v.Detail = fmt.Sprintf("source %q=%q conflicts with %s=%q",
					source, obs.Canonical, winning, authorityCanonical)
				return v
			}
		}
	}

	v.Decision = DecisionAccepted
	return v
}

// collectObservations gathers admissible, parseable evidence for field from
// both the current label and its full evidence trail.
func collectObservations(field string, record *catalog.MediaRecord) []FieldObservation {
	seen := map[string]bool{}
	var out []FieldObservation
	add := func(source catalog.EvidenceSource, raw string) {
		if !admissible(source) || raw == "" {
			return
		}
		canonical, _, err := canonicalize(field, raw)
		if err != nil {
			return
		}
		key := string(source) + "|" + canonical
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, FieldObservation{Source: source, Raw: raw, Canonical: canonical})
	}

	if label, ok := record.Labels[field]; ok {
		add(label.Source, label.Value)
		for _, ev := range label.Evidence {
			if ev.Field == field {
				add(ev.Source, ev.Raw)
			}
		}
	}
	return out
}
