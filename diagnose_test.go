package mp4diag_test

import (
	"testing"

	"mp4diag"
)

func TestInjectionManifest(t *testing.T) {
	if err := mp4diag.VerifyManifest("injections/manifest.json"); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataFaultIsNotPixelCorruptionWithoutReference(t *testing.T) {
	report, err := mp4diag.Analyze("injections/02-sample-table-metadata-only.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if report.Reference != nil {
		t.Fatal("reference should be absent")
	}
	if got := report.Layers[mp4diag.LayerCompressed].Status; got != mp4diag.StatusNotAssessed {
		t.Fatalf("compressed layer status = %q, want %q", got, mp4diag.StatusNotAssessed)
	}
	for _, finding := range report.Findings {
		if finding.PixelDamage == mp4diag.PixelConfirmed {
			t.Fatalf("metadata-only file produced confirmed pixel finding: %+v", finding)
		}
	}
}

func TestCompressedInjectionRequiresReferenceForHashEvidence(t *testing.T) {
	withoutRef, err := mp4diag.Analyze("injections/03-compressed-sample2-display-2-4.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if withoutRef.Reference != nil {
		t.Fatal("reference should be absent")
	}

	withRef, err := mp4diag.AnalyzeOptions(mp4diag.Options{
		Path:      "injections/03-compressed-sample2-display-2-4.mp4",
		Reference: "testdata/clean.mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := withRef.Reference.PixelDamage; got != mp4diag.PixelSuspected {
		t.Fatalf("pixel evidence = %q, want %q", got, mp4diag.PixelSuspected)
	}
	if len(withRef.Reference.ChangedSamples) != 1 || withRef.Reference.ChangedSamples[0] != 2 {
		t.Fatalf("changed samples = %v, want [2]", withRef.Reference.ChangedSamples)
	}
}
