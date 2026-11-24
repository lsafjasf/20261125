package report

import (
	"strings"
	"testing"

	"surrounddiag/internal/analyze"
)

func TestMarkdownStatesWideFieldGuard(t *testing.T) {
	report := &analyze.Report{
		File: "x.wav", SampleRate: 48000, Channels: 2, Bits: 24, Duration: 1,
		Layout: []string{"FL", "FR"},
		WideField: analyze.WideFieldCheck{
			Verdict: "FL/FR 无镜像证据；负相关或低相关不被判为反相。",
		},
		AnalysisNote: "test",
	}
	text := Markdown(report)
	if !strings.Contains(text, "无镜像证据") {
		t.Fatalf("missing wide-field guard:\n%s", text)
	}
	if !strings.Contains(text, "未发现达到证据门槛") {
		t.Fatalf("missing empty conclusion:\n%s", text)
	}
}
