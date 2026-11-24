package audit

import (
	"strings"
	"testing"
)

func statsFromValues(values []uint8) SampleStats {
	acc := NewAccumulator()
	for _, v := range values {
		acc.Add(v)
	}
	return acc.Stats(1, 1, 1)
}

func gradient(lo, hi, repeat int) []uint8 {
	var values []uint8
	for n := 0; n < repeat; n++ {
		for v := lo; v <= hi; v++ {
			values = append(values, uint8(v))
		}
	}
	return values
}

func TestLimitedWrongFullTag(t *testing.T) {
	values := gradient(16, 235, 10)
	values = append(values, make([]uint8, 0)...)
	for i := 0; i < 100; i++ {
		values = append(values, 16, 235)
	}
	a := Assess(DeclaredFull, statsFromValues(values))
	if a.Decision != "limited" || a.Confidence != "high" {
		t.Fatalf("decision = %s/%s, want limited/high; evidence=%v", a.Decision, a.Confidence, a.Evidence)
	}
	if a.LabelStatus != "incorrect" {
		t.Fatalf("label status = %s", a.LabelStatus)
	}
	if a.CorrectedLabel != "limited (16-235)" {
		t.Fatalf("corrected label = %q", a.CorrectedLabel)
	}
	h := a.Hypotheses[0]
	if h.BlackLevel.PedestalCodes != 0 || h.BlackLevel.Reading != "nominal-black" {
		t.Fatalf("limited black level = %+v", h.BlackLevel)
	}
	if len(a.Hypotheses) != 2 {
		t.Fatalf("must keep both range hypotheses")
	}
}

func TestFullRange(t *testing.T) {
	a := Assess(DeclaredFull, statsFromValues(gradient(0, 255, 16)))
	if a.Decision != "full" || a.Confidence != "high" {
		t.Fatalf("decision = %s/%s, want full/high", a.Decision, a.Confidence)
	}
	if a.LabelStatus != "correct" {
		t.Fatalf("label status = %s", a.LabelStatus)
	}
	h := a.Hypotheses[1]
	if h.BlackLevel.PedestalCodes > 1 {
		t.Fatalf("full nominal floor classified as lift: %+v", h.BlackLevel)
	}
}

func TestInsideRangeWithoutRailsStaysAmbiguous(t *testing.T) {
	a := Assess(DeclaredFull, statsFromValues(gradient(20, 220, 10)))
	if a.Decision != "ambiguous" || a.Confidence != "low" {
		t.Fatalf("decision = %s/%s, want ambiguous/low", a.Decision, a.Confidence)
	}
	if !strings.Contains(a.CorrectedLabel, "limited") || !strings.Contains(a.CorrectedLabel, "full") {
		t.Fatalf("ambiguous label lost an interpretation: %q", a.CorrectedLabel)
	}
	if len(a.Hypotheses) != 2 {
		t.Fatalf("ambiguous result must keep both hypotheses")
	}
	if a.Hypotheses[0].Score == 0 || a.Hypotheses[1].Score != 0 {
		t.Fatalf("unexpected scores: limited=%d full=%d", a.Hypotheses[0].Score, a.Hypotheses[1].Score)
	}
}

func TestFullLiftedBlack(t *testing.T) {
	a := Assess(DeclaredFull, statsFromValues(gradient(32, 255, 8)))
	if a.Decision != "full" {
		t.Fatalf("decision = %s, want full", a.Decision)
	}
	h := a.Hypotheses[1]
	if h.BlackLevel.PedestalCodes != 32 || h.BlackLevel.Severity != "strong" {
		t.Fatalf("unexpected lifted black: %+v", h.BlackLevel)
	}
	if len(h.BlackLevel.Alternatives) == 0 {
		t.Fatalf("lifted black must retain a stylistic alternative")
	}
}

func TestStylizedDarkReading(t *testing.T) {
	values := make([]uint8, 0, 1000)
	for i := 0; i < 1000; i++ {
		values = append(values, uint8(16+i%60))
	}
	for i := 0; i < 20; i++ {
		values = append(values, 245)
	}
	a := Assess(DeclaredFull, statsFromValues(values))
	if a.Decision != "full" {
		t.Fatalf("decision = %s, want full from guard-band evidence", a.Decision)
	}
	h := a.Hypotheses[1]
	if h.BlackLevel.PedestalCodes != 16 {
		t.Fatalf("floor = %d, want 16", h.BlackLevel.PedestalCodes)
	}
	if h.BlackLevel.StyleReading != "low-key-or-stylized-dark-compatible" {
		t.Fatalf("style = %s", h.BlackLevel.StyleReading)
	}
}

func TestDeclaredUnknown(t *testing.T) {
	values := gradient(16, 235, 10)
	for i := 0; i < 100; i++ {
		values = append(values, 16, 235)
	}
	a := Assess(DeclaredUnknown, statsFromValues(values))
	if a.LabelStatus != "inferred" {
		t.Fatalf("label status = %s, want inferred", a.LabelStatus)
	}
}
