package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"ffv1mkaudit/audit"
)

type Manifest struct {
	CleanFixture string             `json:"clean_fixture"`
	CleanSHA256  string             `json:"clean_sha256"`
	Expected     audit.Expectations `json:"expectations"`
	Injections   []Injection        `json:"injections"`
}

type Injection struct {
	File              string   `json:"file"`
	Kind              string   `json:"kind"`
	Description       string   `json:"description"`
	CorruptFrame      int      `json:"corrupt_frame,omitempty"`
	ElementOffset     int64    `json:"element_offset,omitempty"`
	CorruptOffset     int64    `json:"corrupt_offset,omitempty"`
	UnknownElements   int      `json:"unknown_elements,omitempty"`
	Frames            int      `json:"frames,omitempty"`
	Errors            int      `json:"errors,omitempty"`
	RecoverableSlices []int    `json:"recoverable_slices,omitempty"`
	RecoverableRanges [][2]int `json:"recoverable_ranges,omitempty"`
}

func main() {
	cleanPath := flag.String("clean", "testdata/ffv1-4slices.mkv", "known-good FFV1/Matroska fixture")
	outDir := flag.String("out", "injections", "output directory")
	flag.Parse()
	if err := run(*cleanPath, *outDir); err != nil {
		fmt.Fprintln(os.Stderr, "mkinject:", err)
		os.Exit(1)
	}
}

func run(cleanPath, outDir string) error {
	clean, err := os.ReadFile(cleanPath)
	if err != nil {
		return err
	}
	report, err := audit.AnalyzeReaderAt(bytes.NewReader(clean), int64(len(clean)), audit.Options{ExpectedSlices: 4})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	cleanSum := sha256.Sum256(clean)
	manifest := Manifest{
		CleanFixture: cleanPath,
		CleanSHA256:  hex.EncodeToString(cleanSum[:]),
		Expected:     expectationsFrom(report),
	}

	unknownPath := filepath.Join(outDir, "01-unknown-matroska-element.mkv")
	unknownData, unknownInj := makeUnknownInjection(clean, report)
	if err := summarize(unknownData, &unknownInj); err != nil {
		return err
	}
	if err := os.WriteFile(unknownPath, unknownData, 0o644); err != nil {
		return err
	}
	unknownInj.File = filepath.Base(unknownPath)
	manifest.Injections = append(manifest.Injections, unknownInj)

	crcPath := filepath.Join(outDir, "02-ffv1-slice-crc-block.mkv")
	crcData, crcInj, err := makeCRCInjection(clean, report)
	if err != nil {
		return err
	}
	if err := summarize(crcData, &crcInj); err != nil {
		return err
	}
	if err := os.WriteFile(crcPath, crcData, 0o644); err != nil {
		return err
	}
	crcInj.File = filepath.Base(crcPath)
	manifest.Injections = append(manifest.Injections, crcInj)

	truncPath := filepath.Join(outDir, "03-truncated-block.mkv")
	truncData, truncInj, err := makeTruncatedInjection(clean, report)
	if err != nil {
		return err
	}
	if err := summarize(truncData, &truncInj); err != nil {
		return err
	}
	if err := os.WriteFile(truncPath, truncData, 0o644); err != nil {
		return err
	}
	truncInj.File = filepath.Base(truncPath)
	manifest.Injections = append(manifest.Injections, truncInj)

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	expectationsData, err := json.MarshalIndent(manifest.Expected, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "expectations.json"), append(expectationsData, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "manifest.json"), append(manifestData, '\n'), 0o644)
}
