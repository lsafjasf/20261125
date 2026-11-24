package anc

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestParseJSONDocumentWords(t *testing.T) {
	words := mustPacket(t, 0x61, 0x01, []byte{0xCA, 0xFE})
	var nums strings.Builder
	for i, w := range words {
		if i > 0 {
			nums.WriteByte(',')
		}
		fmt.Fprintf(&nums, "%d", w)
	}
	doc := `{
		"schema":"ancscan/input-v1",
		"standard":"525i59.94",
		"lines":[{"field":1,"line":272,"words":[` + nums.String() + `]}]
	}`
	rep, err := ParseAndScan(strings.NewReader(doc), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Standard != "525i59.94" || rep.ScannedLines != 1 {
		t.Fatalf("report header wrong: %+v", rep)
	}
	if len(rep.Packets) != 1 || rep.Packets[0].Classification != ClassValidVANC {
		t.Fatalf("packets=%+v", rep.Packets)
	}
	if rep.Packets[0].ByteOffset != 0 || rep.Packets[0].WordOffset != 0 {
		t.Fatal("words input should map offset 0")
	}
}

func TestParseJSONLAndBase64(t *testing.T) {
	raw, _ := BuildANC8(0x61, 0x01, []byte{0x01, 0x02})
	b64 := base64.StdEncoding.EncodeToString(raw)
	jsonl := `{"field":1,"line":21,"encoding":"anc8","data_base64":"` + b64 + `"}` + "\n" +
		`{"field":2,"line":284,"encoding":"anc8","hex":"00 FF FF 61 01 00 62"}` + "\n"
	rep, err := ParseAndScan(strings.NewReader(jsonl), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 2 {
		t.Fatalf("expected 2 packets, got %d", len(rep.Packets))
	}
	if rep.Packets[0].PayloadHex != "0102" {
		t.Fatalf("base64 payload=%s", rep.Packets[0].PayloadHex)
	}
	// Second packet DC=0, checksum 61+01+00 = 0x62.
	if rep.Packets[1].Classification != ClassValidVANC {
		t.Fatalf("zero-length packet class=%s reasons=%v", rep.Packets[1].Classification, rep.Packets[1].Reasons)
	}
}

func TestParseHexLinesFormat(t *testing.T) {
	input := "# comment\nF1-L21: 00 FF FF 61 01 01 41 A4\n12: 00 FF FF CB 01 00 CC\n"
	lines, err := ParseHexLines(strings.NewReader(input), EncANC8)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines=%d", len(lines))
	}
	if lines[0].Field != 1 || lines[0].Number != 21 {
		t.Fatalf("prefix parse wrong: %+v", lines[0])
	}
	if lines[1].Number != 3 { // physical text line accounting comment? data on text line 3
		t.Fatalf("fallback line number=%d want 3", lines[1].Number)
	}
	rep, err := Scan(lines, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 2 {
		t.Fatalf("packets=%d", len(rep.Packets))
	}
}

func TestDecodeFlexHex0xAndSeparators(t *testing.T) {
	b, err := DecodeFlexHex("0x00, 0xFF:0xff\n61")
	if err != nil {
		t.Fatal(err)
	}
	if hexEncode(b) != "00ffff61" {
		t.Fatalf("got %x", b)
	}
}

func TestV210RoundTripScan(t *testing.T) {
	words := mustPacket(t, 0x61, 0x01, []byte{0x07, 0x08})
	v210 := encodeV210(words)
	decoded, _ := decodeV210(v210)
	if len(decoded) < len(words) {
		t.Fatalf("decoded %d units for %d words", len(decoded), len(words))
	}
	for i := range words {
		if decoded[i].w != words[i] {
			t.Fatalf("word %d roundtrip %03X != %03X", i, decoded[i].w, words[i])
		}
	}
	rep, err := Scan([]Line{{Number: 12, Bytes: v210, Encoding: EncV210}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 1 || rep.Packets[0].Classification != ClassValidVANC {
		t.Fatalf("v210 packets=%+v err=%v", rep.Packets, err)
	}
	if rep.Packets[0].ByteOffset != 0 {
		t.Fatalf("ADF byte offset=%d want 0", rep.Packets[0].ByteOffset)
	}
}

func TestParseExpectedLinesAdvisory(t *testing.T) {
	raw, _ := BuildANC8(0x60, 0x01, []byte{0x00})
	doc := `{"expected_lines":{"0x60":[12]},"lines":[` +
		`{"field":1,"line":99,"encoding":"anc8","data_hex":"` + hexEncode(raw) + `"}]}`
	rep, err := ParseAndScan(strings.NewReader(doc), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasLabel(rep.Packets[0].Labels, LabelDIDLineMismatch) {
		t.Fatalf("expected line mismatch label, got %v", rep.Packets[0].Labels)
	}
}
