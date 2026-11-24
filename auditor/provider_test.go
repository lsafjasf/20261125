package auditor

import (
	"errors"
	"testing"
)

type stubProvider struct {
	source       Source
	observations []Observation
	err          error
}

func (s stubProvider) Source() Source { return s.source }

func (s stubProvider) Collect(_ string) ([]Observation, error) {
	return s.observations, s.err
}

func TestReviewWithProvidersContinuesAfterProviderError(t *testing.T) {
	providers := []Provider{
		stubProvider{
			source: SourceFilesystem,
			err:    errors.New("permission denied"),
		},
		stubProvider{
			source: SourceBitstream,
			observations: []Observation{
				obs(SourceBitstream, FieldLanguage, "en"),
			},
		},
		stubProvider{
			source: SourceContainer,
			observations: []Observation{
				obs(SourceContainer, FieldLanguage, "eng"),
			},
		},
	}

	report := ReviewWithProviders("m1", providers, DefaultPolicy())

	if len(report.SourceErrors) != 1 || report.SourceErrors[0].Source != SourceFilesystem {
		t.Fatalf("source errors = %+v", report.SourceErrors)
	}
	verdict := verdictFor(t, report, FieldLanguage)
	if verdict.Status != StatusAutomatic || verdict.Value != "en" {
		t.Fatalf("verdict = %+v, want automatic en", verdict)
	}
}
