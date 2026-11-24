package mp4diag

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type Manifest struct {
	SchemaVersion int                  `json:"schema_version"`
	Reference     string               `json:"reference"`
	Fixtures      []FixtureExpectation `json:"fixtures"`
}

type FixtureExpectation struct {
	File                    string      `json:"file"`
	Title                   string      `json:"title"`
	DominantLayer           string      `json:"dominant_layer"`
	FirstBadBox             *BoxLocator `json:"first_bad_box,omitempty"`
	MetadataBox             *BoxLocator `json:"metadata_box,omitempty"`
	FaultSample             int         `json:"fault_sample,omitempty"`
	ChangedSamples          []int       `json:"changed_samples,omitempty"`
	AffectedSamples         []int       `json:"affected_samples,omitempty"`
	AffectedDisplayFrames   []int       `json:"affected_display_frames,omitempty"`
	CleanSampleRanges       []Span      `json:"clean_sample_ranges,omitempty"`
	CleanDisplayFrameRanges []Span      `json:"clean_display_frame_ranges,omitempty"`
	ReferencePixelDamage    string      `json:"reference_pixel_damage,omitempty"`
	DecoderVerification     string      `json:"decoder_verification,omitempty"`
}

func LoadManifest(path string) (Manifest, error) {
	var manifest Manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func VerifyManifest(manifestPath string) error {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	baseDir := filepath.Dir(manifestPath)
	reference := manifest.Reference
	if reference != "" && !filepath.IsAbs(reference) {
		reference = filepath.Join(baseDir, reference)
	}

	var failures []string
	for _, expected := range manifest.Fixtures {
		fixturePath := expected.File
		if !filepath.IsAbs(fixturePath) {
			fixturePath = filepath.Join(baseDir, fixturePath)
		}
		report, err := AnalyzeOptions(Options{Path: fixturePath, Reference: reference})
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: analyze: %v", expected.File, err))
			continue
		}
		failures = append(failures, verifyFixtureExpectation(expected, report)...)
	}
	if len(failures) > 0 {
		return fmt.Errorf("manifest verification failed:\n  - %s", strings.Join(failures, "\n  - "))
	}
	return nil
}

func verifyFixtureExpectation(expected FixtureExpectation, report *Report) []string {
	var failures []string
	prefix := expected.File

	if expected.FirstBadBox != nil {
		if report.FirstBadBox == nil {
			failures = append(failures, prefix+": expected first bad box but got none")
		} else {
			if report.FirstBadBox.Path != expected.FirstBadBox.Path ||
				report.FirstBadBox.Type != expected.FirstBadBox.Type ||
				report.FirstBadBox.Offset != expected.FirstBadBox.Offset {
				failures = append(failures, fmt.Sprintf("%s: first bad box = %s@%d/%q, want %s@%d/%q",
					prefix, report.FirstBadBox.Path, report.FirstBadBox.Offset, report.FirstBadBox.Type,
					expected.FirstBadBox.Path, expected.FirstBadBox.Offset, expected.FirstBadBox.Type))
			}
		}
	}

	if expected.MetadataBox != nil {
		finding := findMetadataReferenceFinding(report)
		if finding == nil || finding.Box == nil {
			failures = append(failures, prefix+": missing metadata reference finding")
		} else if finding.Box.Path != expected.MetadataBox.Path ||
			finding.Box.Offset != expected.MetadataBox.Offset ||
			finding.Box.Entry != expected.MetadataBox.Entry {
			failures = append(failures, fmt.Sprintf("%s: metadata box = %s@%d entry=%d, want %s@%d entry=%d",
				prefix, finding.Box.Path, finding.Box.Offset, finding.Box.Entry,
				expected.MetadataBox.Path, expected.MetadataBox.Offset, expected.MetadataBox.Entry))
		} else if finding.PixelDamage != PixelRuledOut {
			failures = append(failures, prefix+": metadata finding must rule out pixel damage")
		}
	}

	if expected.ReferencePixelDamage != "" {
		if report.Reference == nil {
			failures = append(failures, prefix+": reference report is absent")
		} else if report.Reference.PixelDamage != expected.ReferencePixelDamage {
			failures = append(failures, fmt.Sprintf("%s: pixel evidence = %q, want %q",
				prefix, report.Reference.PixelDamage, expected.ReferencePixelDamage))
		}
	}

	checkIntSlice := func(name string, got, want []int) {
		if len(want) > 0 && !reflect.DeepEqual(got, want) {
			failures = append(failures, fmt.Sprintf("%s: %s = %v, want %v", prefix, name, got, want))
		}
	}
	checkSpanSlice := func(name string, got, want []Span) {
		if len(want) > 0 && !reflect.DeepEqual(got, want) {
			failures = append(failures, fmt.Sprintf("%s: %s = %+v, want %+v", prefix, name, got, want))
		}
	}

	if report.Reference != nil {
		checkIntSlice("changed samples", report.Reference.ChangedSamples, expected.ChangedSamples)
	}
	checkIntSlice("affected samples", report.Display.AffectedSamples, expected.AffectedSamples)
	checkIntSlice("affected display frames", report.Display.AffectedDisplayFrames, expected.AffectedDisplayFrames)
	checkSpanSlice("clean sample ranges", report.Display.CleanSampleRanges, expected.CleanSampleRanges)
	checkSpanSlice("clean display ranges", report.Display.CleanDisplayFrameRanges, expected.CleanDisplayFrameRanges)
	return failures
}

func findMetadataReferenceFinding(report *Report) *Finding {
	for i := range report.Findings {
		if report.Findings[i].Code == "metadata_entry_differs_from_reference" {
			return &report.Findings[i]
		}
	}
	return nil
}
