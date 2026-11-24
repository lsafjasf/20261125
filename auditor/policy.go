package auditor

// Policy controls how observations are reconciled.
type Policy struct {
	// AdmissibleSources are the only sources allowed to support an automatic
	// conclusion. The file system is excluded by default and can never be
	// added implicitly: callers must opt in explicitly.
	AdmissibleSources []Source
	// DurationEpsilon is the maximum absolute difference in seconds at which
	// two duration observations count as equal.
	DurationEpsilon float64
	// RequireCorroboration forces manual review when only one admissible
	// source supplies a value. By default a lone trustworthy source produces
	// a low-confidence automatic conclusion.
	RequireCorroboration bool
}

// DefaultPolicy returns the conservative default policy:
// container, bitstream and sidecar are admissible; the file system is not.
func DefaultPolicy() Policy {
	return Policy{
		AdmissibleSources: []Source{
			SourceBitstream,
			SourceContainer,
			SourceSidecar,
		},
		DurationEpsilon: 0.01,
	}
}

func (p Policy) admissible(source Source) bool {
	for _, candidate := range p.AdmissibleSources {
		if candidate == source {
			return true
		}
	}
	return false
}

// trustRank orders sources for deterministic output. Lower wins.
var trustRank = map[Source]int{
	SourceBitstream:  0,
	SourceContainer:  1,
	SourceSidecar:    2,
	SourceFilesystem: 3,
}
