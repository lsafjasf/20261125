package pipeline

import (
	"strings"
	"testing"

	"logshoulder/internal/display"
)

func TestThreeExposuresRecoverWithoutMovingMidtones(t *testing.T) {
	shoulder := display.DefaultShoulder()
	for _, c := range Cases() {
		conv, err := Run(c, shoulder)
		if err != nil {
			t.Fatal(err)
		}
		finding := conv.Diagnose()
		if finding.PrematureClips == 0 {
			t.Errorf("%s: expected an early display clip", c.Name)
		}
		if finding.SourceClips != 0 {
			t.Errorf("%s: source curve is clipped; test data must stay below its ceiling", c.Name)
		}
		if !strings.Contains(finding.Stage, "before inverse Log") {
			t.Errorf("%s: wrong clip stage: %s", c.Name, finding.Stage)
		}
		if finding.RecoverableSceneLevels < 2 {
			t.Errorf("%s: curve did not preserve recoverable scene levels", c.Name)
		}
		m := conv.Measure()
		if m.FixedHighlightLevels != m.SourceHighlightLevels {
			t.Errorf("%s: fixed levels %d, source %d", c.Name, m.FixedHighlightLevels, m.SourceHighlightLevels)
		}
		if m.FixedHighlightLevels <= m.LegacyHighlightLevels {
			t.Errorf("%s: repair did not recover levels: %d <= %d", c.Name, m.FixedHighlightLevels, m.LegacyHighlightLevels)
		}
		if !m.FixedStrictlyOrdered || m.FixedLastGap <= 0 {
			t.Errorf("%s: repaired highlights are flat", c.Name)
		}
		if m.FixedWindowMaxCode >= 1 {
			t.Errorf("%s: repaired highlights still touch display white", c.Name)
		}
		if m.FixedMidtoneError > 1e-12 {
			t.Errorf("%s: midtone moved by %.9f", c.Name, m.FixedMidtoneError)
		}
		if m.FixedShadowError > 1e-12 {
			t.Errorf("%s: shadow moved by %.9f", c.Name, m.FixedShadowError)
		}
		if m.LegacyShadowLift <= 0.2 {
			t.Errorf("%s: legacy shadow lift not demonstrated: %.4f", c.Name, m.LegacyShadowLift)
		}
	}
}

func TestRepairDoesNotInventSaturatedSourceDetail(t *testing.T) {
	c := Cases()[0]
	c.Highlights = append([]float64{}, c.Highlights...)
	c.Highlights[len(c.Highlights)-1] = 64
	conv, err := Run(c, display.DefaultShoulder())
	if err != nil {
		t.Fatal(err)
	}
	f := conv.Diagnose()
	if f.SourceClips != 1 {
		t.Fatalf("source saturation count = %d, want 1", f.SourceClips)
	}
	if !strings.Contains(f.Stage, "before inverse Log") {
		t.Fatalf("recoverable clamp should still be reported: %s", f.Stage)
	}
	last := conv.Rows[len(conv.Rows)-1]
	if !last.SourceClipped || last.Code != 1 {
		t.Fatalf("saturated source must remain marked at code 1: %+v", last)
	}
}
