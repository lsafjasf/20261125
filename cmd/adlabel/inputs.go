package main

import (
	"encoding/json"
	"os"

	"adlabel/internal/authority"
	"adlabel/internal/catalog"
)

// DefaultAuthorityOptions exposes the conservative chain defaults to the CLI.
func repairDefaultOptions() authority.Options { return authority.DefaultOptions() }

func loadInputs(libraryDir, correctionsPath string) (*catalog.Library, []catalog.Correction, error) {
	lib, err := catalog.Load(libraryDir)
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(correctionsPath)
	if err != nil {
		return nil, nil, err
	}
	var batch catalog.CorrectionFile
	if err := json.Unmarshal(data, &batch); err != nil {
		return nil, nil, err
	}
	return lib, batch.Corrections, nil
}
