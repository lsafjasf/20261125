package main

import "testing"

func TestCandidateMappingsCompleteForSmallChannelCount(t *testing.T) {
	candidates := candidateMappings(3)
	if len(candidates) != 6 {
		t.Fatalf("candidate count = %d, want 6", len(candidates))
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if err := validatePermutation(candidate, 3); err != nil {
			t.Fatalf("invalid candidate %v: %v", candidate, err)
		}
		seen[formatMapping(candidate)] = true
	}
	if len(seen) != 6 {
		t.Fatalf("unique candidates = %d, want 6", len(seen))
	}
}

func TestCandidateMappingsBoundedForLargeChannelCount(t *testing.T) {
	candidates := candidateMappings(18)
	if len(candidates) != 18 {
		t.Fatalf("candidate count = %d, want 18", len(candidates))
	}
	if err := validatePermutation(candidates[1], 18); err != nil {
		t.Fatalf("first swap candidate invalid: %v", err)
	}
}
