package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"ancscan/anc"
)

func TestScanScenarioViaStdinInterspersedFlags(t *testing.T) {
	doc, err := BuildScenario()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"scan", "-", "--format", "json"}, bytes.NewReader(data), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}

	var rep anc.Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout.String())
	}
	want := anc.Summary{
		Packets: 9, ValidVANC: 3, Duplicate: 1, Truncated: 3, Invalid: 2,
		UnknownDID: 2, ParityErrors: 1, ChecksumErrors: 2,
		CrossLine: 1, ReassembledOK: 1,
	}
	if rep.Summary != want {
		t.Fatalf("summary=%+v want %+v", rep.Summary, want)
	}

	classCount := map[anc.Classification]int{}
	var sawFields bool
	for _, p := range rep.Packets {
		classCount[p.Classification]++
		if p.RawHex == "" || p.DIDHex == "" {
			t.Fatalf("packet missing raw/did: %+v", p)
		}
		if p.Field == 2 {
			sawFields = true
		}
	}
	if classCount[anc.ClassDuplicate] != 1 || classCount[anc.ClassTruncated] != 3 {
		t.Fatalf("class counts=%v", classCount)
	}
	if !sawFields {
		t.Fatal("expected field-2 packets")
	}
}

func TestScanTextFormat(t *testing.T) {
	doc, _ := BuildScenario()
	data, _ := json.Marshal(doc)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"scan", "--format", "text"}, bytes.NewReader(data), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"合法VANC", "重复包", "行间截断", "duplicate_of", "continuation"} {
		if !strings.Contains(out, want) {
			t.Fatalf("text output missing %q", want)
		}
	}
}

func TestUnknownCommandExit2(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := Run([]string{"frobnicate"}, nil, &out, &errBuf); code != 2 {
		t.Fatalf("code=%d", code)
	}
}
