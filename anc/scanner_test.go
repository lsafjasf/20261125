package anc

import (
	"testing"
)

func mustPacket(t *testing.T, did, sdid byte, payload []byte) []Word {
	t.Helper()
	ws, err := BuildWords(did, sdid, payload)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func scanOne(t *testing.T, l Line) *Packet {
	t.Helper()
	rep, err := Scan([]Line{l}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 1 {
		t.Fatalf("expected 1 packet, got %d", len(rep.Packets))
	}
	return rep.Packets[0]
}

func TestValidU16BEWithByteOffset(t *testing.T) {
	words := mustPacket(t, 0x61, 0x01, []byte{0xBA, 0xCD, 0x00, 0x01})
	pktBytes, err := SerializeWords(words, EncU16BE)
	if err != nil {
		t.Fatal(err)
	}
	prefix := []byte{0xDE, 0xAD, 0xBE, 0xEF} // 2 non-ADF u16 units
	line := Line{Field: 1, Number: 21, Bytes: append(prefix, pktBytes...), Encoding: EncU16BE}

	p := scanOne(t, line)
	if p.Classification != ClassValidVANC {
		t.Fatalf("class=%s labels=%v reasons=%v", p.Classification, p.Labels, p.Reasons)
	}
	if p.Field != 1 || p.Line != 21 || p.ByteOffset != 4 || p.WordOffset != 2 {
		t.Fatalf("location wrong: f=%d line=%d off=%d word=%d", p.Field, p.Line, p.ByteOffset, p.WordOffset)
	}
	if p.Offsets == nil || p.Offsets.DID != 10 || p.Offsets.Payload != 16 || p.Offsets.Checksum != 24 {
		t.Fatalf("field offsets wrong: %+v", p.Offsets)
	}
	if p.DID != 0x61 || p.SDIDOrDBN != 0x01 || p.DC != 4 {
		t.Fatalf("header wrong: did=%02X sdid=%02X dc=%d", p.DID, p.SDIDOrDBN, p.DC)
	}
	if p.PayloadHex != "bacd0001" {
		t.Fatalf("payload=%s", p.PayloadHex)
	}
	if p.RawHex != hexEncode(pktBytes) {
		t.Fatalf("raw mismatch:\n got %s\nwant %s", p.RawHex, hexEncode(pktBytes))
	}
	if p.Parity == nil || p.Parity.DID != "ok" || p.Checksum == nil || p.Checksum.Status != "ok" {
		t.Fatalf("parity/checksum not ok: %+v %+v", p.Parity, p.Checksum)
	}
}

func TestValidANC8UnknownDIDPreserved(t *testing.T) {
	// 0xCB has b7 set -> Type 1 (DBN) and is not in the registry.
	raw, err := BuildANC8(0xCB, 0x77, []byte{0x10, 0x20, 0x30})
	if err != nil {
		t.Fatal(err)
	}
	p := scanOne(t, Line{Field: 2, Number: 318, Bytes: raw, Encoding: EncANC8})
	if p.Classification != ClassValidVANC {
		t.Fatalf("class=%s reasons=%v", p.Classification, p.Reasons)
	}
	if !p.UnknownDID || p.DIDName == "" || p.DIDName[:8] != "Unknown " {
		t.Fatalf("unknown DID not labelled/preserved: %+v", p)
	}
	if p.IDType != "type1_dbn" {
		t.Fatalf("id type=%s", p.IDType)
	}
	if p.PayloadHex != "102030" || p.RawHex != hexEncode(raw) {
		t.Fatalf("payload/raw not preserved: %s %s", p.PayloadHex, p.RawHex)
	}
	if p.Parity.Present {
		t.Fatal("anc8 parity should be absent")
	}
}

func TestParityDamageStillExtracted(t *testing.T) {
	words := mustPacket(t, 0x60, 0x01, []byte{0x12, 0x34})
	words[3] ^= 1 << 8 // flip DID parity bit
	raw, err := SerializeWords(words, EncU16BE)
	if err != nil {
		t.Fatal(err)
	}
	p := scanOne(t, Line{Number: 13, Bytes: raw, Encoding: EncU16BE})
	if p.Classification != ClassInvalid {
		t.Fatalf("expected invalid, got %s", p.Classification)
	}
	if p.Parity.DID != "error" {
		t.Fatalf("DID parity status=%s", p.Parity.DID)
	}
	if !hasLabel(p.Labels, LabelParityError) || !hasLabel(p.Labels, LabelChecksumError) {
		t.Fatalf("expected parity+checksum labels, got %v", p.Labels)
	}
	if p.DID != 0x60 || p.PayloadHex != "1234" {
		t.Fatalf("data not recovered despite bad parity: did=%02X payload=%s", p.DID, p.PayloadHex)
	}
}

func TestChecksumMismatchANC8(t *testing.T) {
	raw, err := BuildANC8(0x61, 0x01, []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xFF // corrupt CS byte
	p := scanOne(t, Line{Number: 21, Bytes: raw, Encoding: EncANC8})
	if p.Classification != ClassInvalid {
		t.Fatalf("class=%s", p.Classification)
	}
	if p.Checksum.Status != "error" || p.Checksum.Mode != "sum8" {
		t.Fatalf("checksum info=%+v", p.Checksum)
	}
}

func TestDuplicateAcrossFields(t *testing.T) {
	words := mustPacket(t, 0x61, 0x01, []byte{0xDE, 0xAD})
	raw, _ := SerializeWords(words, EncU16BE)
	lines := []Line{
		{Field: 1, Number: 21, Bytes: raw, Encoding: EncU16BE},
		{Field: 2, Number: 284, Bytes: raw, Encoding: EncU16BE},
	}
	rep, err := Scan(lines, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.ValidVANC != 1 || rep.Summary.Duplicate != 1 {
		t.Fatalf("summary=%+v", rep.Summary)
	}
	first, second := rep.Packets[0], rep.Packets[1]
	if first.Classification != ClassValidVANC || second.Classification != ClassDuplicate {
		t.Fatalf("classes=%s,%s", first.Classification, second.Classification)
	}
	if second.DuplicateOf == nil || second.DuplicateOf.Line != 21 || second.DuplicateOf.Field != 1 {
		t.Fatalf("duplicate_of=%+v", second.DuplicateOf)
	}
	if second.DuplicateMatch != "raw" || !hasLabel(second.Labels, LabelExactDuplicate) {
		t.Fatalf("match=%s labels=%v", second.DuplicateMatch, second.Labels)
	}
}

func TestContentDuplicateWithParityDamage(t *testing.T) {
	words := mustPacket(t, 0x61, 0x01, []byte{0x01, 0x02})
	raw1, _ := SerializeWords(words, EncU16BE)

	damaged := append([]Word{}, words...)
	damaged[3] ^= 1 << 8
	raw2, _ := SerializeWords(damaged, EncU16BE)

	rep, err := Scan([]Line{
		{Number: 11, Bytes: raw1, Encoding: EncU16BE},
		{Number: 12, Bytes: raw2, Encoding: EncU16BE},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p2 := rep.Packets[1]
	if p2.Classification != ClassDuplicate || p2.DuplicateMatch != "content" {
		t.Fatalf("class=%s match=%s", p2.Classification, p2.DuplicateMatch)
	}
	if !hasLabel(p2.Labels, LabelParityError) {
		t.Fatalf("damaged duplicate should still carry parity label: %v", p2.Labels)
	}
}

func TestTruncatedMissingChecksumOnly(t *testing.T) {
	words := mustPacket(t, 0x61, 0x01, []byte{0x48, 0x49})
	full, _ := SerializeWords(words, EncU16BE)
	lineBytes := full[:len(full)-2] // drop CS
	p := scanOne(t, Line{Field: 1, Number: 22, Bytes: lineBytes, Encoding: EncU16BE})
	if p.Classification != ClassTruncated {
		t.Fatalf("class=%s", p.Classification)
	}
	if p.Checksum == nil || p.Checksum.Status != "missing" {
		t.Fatalf("checksum=%+v", p.Checksum)
	}
	if p.Continuation != nil {
		t.Fatal("no continuation line supplied, expected nil continuation")
	}
	if p.PayloadHex != "4849" {
		t.Fatalf("available payload should remain: %s", p.PayloadHex)
	}
}

func TestTruncatedReassembledAcrossLine(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03, 0x04}
	words := mustPacket(t, 0x61, 0x01, payload)
	full, _ := SerializeWords(words, EncU16BE)

	// Split after the second payload word: line1 keeps ADF..UDW[1],
	// line2 carries UDW[2], UDW[3], CS (3 units = 6 bytes).
	splitUnit := 6 + 2 // unit index after the two kept UDW
	line1 := full[:2*splitUnit]
	line2 := full[2*splitUnit:]

	rep, err := Scan([]Line{
		{Field: 1, Number: 23, Bytes: line1, Encoding: EncU16BE},
		{Field: 1, Number: 24, Bytes: line2, Encoding: EncU16BE},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 1 {
		t.Fatalf("continuation prefix must not create a second packet, got %d", len(rep.Packets))
	}
	p := rep.Packets[0]
	if p.Classification != ClassTruncated || p.Continuation == nil {
		t.Fatalf("class=%s cont=%+v", p.Classification, p.Continuation)
	}
	c := p.Continuation
	if !c.Reassembled || !c.ChecksumOK {
		t.Fatalf("expected reassembled+checksum ok: %+v checksum=%+v", c, p.Checksum)
	}
	if c.Field != 1 || c.Line != 24 || c.ByteOffset != 0 {
		t.Fatalf("continuation location=%+v", c.Location)
	}
	if p.PayloadHex != "01020304" {
		t.Fatalf("merged payload=%s", p.PayloadHex)
	}
	if rep.Summary.CrossLine != 1 || rep.Summary.ReassembledOK != 1 {
		t.Fatalf("summary=%+v", rep.Summary)
	}
}

func TestContinuationBadChecksum(t *testing.T) {
	words := mustPacket(t, 0x61, 0x01, []byte{0xAA, 0xBB})
	full, _ := SerializeWords(words, EncU16BE)
	line1 := full[:len(full)-2] // everything except CS
	badCS := []byte{0xDE, 0xAD}
	rep, err := Scan([]Line{
		{Number: 30, Bytes: line1, Encoding: EncU16BE},
		{Number: 31, Bytes: badCS, Encoding: EncU16BE},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := rep.Packets[0]
	if p.Classification != ClassTruncated {
		t.Fatalf("class=%s", p.Classification)
	}
	if p.Continuation == nil || !p.Continuation.Reassembled || p.Continuation.ChecksumOK {
		t.Fatalf("cont=%+v", p.Continuation)
	}
	if p.Checksum.Status != "error" {
		t.Fatalf("checksum=%+v", p.Checksum)
	}
}

func TestNextLineStartsADFNotConsumed(t *testing.T) {
	words := mustPacket(t, 0x61, 0x01, []byte{0x01, 0x02, 0x03})
	full, _ := SerializeWords(words, EncU16BE)
	// Keep only ADF+header+one UDW, DC=3 -> two UDW + CS missing (3 units).
	line1 := full[:2*7]
	nextADFBytes, _ := SerializeWords(mustPacket(t, 0x41, 0x05, []byte{0x00}), EncU16BE)
	rep, err := Scan([]Line{
		{Number: 40, Bytes: line1, Encoding: EncU16BE},
		{Number: 41, Bytes: nextADFBytes, Encoding: EncU16BE},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 2 {
		t.Fatalf("expected truncated + next-line packet, got %d", len(rep.Packets))
	}
	trunc := rep.Packets[0]
	if trunc.Classification != ClassTruncated || trunc.Continuation == nil || !trunc.Continuation.NextStartsADF {
		t.Fatalf("trunc=%s cont=%+v", trunc.Classification, trunc.Continuation)
	}
}

func TestMultipleANC8PacketsOneLine(t *testing.T) {
	p1, _ := BuildANC8(0x61, 0x01, []byte{0x01})
	p2, _ := BuildANC8(0x60, 0x01, []byte{0x02, 0x03})
	blob := append(append(append([]byte{}, p1...), 0x80, 0x90), p2...)
	rep, err := Scan([]Line{{Number: 15, Bytes: blob, Encoding: EncANC8}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 2 {
		t.Fatalf("packets=%d", len(rep.Packets))
	}
	if rep.Packets[0].ByteOffset != 0 || rep.Packets[1].ByteOffset != len(p1)+2 {
		t.Fatalf("offsets %d %d", rep.Packets[0].ByteOffset, rep.Packets[1].ByteOffset)
	}
}

func TestExpectedLineAdvisory(t *testing.T) {
	raw, _ := BuildANC8(0x61, 0x01, []byte{0x00})
	rep, err := Scan([]Line{{Field: 1, Number: 99, Bytes: raw, Encoding: EncANC8}},
		&Options{ExpectedLines: map[byte][]int{0x61: {21}}})
	if err != nil {
		t.Fatal(err)
	}
	p := rep.Packets[0]
	if !hasLabel(p.Labels, LabelDIDLineMismatch) {
		t.Fatalf("expected mismatch advisory, labels=%v", p.Labels)
	}
	if p.Classification != ClassValidVANC {
		t.Fatalf("advisory must not invalidate packet: %s", p.Classification)
	}
}

func TestParserWrapper(t *testing.T) {
	raw, _ := BuildANC8(0x61, 0x01, []byte{0x00})
	p := NewParser(&Options{Encoding: EncANC8})
	rep, err := p.Parse([]Line{{Number: 1, Bytes: raw}}) // line encoding falls back to opts
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 1 || rep.Packets[0].Classification != ClassValidVANC {
		t.Fatalf("wrapper report=%+v", rep)
	}
}

func TestHeaderShortDoesNotPanic(t *testing.T) {
	// ADF plus only DID/SDID, DC missing at line end.
	words := []Word{0, 0x3FF, 0x3FF, WithParity(0x61), WithParity(0x01)}
	raw, _ := SerializeWords(words, EncU16BE)
	rep, err := Scan([]Line{{Number: 9, Bytes: raw, Encoding: EncU16BE}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 1 || rep.Packets[0].Classification != ClassTruncated {
		t.Fatalf("packets=%+v", rep.Packets)
	}
	if !hasLabel(rep.Packets[0].Labels, LabelHeaderShort) {
		t.Fatalf("labels=%v", rep.Packets[0].Labels)
	}
}

func TestCorruptLargeDCIsBounded(t *testing.T) {
	// DC word claims 255 UDW but the line ends immediately after the header.
	words := []Word{0, 0x3FF, 0x3FF, WithParity(0x61), WithParity(0x01), WithParity(255)}
	raw, _ := SerializeWords(words, EncU16BE)
	rep, err := Scan([]Line{{Number: 9, Bytes: raw, Encoding: EncU16BE}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := rep.Packets[0]
	if p.Classification != ClassTruncated || p.DC != 255 {
		t.Fatalf("class=%s dc=%d", p.Classification, p.DC)
	}
}

func TestU16LERoundTrip(t *testing.T) {
	words := mustPacket(t, 0x61, 0x01, []byte{0xAB})
	raw, _ := SerializeWords(words, EncU16LE)
	p := scanOne(t, Line{Number: 1, Bytes: raw, Encoding: EncU16LE})
	if p.Classification != ClassValidVANC || p.PayloadHex != "ab" {
		t.Fatalf("class=%s payload=%s reasons=%v", p.Classification, p.PayloadHex, p.Reasons)
	}
}

func TestNoiseOnlyLines(t *testing.T) {
	rep, err := Scan([]Line{
		{Number: 1, Bytes: []byte{0x80, 0x10, 0x22, 0x44}, Encoding: EncANC8},
		{Number: 2, Encoding: EncWords, Words: []Word{0x200, 0x2FF, 0x155}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Packets) != 0 || rep.Summary.Packets != 0 {
		t.Fatalf("expected no packets, got %+v", rep.Packets)
	}
}
