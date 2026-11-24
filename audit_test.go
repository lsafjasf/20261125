package cbarcheck

import (
	"bytes"
	"strings"
	"testing"
)

const (
	testWidth  = 160
	testHeight = 90
)

func testLayout(t *testing.T) Layout {
	t.Helper()
	layout, err := BarsLayout(PatternSMPTE100, testWidth, testHeight)
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func TestCleanSaturatedBarsPassAllDepths(t *testing.T) {
	layout := testLayout(t)
	for _, depth := range []BitDepth{Depth8, Depth10, Depth12} {
		target := Config{Range: RangeLimited, Matrix: MatrixBT709, Depth: depth}
		frame, err := GenerateFrame(layout, target)
		if err != nil {
			t.Fatal(err)
		}
		report, err := Analyze(frame, layout, target, DefaultThresholds())
		if err != nil {
			t.Fatal(err)
		}
		if !report.Summary.Compliant || report.FirstViolation != nil {
			t.Fatalf("%s: clean saturated bars failed: %+v", target, report.FirstViolation)
		}
		if report.Summary.ChromaViolationPixels != 0 || report.Summary.IllegalChromaPixels != 0 {
			t.Fatalf("%s: legal saturation counted as chroma error", target)
		}
		if report.Summary.ConfigurationMismatch || report.Summary.Best != target {
			t.Fatalf("%s: clean frame inferred as %s (score %.3f vs %.3f)",
				target, report.Summary.Best, report.Summary.BestScore, report.Summary.TargetScore)
		}
		bounds := Bounds(target)
		for _, region := range layout.Regions {
			if region.Name != "red100" {
				continue
			}
			expected, err := EncodeRGB(region.RGB, target)
			if err != nil {
				t.Fatal(err)
			}
			if !inRange(expected.Cr, bounds.CrMin, bounds.CrMax) {
				t.Fatalf("%s: expected saturated red Cr %.0f is illegal", target, expected.Cr)
			}
		}
	}
}

func TestEachSupportedConfigurationIdentifiesItself(t *testing.T) {
	layout := testLayout(t)
	configs := []Config{
		{Range: RangeLimited, Matrix: MatrixBT601, Depth: Depth8},
		{Range: RangeLimited, Matrix: MatrixBT709, Depth: Depth10},
		{Range: RangeLimited, Matrix: MatrixBT2020, Depth: Depth12},
		{Range: RangeFull, Matrix: MatrixBT601, Depth: Depth8},
		{Range: RangeFull, Matrix: MatrixBT709, Depth: Depth10},
		{Range: RangeFull, Matrix: MatrixBT2020, Depth: Depth12},
	}
	for _, cfg := range configs {
		frame, err := GenerateFrame(layout, cfg)
		if err != nil {
			t.Fatal(err)
		}
		report, err := Analyze(frame, layout, cfg, DefaultThresholds())
		if err != nil {
			t.Fatal(err)
		}
		if !report.Summary.Compliant || report.Summary.ConfigurationMismatch {
			t.Fatalf("%s: self-audit failed: first=%+v mismatch=%t",
				cfg, report.FirstViolation, report.Summary.ConfigurationMismatch)
		}
		if report.Summary.Best != cfg {
			t.Fatalf("%s: inferred %s", cfg, report.Summary.Best)
		}
	}
}

func TestFullBT601AgainstLimitedBT709FindsFirstPixel(t *testing.T) {
	layout := testLayout(t)
	observed := Config{Range: RangeFull, Matrix: MatrixBT601, Depth: Depth10}
	frame, err := GenerateFrame(layout, observed)
	if err != nil {
		t.Fatal(err)
	}
	target := Config{Range: RangeLimited, Matrix: MatrixBT709, Depth: Depth10}
	report, err := Analyze(frame, layout, target, DefaultThresholds())
	if err != nil {
		t.Fatal(err)
	}
	if report.FirstViolation == nil {
		t.Fatal("expected a violation")
	}
	v := report.FirstViolation
	if v.X != 0 || v.Y != 0 || v.Region != "white100" {
		t.Fatalf("first violation = (%d,%d) %s, want (0,0) white100", v.X, v.Y, v.Region)
	}
	if !contains(v.Reasons, "illegal_luma") || !contains(v.Reasons, "luma_level") {
		t.Fatalf("first reasons = %v", v.Reasons)
	}
	if report.Summary.IllegalChromaPixels == 0 {
		t.Fatal("expected illegal chroma in saturated bars")
	}
	if v := report.FirstChromaViolation; v == nil || v.X != 20 || v.Y != 0 || v.Region != "yellow100" {
		t.Fatalf("first chroma violation = %+v, want (20,0) yellow100", v)
	}
	if report.Summary.Best != observed {
		t.Fatalf("best config = %s, want %s", report.Summary.Best, observed)
	}
	if !report.Summary.ConfigurationMismatch {
		t.Fatal("expected range/matrix configuration mismatch")
	}
}

func TestFirstViolationUsesRasterOrder(t *testing.T) {
	layout := testLayout(t)
	target := Config{Range: RangeLimited, Matrix: MatrixBT709, Depth: Depth10}
	frame, err := GenerateFrame(layout, target)
	if err != nil {
		t.Fatal(err)
	}
	x, y := 42, 61
	i := y*testWidth + x
	frame.Cr[i] += 100
	report, err := Analyze(frame, layout, target, DefaultThresholds())
	if err != nil {
		t.Fatal(err)
	}
	if report.FirstViolation == nil {
		t.Fatal("expected a violation")
	}
	v := report.FirstViolation
	if v.X != x || v.Y != y {
		t.Fatalf("first violation = (%d,%d), want (%d,%d)", v.X, v.Y, x, y)
	}
	if len(v.Reasons) != 1 || v.Reasons[0] != "chroma_level" {
		t.Fatalf("reasons = %v, want [chroma_level]", v.Reasons)
	}
	if report.Summary.ChromaViolationPixels != 1 {
		t.Fatalf("chroma violations = %d, want 1", report.Summary.ChromaViolationPixels)
	}
}

func TestBitDepthMismatchIsNoticedByCandidateComparison(t *testing.T) {
	layout := testLayout(t)
	encoded := Config{Range: RangeLimited, Matrix: MatrixBT709, Depth: Depth10}
	frame, err := GenerateFrame(layout, encoded)
	if err != nil {
		t.Fatal(err)
	}
	target := Config{Range: RangeLimited, Matrix: MatrixBT709, Depth: Depth8}
	report, err := Analyze(frame, layout, target, DefaultThresholds())
	if err != nil {
		t.Fatal(err)
	}
	if report.FirstViolation == nil {
		t.Fatal("expected a depth violation")
	}
	if report.Summary.Best != encoded {
		t.Fatalf("best config = %s, want %s", report.Summary.Best, encoded)
	}
	if !report.Summary.ConfigurationMismatch {
		t.Fatal("expected configuration mismatch")
	}
}

func TestRaw444RoundTrip(t *testing.T) {
	layout := testLayout(t)
	for _, depth := range []BitDepth{Depth8, Depth10, Depth12} {
		cfg := Config{Range: RangeLimited, Matrix: MatrixBT709, Depth: depth}
		frame, err := GenerateFrame(layout, cfg)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := WriteRaw444(&buf, frame); err != nil {
			t.Fatal(err)
		}
		if int64(buf.Len()) != Raw444Size(testWidth, testHeight, depth) {
			t.Fatalf("%d-bit size = %d, want %d", depth, buf.Len(), Raw444Size(testWidth, testHeight, depth))
		}
		got, err := ReadRaw444(&buf, testWidth, testHeight, depth)
		if err != nil {
			t.Fatal(err)
		}
		for i := range frame.Y {
			if got.Y[i] != frame.Y[i] || got.Cb[i] != frame.Cb[i] || got.Cr[i] != frame.Cr[i] {
				t.Fatalf("%d-bit round trip differs at %d", depth, i)
			}
		}
	}
}

func TestJSONReportIsDeterministic(t *testing.T) {
	layout := testLayout(t)
	target := Config{Range: RangeLimited, Matrix: MatrixBT709, Depth: Depth10}
	frame, err := GenerateFrame(layout, target)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Analyze(frame, layout, target, DefaultThresholds())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Analyze(frame, layout, target, DefaultThresholds())
	if err != nil {
		t.Fatal(err)
	}
	var a, b bytes.Buffer
	if err := WriteJSON(&a, first); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(&b, second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("identical inputs produced different JSON reports")
	}
	if !strings.Contains(a.String(), `"schema": "cbarcheck/v1"`) {
		t.Fatal("JSON is missing schema")
	}
}

func TestTruncatedRawFrameIsRejected(t *testing.T) {
	layout := testLayout(t)
	cfg := Config{Range: RangeLimited, Matrix: MatrixBT709, Depth: Depth10}
	frame, err := GenerateFrame(layout, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WriteRaw444(&buf, frame); err != nil {
		t.Fatal(err)
	}
	truncated := buf.Bytes()[:buf.Len()-1]
	if _, err := ReadRaw444(bytes.NewReader(truncated), testWidth, testHeight, Depth10); err == nil {
		t.Fatal("expected truncated frame error")
	}
}
