package auditor

// Provider is implemented by each concrete evidence collector (a directory
// walker, a container parser, a bitstream decoder, a sidecar reader).
// Collectors return only observations; the reconciliation policy lives in
// this package so every collector is judged by the same rules.
type Provider interface {
	Source() Source
	Collect(mediaID string) ([]Observation, error)
}

// ReviewWithProviders runs every provider, records per-provider failures
// without aborting, and reviews the combined observations.
func ReviewWithProviders(mediaID string, providers []Provider, policy Policy) Report {
	var observations []Observation
	var sourceErrors []SourceError

	for _, provider := range providers {
		collected, err := provider.Collect(mediaID)
		if err != nil {
			sourceErrors = append(sourceErrors, SourceError{
				Source: provider.Source(),
				Err:    err.Error(),
			})
			continue
		}
		observations = append(observations, collected...)
	}

	report := Review(mediaID, observations, policy)
	report.SourceErrors = sourceErrors
	return report
}
