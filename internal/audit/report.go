package audit

import (
	"os"
	"path/filepath"
	"strings"
)

// BuildReport computes aggregate counters for file reports.
func BuildReport(files []FileReport) Report {
	report := Report{
		SchemaVersion: SchemaVersion,
		ReadOnly:      true,
		Inputs:        files,
	}

	for _, file := range files {
		for _, track := range file.Tracks {
			report.Summary.Tracks++
			report.Summary.Edits += len(track.Edits)
			if track.MDHD != nil && file.Movie != nil &&
				track.MDHD.Timescale != 0 && file.Movie.Timescale != 0 &&
				track.MDHD.Timescale != file.Movie.Timescale {
				report.Summary.CrossTimescales++
			}
			if track.AudioStart != nil && (track.AudioStart.NegativeTimelineOrigin || track.AudioStart.SourceMediaTimeUnits < 0) {
				report.Summary.NegativeStarts++
			}
			for _, edit := range track.Edits {
				if edit.Empty {
					report.Summary.EmptyEdits++
				}
				if edit.Dwell {
					report.Summary.DwellEdits++
				}
			}
		}
		for _, finding := range file.Findings {
			switch finding.Severity {
			case "error":
				report.Summary.Errors++
			case "warning":
				report.Summary.Warnings++
			}
		}
	}
	report.Summary.Files = len(files)
	return report
}

// CollectInputs expands directories to supported MOV/MP4-like files.
func CollectInputs(paths []string) ([]string, error) {
	if len(paths) == 0 {
		paths = []string{"testdata/cases"}
	}

	var out []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, path)
			continue
		}

		err = filepath.WalkDir(path, func(candidate string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(candidate)) {
			case ".mov", ".mp4", ".m4v", ".m4a", ".3gp":
				out = append(out, candidate)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// AuditPaths opens every input read-only and builds one report.
func AuditPaths(paths []string) (Report, error) {
	inputPaths, err := CollectInputs(paths)
	if err != nil {
		return Report{}, err
	}

	files := make([]FileReport, 0, len(inputPaths))
	for _, path := range inputPaths {
		fileReport, auditErr := AuditFile(path)
		if auditErr != nil && fileReport.Error == "" {
			fileReport.Error = auditErr.Error()
		}
		files = append(files, fileReport)
	}
	return BuildReport(files), nil
}
