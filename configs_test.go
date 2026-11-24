package main

import (
	"path/filepath"
	"testing"
)

func TestShippedConfigs(t *testing.T) {
	cases := []struct {
		file          string
		wantValid     bool
		wantCycles    int
		wantDupHandle string
		wantForks     int
		wantMerges    int
		wantUnused    int
	}{
		{"color_graph.json", true, 0, "", 1, 1, 3},
		{"diamond.json", true, 0, "", 1, 1, 0},
		{"cycle.json", false, 1, "", 1, 1, 0},
		{"duplicate_outputs.json", false, 0, "plate_a", 0, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			cfg, err := LoadConfig(filepath.Join("configs", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			r := BuildReport(cfg)
			if r.Valid != tc.wantValid {
				t.Fatalf("valid=%v want %v; diags=%+v", r.Valid, tc.wantValid, r.Diagnostics)
			}
			if len(r.Cycles) != tc.wantCycles {
				t.Fatalf("cycles=%d want %d (%+v)", len(r.Cycles), tc.wantCycles, r.Cycles)
			}
			if tc.wantDupHandle != "" {
				if len(r.DuplicateOutputs) != 1 || r.DuplicateOutputs[0].Handle != tc.wantDupHandle {
					t.Fatalf("duplicates=%+v want handle %s", r.DuplicateOutputs, tc.wantDupHandle)
				}
			}
			if len(r.Forks) != tc.wantForks {
				t.Fatalf("forks=%d want %d: %+v", len(r.Forks), tc.wantForks, r.Forks)
			}
			if len(r.Merges) != tc.wantMerges {
				t.Fatalf("merges=%d want %d: %+v", len(r.Merges), tc.wantMerges, r.Merges)
			}
			if len(r.UnusedNodes) != tc.wantUnused {
				t.Fatalf("unused=%d want %d: %+v", len(r.UnusedNodes), tc.wantUnused, r.UnusedNodes)
			}
		})
	}
}
