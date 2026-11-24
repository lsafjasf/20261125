package auditor

import "math"

func buildProvenance(entries []normalized, policy Policy) []SourceValue {
	out := make([]SourceValue, 0, len(entries))
	for _, entry := range entries {
		value := SourceValue{
			Source:   entry.obs.Source,
			RawValue: entry.obs.RawValue,
			Detail:   entry.obs.Detail,
			Present:  entry.err == nil,
		}
		if entry.err == nil {
			value.Canonical = entry.canonical
			value.Admitted = policy.admissible(entry.obs.Source)
			if entry.obs.Field == FieldDuration {
				value.NormalizedSeconds = entry.seconds
			}
		}
		out = append(out, value)
	}
	return out
}

func buildConflicts(field Field, entries []normalized, policy Policy) []Conflict {
	valid := make([]normalized, 0, len(entries))
	for _, entry := range entries {
		if entry.err == nil {
			valid = append(valid, entry)
		}
	}

	var conflicts []Conflict
	for i := 0; i < len(valid); i++ {
		for j := i + 1; j < len(valid); j++ {
			a, b := valid[i], valid[j]
			if equivalent(field, a, b, policy) {
				continue
			}
			conflicts = append(conflicts, Conflict{
				Field:              field,
				SourceA:            a.obs.Source,
				ValueA:             a.canonical,
				SourceB:            b.obs.Source,
				ValueB:             b.canonical,
				InvolvesFilesystem: a.obs.Source == SourceFilesystem || b.obs.Source == SourceFilesystem,
				IntraSource:        a.obs.Source == b.obs.Source,
			})
		}
	}
	return conflicts
}

func equivalent(field Field, a, b normalized, policy Policy) bool {
	if field == FieldDuration {
		return math.Abs(a.seconds-b.seconds) <= policy.DurationEpsilon
	}
	return a.canonical == b.canonical
}

// groupAgreeing clusters admissible entries whose values are mutually
// equivalent. Each returned group is internally consistent; more than one
// group means an unresolvable conflict between sources.
func groupAgreeing(field Field, entries []normalized, policy Policy) [][]normalized {
	var groups [][]normalized
entryLoop:
	for _, entry := range entries {
		for groupIndex, group := range groups {
			if equivalent(field, entry, group[0], policy) {
				groups[groupIndex] = append(group, entry)
				continue entryLoop
			}
		}
		groups = append(groups, []normalized{entry})
	}
	return groups
}

func distinctSources(entries []normalized) []Source {
	seen := map[Source]bool{}
	var sources []Source
	for _, entry := range entries {
		if !seen[entry.obs.Source] {
			seen[entry.obs.Source] = true
			sources = append(sources, entry.obs.Source)
		}
	}
	sortSources(sources)
	return sources
}

func sortSources(sources []Source) {
	for i := 1; i < len(sources); i++ {
		for j := i; j > 0 && trustRank[sources[j]] < trustRank[sources[j-1]]; j-- {
			sources[j], sources[j-1] = sources[j-1], sources[j]
		}
	}
}

func hasIntraSourceConflict(conflicts []Conflict) bool {
	for _, conflict := range conflicts {
		if conflict.IntraSource {
			return true
		}
	}
	return false
}
